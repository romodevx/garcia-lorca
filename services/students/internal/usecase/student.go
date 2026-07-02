package usecase

import (
	"context"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// StudentService orchestrates the students domain: it creates applicants and
// drives the admission state machine, persisting via the repository and
// publishing the resulting domain events. It depends only on ports + domain.
//
//	ChangeAdmissionStatus:
//	  load ─> domain transition (enforces the machine) ─> repo.Update ─> publish events
//	                      │illegal
//	                      └── KindConflict (nothing persisted/published)
type StudentService struct {
	repo   ports.StudentRepository
	pub    ports.EventPublisher
	tenant string
	newID  func() string
}

// NewStudentService wires the use case. `tenant` is the single-tenant scope for
// M03; `newID` mints student ids (injectable for deterministic tests).
func NewStudentService(repo ports.StudentRepository, pub ports.EventPublisher, tenant string, newID func() string) *StudentService {
	return &StudentService{repo: repo, pub: pub, tenant: tenant, newID: newID}
}

// CreateApplicantInput is the raw input for a new applicant (CURP is validated
// here; Level is already the domain type, mapped from the wire by the handler).
type CreateApplicantInput struct {
	CURP         string
	FirstName    string
	LastName     string
	Level        domain.Level
	GradeGroup   string
	GuardianName string
}

// CreateApplicant validates the input, builds a pending applicant, and persists
// it. A duplicate CURP (per tenant) surfaces as KindConflict from the repo.
func (s *StudentService) CreateApplicant(ctx context.Context, in CreateApplicantInput) (*domain.Student, error) {
	curp, err := domain.NewCURP(in.CURP)
	if err != nil {
		return nil, err
	}
	student, err := domain.NewApplicant(domain.NewApplicantParams{
		ID:           s.newID(),
		TenantID:     s.tenant,
		CURP:         curp,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Level:        in.Level,
		GradeGroup:   in.GradeGroup,
		GuardianName: in.GuardianName,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, student); err != nil {
		return nil, err
	}
	return student, nil
}

// ChangeStatusInput drives one admission transition.
type ChangeStatusInput struct {
	ID      string
	Target  domain.AdmissionStatus
	Reason  string // for reject/withdraw
	CycleID string // for enroll
}

// ChangeAdmissionStatus applies the requested transition to a student. The
// domain enforces the state machine (illegal → KindConflict); on success it
// persists and publishes the events. Events are published only after a
// successful Update, so a failed write publishes nothing.
func (s *StudentService) ChangeAdmissionStatus(ctx context.Context, in ChangeStatusInput) (*domain.Student, error) {
	student, err := s.repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	changed, enrolled, err := applyTransition(student, in)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, student); err != nil {
		return nil, err
	}

	if err := s.pub.PublishAdmissionChanged(ctx, changed); err != nil {
		return nil, err
	}
	if enrolled != nil {
		if err := s.pub.PublishStudentEnrolled(ctx, *enrolled); err != nil {
			return nil, err
		}
	}
	return student, nil
}

// applyTransition dispatches to the right domain method for the target status.
func applyTransition(student *domain.Student, in ChangeStatusInput) (domain.AdmissionChangedEvent, *domain.StudentEnrolledEvent, error) {
	switch in.Target {
	case domain.AdmissionAccepted:
		ev, err := student.Accept()
		return ev, nil, err
	case domain.AdmissionRejected:
		ev, err := student.Reject(in.Reason)
		return ev, nil, err
	case domain.AdmissionWithdrawn:
		ev, err := student.Withdraw(in.Reason)
		return ev, nil, err
	case domain.AdmissionEnrolled:
		enrolled, ev, err := student.Enroll(in.CycleID)
		if err != nil {
			return domain.AdmissionChangedEvent{}, nil, err
		}
		return ev, &enrolled, nil
	default:
		return domain.AdmissionChangedEvent{}, nil, glerrors.New(glerrors.KindInvalid, "unsupported target admission status")
	}
}

// GetStudent returns a student by id (KindNotFound if absent).
func (s *StudentService) GetStudent(ctx context.Context, id string) (*domain.Student, error) {
	return s.repo.FindByID(ctx, id)
}

// ListStudents returns students matching the filter plus the next page token.
func (s *StudentService) ListStudents(ctx context.Context, f ports.ListFilter) ([]*domain.Student, string, error) {
	return s.repo.List(ctx, f)
}
