package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// EnrollmentService places a student into a group of a cycle and moves them
// between groups with a dated, reasoned history. It orchestrates three seams:
// the local StudentRepository, the EnrollmentRepository, and the cross-service
// CatalogValidator (config, by RPC — faked in tests). It depends only on ports +
// domain; the cycle an enrollment lands in is always DERIVED from its group, so a
// client can never pick a cycle directly.
//
//	Enroll: student exists ─> group (RPC) ─> cycle := group.cycle
//	        ─> [term (RPC) in same cycle] ─> persist active ─> publish StudentEnrolled
type EnrollmentService struct {
	students    ports.StudentRepository
	enrollments ports.EnrollmentRepository
	catalog     ports.CatalogValidator
	events      ports.EventPublisher // optional; nil-safe (best-effort publish)
	tenant      string
	newID       func() string
	now         func() time.Time
}

// NewEnrollmentService wires the use case. `tenant` is the single-tenant scope
// for this phase; `newID` mints ids and `now` stamps enrollments/transfers (both
// injectable for deterministic tests). `events` may be nil.
func NewEnrollmentService(
	students ports.StudentRepository,
	enrollments ports.EnrollmentRepository,
	catalog ports.CatalogValidator,
	events ports.EventPublisher,
	tenant string,
	newID func() string,
	now func() time.Time,
) *EnrollmentService {
	return &EnrollmentService{
		students:    students,
		enrollments: enrollments,
		catalog:     catalog,
		events:      events,
		tenant:      tenant,
		newID:       newID,
		now:         now,
	}
}

// Enroll runs the enroll knot: the student must exist locally, the group is
// validated against config (its cycle is derived), an optional term must belong
// to that same cycle, and the enrollment is persisted active. A duplicate
// (student, cycle) is a conflict. On success it best-effort publishes
// StudentEnrolled — a publish failure does NOT fail the enroll.
func (s *EnrollmentService) Enroll(ctx context.Context, studentID, groupID, termID string) (domain.Enrollment, error) {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return domain.Enrollment{}, err // KindNotFound propagates
	}

	grp, err := s.catalog.GroupByID(ctx, groupID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "group not found")
		}
		return domain.Enrollment{}, err
	}
	cycleID := grp.CycleID // DERIVED from the group — never a client argument.

	if termID != "" {
		term, err := s.catalog.TermByID(ctx, termID)
		if err != nil {
			if glerrors.KindOf(err) == glerrors.KindNotFound {
				return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "term not found")
			}
			return domain.Enrollment{}, err
		}
		if term.CycleID != cycleID {
			return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "term does not belong to the group's cycle")
		}
	}

	// The FILE moves first, and it moves before anything is persisted.
	//
	// Two paths could change a pupil's standing — this one and
	// ChangeAdmissionStatus — and this one used to move only the placement,
	// leaving the record behind. A school then had pupils actively enrolled in
	// groups whose files all still read "Trámite": the roster's own tabs meant
	// nothing, and so did every report built on admission status.
	//
	// Doing it first is what makes a terminal file (a withdrawn pupil) refuse the
	// placement instead of quietly acquiring one.
	changes, err := admitAndEnroll(student, cycleID)
	if err != nil {
		return domain.Enrollment{}, err
	}

	enrollment, err := domain.NewEnrollment(s.newID(), s.tenant, student.ID(), cycleID, groupID, termID,
		domain.EnrollmentStatusActive, s.now())
	if err != nil {
		return domain.Enrollment{}, err
	}
	if err := s.enrollments.CreateEnrollment(ctx, enrollment); err != nil {
		return domain.Enrollment{}, err // KindConflict on duplicate propagates
	}
	if err := s.students.Update(ctx, student); err != nil {
		return domain.Enrollment{}, err
	}

	s.publishAdmissionChanges(ctx, changes)
	s.publishEnrolled(ctx, student, cycleID)
	return enrollment, nil
}

