package ports

import (
	"context"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

// This file declares the seams the grades use cases depend on. Interfaces live
// here (consumer side); adapters (config/students Connect clients, postgres)
// implement them. The domain imports none of this — dependencies point inward.
//
// The grades context has no local ownership of assignments or enrollments: it
// combines config (which teaching assignment = group+subject+teacher+cycle) with
// students (who is enrolled in the group) and stores grade_entries. Both
// cross-service views are minimal, read-only projections carrying only what the
// capture knot and roster need — never PII.

// AssignmentRef is the grades-owned view of a config teaching assignment: the
// group it teaches, and the subject/teacher/cycle it binds. subjectID and
// teacherID are DERIVED from here by the use case, never client-supplied.
type AssignmentRef struct {
	ID        string
	GroupID   string
	SubjectID string
	TeacherID string
	CycleID   string
}

// TermRef is the grades-owned view of a config term (its identity and cycle).
type TermRef struct {
	ID      string
	CycleID string
}

// AssignmentReader validates teaching-assignment and term references against the
// config bounded context (by RPC). It is a consumer-defined seam: the use case
// depends on this interface and stays testable with a fake, while the real
// adapter is a Connect client to config. A missing assignment/term is reported
// as glerrors.ErrNotFound (KindNotFound); any other error propagates unchanged.
type AssignmentReader interface {
	ByID(ctx context.Context, id string) (AssignmentRef, error)
	TermByID(ctx context.Context, id string) (TermRef, error)
}

// EnrollmentRef is the grades-owned view of a students enrollment: enough to
// place the student in a group of a cycle and know whether the enrollment is
// live. StudentID is carried through to the roster (names are resolved in the
// web from the students list — grades stays free of student PII).
type EnrollmentRef struct {
	ID        string
	StudentID string
	GroupID   string
	CycleID   string
	Status    string
}

// EnrollmentReader reads enrollments from the students bounded context (by RPC).
// A missing enrollment is reported as glerrors.ErrNotFound (KindNotFound); any
// other error propagates unchanged.
type EnrollmentReader interface {
	ByID(ctx context.Context, id string) (EnrollmentRef, error)
	ByGroup(ctx context.Context, groupID string) ([]EnrollmentRef, error)
}

// GradeEntryRepository loads and persists grade entries. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction,
// as in students/iam).
type GradeEntryRepository interface {
	// UpsertGradeEntry inserts a grade entry, or updates the value of the
	// existing row on the (tenant, enrollment, subject, term) key — one grade per
	// student·subject·term ("capturar/editar"). Returns the stored entry.
	UpsertGradeEntry(ctx context.Context, e domain.GradeEntry) (domain.GradeEntry, error)
	// GetForKeys batch-loads the grade entries for many enrollments under the
	// same (subject, term), in ONE tenant-bound query, so building a roster never
	// fans out into a grade lookup per student (N+1). The result is keyed by
	// enrollment_id and holds an entry ONLY for enrollments that have a grade;
	// absent enrollments are simply missing from the map. An empty enrollmentIDs
	// slice yields an empty map without touching the database.
	GetForKeys(ctx context.Context, enrollmentIDs []string, subjectID, termID string) (map[string]domain.GradeEntry, error)
	// ListGradeEntries returns entries for an enrollment in a term; an empty
	// string skips that filter.
	ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]domain.GradeEntry, error)
}
