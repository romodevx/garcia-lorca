package ports

import (
	"context"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// This file declares the seams the students use cases depend on. Interfaces live
// here (consumer side); adapters (postgres, pubsub) implement them. The domain
// imports none of this — dependencies point inward.

// ListFilter narrows a ListStudents query. Zero values mean "no filter".
type ListFilter struct {
	Level      domain.Level
	Status     domain.AdmissionStatus
	GradeGroup string
	PageSize   int
	PageToken  string
}

// StudentRepository loads and persists students. All access is tenant-scoped for
// RLS (the adapter binds app.current_tenant per transaction, as in iam).
type StudentRepository interface {
	// Create inserts a new applicant. A duplicate CURP (per tenant) is KindConflict.
	Create(ctx context.Context, s *domain.Student) error
	// Update persists a status transition (and any mutable fields).
	Update(ctx context.Context, s *domain.Student) error
	// FindByID returns a student or KindNotFound.
	FindByID(ctx context.Context, id string) (*domain.Student, error)
	// List returns students matching the filter plus the next page token ("" = last page).
	List(ctx context.Context, f ListFilter) (students []*domain.Student, nextPageToken string, err error)
}

// EventPublisher emits the module's domain events (StudentEnrolled,
// AdmissionStatusChanged). The pubsub adapter implements it; the use case calls
// it after a successful state transition is persisted.
type EventPublisher interface {
	PublishStudentEnrolled(ctx context.Context, ev domain.StudentEnrolledEvent) error
	PublishAdmissionChanged(ctx context.Context, ev domain.AdmissionChangedEvent) error
}

// GroupRef is the students-owned view of a config group. It is the minimum the
// enroll knot needs: the group's identity and the cycle it derives from.
type GroupRef struct {
	ID           string
	CycleID      string
	GradeLevelID string
}

// TermRef is the students-owned view of a config term (its identity and cycle).
type TermRef struct {
	ID      string
	CycleID string
}

// CatalogValidator validates cross-service references against the config bounded
// context (by RPC). It is a consumer-defined seam: the use case depends on this
// interface and stays testable with a fake, while the real adapter is a Connect
// client to config. A missing group/term is reported as glerrors.ErrNotFound
// (KindNotFound); any other error propagates unchanged.
type CatalogValidator interface {
	GroupByID(ctx context.Context, id string) (GroupRef, error)
	TermByID(ctx context.Context, id string) (TermRef, error)
}

// EnrollmentRepository loads and persists enrollments and their transfer history.
// All access is tenant-scoped for RLS (the adapter binds app.current_tenant per
// transaction, as in students/iam).
type EnrollmentRepository interface {
	// CreateEnrollment inserts a new enrollment. A duplicate (student, cycle) per
	// tenant is KindConflict.
	CreateEnrollment(ctx context.Context, e domain.Enrollment) error
	// GetEnrollment returns an enrollment or KindNotFound.
	GetEnrollment(ctx context.Context, id string) (domain.Enrollment, error)
	// ListEnrollments returns enrollments matching the given filters; an empty
	// string skips that filter.
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error)
	// TransferEnrollment writes the transfer history row and updates the
	// enrollment's group in ONE transaction.
	TransferEnrollment(ctx context.Context, enrollmentID, toGroupID string, transfer domain.EnrollmentTransfer) error
	// SetEnrollmentStatus updates an enrollment's status (e.g. to withdrawn).
	SetEnrollmentStatus(ctx context.Context, id string, status domain.EnrollmentStatus) error
}

// GuardianRepository loads and persists guardian ↔ student links. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction, as
// in students/enrollments).
type GuardianRepository interface {
	// Link inserts a guardian ↔ student link. A duplicate
	// (guardian, student) per tenant is KindConflict.
	Link(ctx context.Context, l domain.GuardianLink) error
	// Unlink removes a link. An unknown (or invisible) link is KindNotFound.
	Unlink(ctx context.Context, guardianUserID, studentID string) error
	// ListByStudent returns the links of one student (its guardians).
	ListByStudent(ctx context.Context, studentID string) ([]domain.GuardianLink, error)
	// ListByGuardian returns the links of one guardian (their students).
	ListByGuardian(ctx context.Context, guardianUserID string) ([]domain.GuardianLink, error)
}

// UserRef is the students-owned view of an iam user: the minimum the guardian
// link needs to decide whether the referenced principal may be a guardian — its
// identity, account status, and the names of the roles it holds.
type UserRef struct {
	ID     string
	Status string
	Roles  []string
}

// UserReader resolves a user against the iam bounded context (by RPC). Like
// CatalogValidator it is a consumer-defined seam: the use case depends on this
// interface and stays testable with a fake, while the real adapter is a Connect
// client to iam. A missing user is reported as glerrors.ErrNotFound
// (KindNotFound); any other error propagates unchanged.
type UserReader interface {
	ByID(ctx context.Context, userID string) (UserRef, error)
}
