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
