package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

const (
	withdrawalOutboxDrainLimit = 5
	withdrawalOutboxTimeout    = 2 * time.Second
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
	// zone is the school's calendar: it decides the school's today and the
	// midnight a chosen entry day is stored at. UTC until WithSchoolZone.
	zone *time.Location
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
		zone:        time.UTC,
	}
}

// WithSchoolZone returns the service reading days in the school's zone,
// resolved once at boot (SCHOOL_TIMEZONE).
func (s *EnrollmentService) WithSchoolZone(zone *time.Location) *EnrollmentService {
	cp := *s
	cp.zone = zone
	return &cp
}

// enrolledAt is when a new enrolment starts. No entry day is the alta's
// instant, as before. A chosen day (a calendar date, midnight UTC) must lie
// between the cycle's first day and the school's today, and is stored as
// 00:00 of that day in the school's zone; billing starts in its month.
func (s *EnrollmentService) enrolledAt(ctx context.Context, cycleID string, on time.Time) (time.Time, error) {
	if on.IsZero() {
		return s.now(), nil
	}
	cycle, err := s.catalog.CycleByID(ctx, cycleID)
	if err != nil {
		return time.Time{}, err
	}
	return domain.EntryInstant(on, domain.SchoolDate(s.now(), s.zone), cycle.StartsOn, s.zone)
}

// Enroll runs the enroll knot: the student must exist locally, the group is
// validated against config (its cycle is derived), an optional term must belong
// to that same cycle, and the enrollment is persisted active. A duplicate
// (student, cycle) is a conflict. On success it best-effort publishes
// StudentEnrolled — a publish failure does NOT fail the enroll.
//
// on is the optional entry day (a calendar date; zero means now), see enrolledAt.
func (s *EnrollmentService) Enroll(ctx context.Context, studentID, groupID, termID string, on time.Time) (domain.Enrollment, error) {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return domain.Enrollment{}, err // KindNotFound propagates
	}

	grp, err := groupOf(ctx, s.catalog, groupID)
	if err != nil {
		return domain.Enrollment{}, err
	}
	cycleID := grp.CycleID // DERIVED from the group — never a client argument.
	if err := s.requireSameLevel(ctx, student, grp.GradeLevelID); err != nil {
		return domain.Enrollment{}, err
	}

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

	// A terminal file refuses the placement rather than quietly acquiring one:
	// a withdrawn pupil is not re-enrolled by being dropped into a group.
	if err := refusePlacementIfTerminal(student); err != nil {
		return domain.Enrollment{}, err
	}

	startsAt, err := s.enrolledAt(ctx, cycleID, on)
	if err != nil {
		return domain.Enrollment{}, err
	}
	enrollment, err := domain.NewEnrollment(s.newID(), s.tenant, student.ID(), cycleID, groupID, termID,
		domain.EnrollmentStatusActive, startsAt)
	if err != nil {
		return domain.Enrollment{}, err
	}
	seats := ports.GroupSeats{ID: grp.ID, Name: grp.Name, CycleID: grp.CycleID, Capacity: grp.Capacity}
	if err := s.enrollments.CreateEnrollment(ctx, enrollment, seats); err != nil {
		return domain.Enrollment{}, err // KindConflict on duplicate or no seat propagates
	}
	// The record follows the placement. Both enrolment paths write it, so a
	// pupil's file can never name a group they are not in. The ADMISSION status
	// is deliberately untouched: a pupil is "inscrito" when the inscription is
	// paid, and finance is what knows that.
	student.PlaceIn(grp.Name)
	if err := s.students.Update(ctx, student); err != nil {
		return domain.Enrollment{}, err
	}

	s.publishEnrolled(ctx, student, cycleID, enrollment.EnrolledAt())
	return enrollment, nil
}

// refusePlacementIfTerminal stops a rejected or withdrawn pupil being re-enrolled
// by dropping them into a group. Placement leaves the admission file alone
// otherwise — a pupil becomes "inscrito" when the inscription is PAID — but a
// terminal file is a decision the school already made, and a group is not a way
// around it.
func refusePlacementIfTerminal(student *domain.Student) error {
	if student.Status().IsTerminal() {
		return glerrors.New(glerrors.KindConflict,
			"ese alumno está dado de baja: no se le puede colocar en un grupo")
	}
	return nil
}

// MarkInscriptionPaid is what actually makes a pupil "inscrito".
//
// The school's rule, in its own words: a new pupil is en trámite, and inscrito
// only once the inscription is paid. Finance is what knows that a payment closed
// the inscription charge, so this runs off its ChargeSettled event.
//
// The walk is pending → accepted → enrolled rather than a jump, so an audit can
// tell a legal path from a bypassed state machine, and both steps are published.
// A pupil already enrolled is a NO-OP and not an error: payments arrive at least
// once, and a consumer that failed on a redelivery would be redelivered forever.
func (s *EnrollmentService) MarkInscriptionPaid(ctx context.Context, studentID string) error {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return err
	}
	if student.Status() == domain.AdmissionEnrolled {
		return nil
	}
	if student.Status().IsTerminal() {
		slog.InfoContext(ctx, "ignoring inscription payment for terminal admission")
		return nil
	}
	changes, err := admitAndEnroll(student, "")
	if err != nil {
		return err
	}
	if err := s.students.Update(ctx, student); err != nil {
		return err
	}
	s.publishAdmissionChanges(ctx, changes)
	return nil
}

