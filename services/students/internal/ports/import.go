package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// ExistingPupil is a pupil the school already has, as the legacy import needs
// to see them: who they are and the groups they are actively placed in.
type ExistingPupil struct {
	StudentID      string
	ActiveGroupIDs []string
}

// ImportCommand is one pupil of the legacy import (ADR 0033): the pupil,
// already enrolled and placed, and their enrollment (origin import).
type ImportCommand struct {
	Student    *domain.Student
	Enrollment domain.Enrollment
	BatchID    string
	// NumberYear is the year the matrícula is numbered in: the school's
	// calendar year of the import, not the database's UTC year.
	NumberYear int
}

// ImportedPupil is what storing one imported pupil did.
type ImportedPupil struct {
	// EnrollmentNumber is the matrícula the store gave.
	EnrollmentNumber string
	// BankReferenceDropped says the pupil's reference was already another
	// pupil's, so they were stored without one.
	BankReferenceDropped bool
}

// CycleStartMove asks to move earlier imports back to the first day of their
// cycle (owner decision 2026-10-17): the import enrollments (origin import) of
// these pupils in CycleID dated after StartsAt get StartsAt. Every other
// enrollment, and a withdrawn one's end date, is left alone. DryRun only
// reports who would move.
type CycleStartMove struct {
	CycleID    string
	StartsAt   time.Time
	StudentIDs []string
	DryRun     bool
}

// ImportRepository is the store side of the legacy import. All access is
// tenant-scoped for RLS.
//
//   - StudentsByCURP returns the school's pupils holding any of the CURPs,
//     keyed by CURP (absent = new to the school).
//   - BankReferencesTaken returns which of the references a pupil already holds.
//   - ImportStudent stores ONE pupil in ONE transaction: the reference is
//     checked first and dropped if taken (so the insert never aborts on it), the
//     matrícula is allocated, the pupil and the enrollment are inserted. A CURP
//     taken meanwhile is domain.ErrCURPTaken; a reference taken between the
//     check and the insert is domain.ErrBankReferenceTaken. Group capacity is
//     NOT checked: the import planned its seats in config beforehand.
//   - MoveImportedToCycleStart applies (or, dry, previews) a CycleStartMove in
//     one statement and returns the pupils it moved; a second run moves nobody.
type ImportRepository interface {
	StudentsByCURP(ctx context.Context, curps []string) (map[string]ExistingPupil, error)
	BankReferencesTaken(ctx context.Context, refs []string) (map[string]bool, error)
	ImportStudent(ctx context.Context, cmd ImportCommand) (ImportedPupil, error)
	MoveImportedToCycleStart(ctx context.Context, move CycleStartMove) ([]string, error)
}
