package ports

import (
	"context"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// ErrGradeNameTaken is InsertGrade's answer when the level already holds a
// grade of that name at ANOTHER ordinal (grade names are unique per level,
// case-insensitively — migration 0006).
var ErrGradeNameTaken = glerrors.New(glerrors.KindConflict, "a grade with this name already exists in the level")

// PlacementStore is the unit of work of EnsurePlacements (legacy student
// import, ADR 0033): every lookup and every insert of one call share ONE
// tenant-bound transaction, so a preview reads one consistent catalog and a
// commit lands whole or not at all.
type PlacementStore interface {
	// InPlacementTx runs fn in one transaction bound to the tenant (RLS). An
	// error from fn rolls everything back. The PlacementTx is valid only inside
	// fn and carries ctx.
	InPlacementTx(ctx context.Context, fn func(PlacementTx) error) error
}

// PlacementTx is what EnsurePlacements may do inside its transaction. Lookups
// answer (value, found, err): a missing row is not an error here, the use case
// turns it into a per-placement answer.
//
// The Insert* methods are find-or-create and never race: they insert unless an
// equal row exists (INSERT … ON CONFLICT DO NOTHING) and return the stored
// row, reporting whether it is the one they inserted. A row a concurrent call
// committed first is returned with created=false — never duplicated.
type PlacementTx interface {
	// CycleExists reports whether the cycle exists (a malformed id does not).
	CycleExists(cycleID string) (bool, error)

	// LockLevel returns the level by id. The row stays share-locked until the
	// transaction ends, so a level deletion cannot begin under the import, and
	// Deleting() is final for the rest of the call.
	LockLevel(id string) (domain.Level, bool, error)
	// LockLevelByName returns the level whose FOLDED name equals name's
	// (config_fold_name), share-locked like LockLevel.
	LockLevelByName(name string) (domain.Level, bool, error)
	// InsertLevel stores l unless a level of the same folded name exists, and
	// returns the stored level, share-locked.
	InsertLevel(l domain.Level) (stored domain.Level, created bool, err error)

	// GradeByOrdinal returns the level's grade at ordinal.
	GradeByOrdinal(levelID string, ordinal int) (domain.GradeLevel, bool, error)
	// GradeNameTaken reports whether the level holds a grade of this name
	// (case-insensitively), at any ordinal.
	GradeNameTaken(levelID, name string) (bool, error)
	// InsertGrade stores g unless the level holds a grade at its ordinal, and
	// returns the grade at that ordinal. ErrGradeNameTaken when the name is held
	// at another ordinal.
	InsertGrade(g domain.GradeLevel) (stored domain.GradeLevel, created bool, err error)

	// GroupByName returns the cycle's group of the grade whose trimmed,
	// upper-cased name equals name (already normalized by the caller).
	GroupByName(cycleID, gradeLevelID, name string) (domain.Group, bool, error)
	// InsertGroup stores g unless the cycle's grade already has a group of that
	// name, and returns the stored group (its capacity untouched if it existed).
	InsertGroup(g domain.Group) (stored domain.Group, created bool, err error)
}