// admitAndEnroll walks the pupil's file to enrolled, accepting them on the way if
// they were still an applicant. Walked rather than jumped so the acceptance is
// recorded: an audit that only ever saw pending → enrolled could not tell a legal
// path from a bypassed machine.
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
func (s *EnrollmentService) publishEnrolled(ctx context.Context, student *domain.Student, cycleID string, enrolledAt time.Time) {
	if s.events == nil {
		return
	}
	_ = s.events.PublishStudentEnrolled(ctx, domain.StudentEnrolledEvent{
		StudentID:  student.ID(),
		TenantID:   s.tenant,
		CycleID:    cycleID,
		LevelID:    student.Level().ID(),
		EnrolledAt: enrolledAt,
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

	toGrp, err := groupOf(ctx, s.catalog, toGroupID)
	if err != nil {
		return domain.Enrollment{}, err
	}
	if toGrp.CycleID != enr.CycleID() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "cannot transfer across cycles")
	}
	if toGroupID == enr.GroupID() {
		return domain.Enrollment{}, glerrors.New(glerrors.KindInvalid, "already in that group")
	}
	student, err := s.students.FindByID(ctx, enr.StudentID())
	if err != nil {
		return domain.Enrollment{}, err
	}
	// A withdrawn or rejected file is not moved around: neither enrol path
	// places one, and a transfer is a placement.
	if err := refusePlacementIfTerminal(student); err != nil {
		return domain.Enrollment{}, err
	}
	if err := s.requireSameLevel(ctx, student, toGrp.GradeLevelID); err != nil {
		return domain.Enrollment{}, err
	}

	transfer, err := domain.NewEnrollmentTransfer(s.newID(), s.tenant, enrollmentID,
		enr.GroupID(), toGroupID, reason, s.now())
	if err != nil {
		return domain.Enrollment{}, err
	}
	// One transaction: seats, history, the move and the pupil's file together,
	// so a failure can never leave the file naming the group they left.
	if err := s.enrollments.TransferEnrollment(ctx, ports.TransferCommand{
		EnrollmentID: enrollmentID,
		To:           ports.GroupSeats{ID: toGrp.ID, Name: toGrp.Name, CycleID: toGrp.CycleID, Capacity: toGrp.Capacity},
		Transfer:     transfer,
	}); err != nil {
		return domain.Enrollment{}, err
	}
	return s.enrollments.GetEnrollment(ctx, enrollmentID)
}

// requireSameLevel refuses a grade outside the pupil's level. Four groups can
// all be called "1° A" — one per level — and a pupil placed in the wrong one is
// billed, graded and listed as another level's. The grade's level is ALWAYS
// resolved, because the lookup also refuses a level whose deletion is running
// (LEVEL_BEING_DELETED, ADR 0021) — for every pupil. A legacy pupil with no
// level id (migration 0007 could not link it) has nothing to compare against
// and otherwise passes.
func (s *EnrollmentService) requireSameLevel(ctx context.Context, student *domain.Student, gradeLevelID string) error {
	got, err := levelOfGrade(ctx, s.catalog, gradeLevelID)
	if err != nil {
		return err
	}
	want := student.Level().ID()
	if want != "" && got != want {
		return glerrors.New(glerrors.KindInvalid,
			"ese grupo es de otro nivel: el alumno está en "+student.Level().Name())
	}
	return nil
}

// groupOf resolves a group against config. A group config does not know is the
// caller's mistake (KindInvalid "group not found"); any other failure is
// config's and propagates unchanged.
func groupOf(ctx context.Context, catalog ports.CatalogValidator, groupID string) (ports.GroupRef, error) {
	grp, err := catalog.GroupByID(ctx, groupID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return ports.GroupRef{}, glerrors.New(glerrors.KindInvalid, "group not found")
	}
	return grp, err
}

// levelOfGrade resolves the config level a grade belongs to. The lookup also
// refuses a level whose deletion is running (LEVEL_BEING_DELETED, ADR 0021). A
// grade config does not know is KindInvalid.
func levelOfGrade(ctx context.Context, catalog ports.CatalogValidator, gradeLevelID string) (string, error) {
	level, err := catalog.LevelOfGrade(ctx, gradeLevelID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return "", glerrors.New(glerrors.KindInvalid, "ese grado no existe")
	}
	return level, err
}