// admitAndEnroll walks the pupil's file to enrolled, accepting them on the way if
// they were still an applicant.
//
// Placing a child in 3°A IS the school accepting them — there is no separate
// "accept" action anywhere in the product — but the acceptance is walked through
// the state machine rather than jumped, and returned so it can be published.
// An audit that only ever saw pending → enrolled could not tell a legal path from
// a bypassed machine.
func admitAndEnroll(student *domain.Student, cycleID string) ([]domain.AdmissionChangedEvent, error) {
	var changes []domain.AdmissionChangedEvent
	if student.Status() == domain.AdmissionPending {
		accepted, err := student.Accept()
		if err != nil {
			return nil, err
		}
		changes = append(changes, accepted)
	}
	_, enrolled, err := student.Enroll(cycleID)
	if err != nil {
		return nil, err
	}
	return append(changes, enrolled), nil
}

// publishAdmissionChanges emits the file's movements best-effort, for the same
// reason the enrolment event is best-effort: the placement is already durable,
// and a broker that is down must not undo a child's enrolment.
func (s *EnrollmentService) publishAdmissionChanges(ctx context.Context, changes []domain.AdmissionChangedEvent) {
	if s.events == nil {
		return
	}
	for _, change := range changes {
		_ = s.events.PublishAdmissionChanged(ctx, change)
	}
}

// publishEnrolled emits StudentEnrolled best-effort: it is skipped when no
// publisher is wired and never fails the enroll on a publish error (the
// enrollment is already durably persisted).
func (s *EnrollmentService) publishEnrolled(ctx context.Context, student *domain.Student, cycleID string) {
	if s.events == nil {
		return
	}
	_ = s.events.PublishStudentEnrolled(ctx, domain.StudentEnrolledEvent{
		StudentID: student.ID(),
		TenantID:  s.tenant,
		CycleID:   cycleID,
		Level:     student.Level(),
	})
}

// Transfer moves an active enrollment to another group in the SAME cycle, writing
// a reasoned history row and updating the group in one repository transaction.
func (s *EnrollmentService) Transfer(ctx context.Context, enrollmentID, toGroupID, reason string) (domain.Enrollment, error) {
	enr, err := s.enrollments.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return domain.Enrollment{}, err
	}
	if !enr.CanTransfer() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "enrollment is not active")
	}

	toGrp, err := s.catalog.GroupByID(ctx, toGroupID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "group not found")
		}
		return domain.Enrollment{}, err
	}
	if toGrp.CycleID != enr.CycleID() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "cannot transfer across cycles")
	}
	if toGroupID == enr.GroupID() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "already in that group")
	}

	transfer, err := domain.NewEnrollmentTransfer(s.newID(), s.tenant, enrollmentID,
		enr.GroupID(), toGroupID, reason, s.now())
	if err != nil {
		return domain.Enrollment{}, err
	}
	if err := s.enrollments.TransferEnrollment(ctx, enrollmentID, toGroupID, transfer); err != nil {
		return domain.Enrollment{}, err
	}
	return s.enrollments.GetEnrollment(ctx, enrollmentID)
}

// Withdraw terminates an active enrollment. Withdrawing a non-active enrollment
// is invalid.
func (s *EnrollmentService) Withdraw(ctx context.Context, enrollmentID string) (domain.Enrollment, error) {
	enr, err := s.enrollments.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return domain.Enrollment{}, err
	}
	if !enr.CanWithdraw() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "enrollment is not active")
	}
	if err := s.enrollments.SetEnrollmentStatus(ctx, enrollmentID, domain.EnrollmentStatusWithdrawn); err != nil {
		return domain.Enrollment{}, err
	}
	return s.enrollments.GetEnrollment(ctx, enrollmentID)
}

// GetEnrollment returns an enrollment by id (KindNotFound if absent).
func (s *EnrollmentService) GetEnrollment(ctx context.Context, id string) (domain.Enrollment, error) {
	return s.enrollments.GetEnrollment(ctx, id)
}

// ListEnrollments returns enrollments matching the given filters ("" skips a filter).
func (s *EnrollmentService) ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error) {
	return s.enrollments.ListEnrollments(ctx, studentID, cycleID, groupID)
}