// Withdraw terminates an active enrollment and records a durable event in the
// same transaction. The relay is attempted immediately; the periodic sweep
// retries any event the broker did not accept.
func (s *EnrollmentService) Withdraw(ctx context.Context, enrollmentID string, on time.Time, reason string) (domain.Enrollment, error) {
	enr, err := s.enrollments.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return domain.Enrollment{}, err
	}
	if on.IsZero() {
		on = s.now().UTC()
	}
	updated, err := enr.Withdraw(on, reason, s.now())
	if err != nil {
		return domain.Enrollment{}, err
	}
	if err := s.enrollments.WithdrawEnrollment(ctx, updated); err != nil {
		return domain.Enrollment{}, err
	}
	if s.events != nil {
		drainCtx, cancel := context.WithTimeout(ctx, withdrawalOutboxTimeout)
		defer cancel()
		if _, err := s.enrollments.DrainOutbox(drainCtx, withdrawalOutboxDrainLimit, s.events.PublishOutboxEvent); err != nil {
			slog.Warn("students could not drain the outbox after withdrawal", slog.Any("error", err))
		}
	}
	return updated, nil
}

// GetEnrollment returns an enrollment by id (KindNotFound if absent).
func (s *EnrollmentService) GetEnrollment(ctx context.Context, id string) (domain.Enrollment, error) {
	return s.enrollments.GetEnrollment(ctx, id)
}

// ListTransfers returns a pupil's group moves, oldest first. A transfer rewrites
// the enrollment's group in place, so this history is the only record of which
// group the pupil was in on a past day. The id is checked here: the column is a
// uuid, and a malformed one is the caller's mistake, not an outage.
func (s *EnrollmentService) ListTransfers(ctx context.Context, studentID string) ([]domain.EnrollmentTransfer, error) {
	if err := uuid.Validate(studentID); err != nil {
		return nil, glerrors.New(glerrors.KindInvalid, "invalid student id")
	}
	return s.enrollments.ListTransfers(ctx, studentID)
}

// ListTransfersIntoGroup returns every move INTO a group, oldest first, so a
// class list learns in ONE read when each pupil joined it.
func (s *EnrollmentService) ListTransfersIntoGroup(ctx context.Context, groupID string) ([]domain.EnrollmentTransfer, error) {
	if err := uuid.Validate(groupID); err != nil {
		return nil, glerrors.New(glerrors.KindInvalid, "invalid group id")
	}
	return s.enrollments.ListTransfersIntoGroup(ctx, groupID)
}

// ListEnrollments returns enrollments matching the given filters ("" skips a filter).
func (s *EnrollmentService) ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error) {
	return s.enrollments.ListEnrollments(ctx, studentID, cycleID, groupID)
}

// Placement is where a pupil landed: the enrolment, and the group the school's
// own capacity chose for them. The group's NAME travels with it because the only
// reader is a person being told "quedó en 1° B".
type Placement struct {
	Enrollment domain.Enrollment
	GroupID    string
	GroupName  string
}

// EnrollIntoGrade places a pupil in a GRADE and lets capacity choose the group.
//
// What a school knows when a child arrives is the level and the grade. Which
// group they land in is a consequence of how full each one is — the panel used
// to ask for "grado y grupo" as free text, so a secretary typed "1° A" and
// nothing checked whether 1°A had a seat left.
//
// The choosing happens inside the repository's transaction, not here. Reading the
// occupancy and then writing would be a race with a real cost: two secretaries
// enrolling at the same moment would each see 24 of 25 seats taken and each take
// the last one.
//
// A grade with no groups and a grade with no seats are different problems and get
// different sentences: one needs a group created, the other needs a decision
// about opening one.
//
// on is the optional entry day (a calendar date; zero means now), see enrolledAt.
func (s *EnrollmentService) EnrollIntoGrade(ctx context.Context, studentID, gradeLevelID string, on time.Time) (Placement, error) {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return Placement{}, err
	}

	if err := s.requireSameLevel(ctx, student, gradeLevelID); err != nil {
		return Placement{}, err
	}
	groups, err := s.catalog.GroupsOfGrade(ctx, gradeLevelID)
	if err != nil {
		return Placement{}, err
	}
	if len(groups) == 0 {
		return Placement{}, glerrors.New(glerrors.KindInvalid,
			"ese grado no tiene ningún grupo: créalo en Configuración antes de inscribir")
	}

	if err := refusePlacementIfTerminal(student); err != nil {
		return Placement{}, err
	}

	// Every group of a grade is in the active cycle (GroupsOfGrade), so the
	// first one names the cycle the entry day is bounded by.
	startsAt, err := s.enrolledAt(ctx, groups[0].CycleID, on)
	if err != nil {
		return Placement{}, err
	}
	draft, err := domain.NewEnrollment(s.newID(), s.tenant, student.ID(), groups[0].CycleID,
		groups[0].ID, "", domain.EnrollmentStatusActive, startsAt)
	if err != nil {
		return Placement{}, err
	}
	chosen, err := s.enrollments.CreateEnrollmentInFirstFreeGroup(ctx, draft, groups)
	if err != nil {
		return Placement{}, err
	}
	student.PlaceIn(chosen.Name)
	if err := s.students.Update(ctx, student); err != nil {
		return Placement{}, err
	}

	s.publishEnrolled(ctx, student, chosen.CycleID, draft.EnrolledAt())
	return Placement{Enrollment: draft, GroupID: chosen.ID, GroupName: chosen.Name}, nil
}
