package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Compile-time proof the adapter satisfies the placement port.
var _ ports.PlacementStore = (*Repository)(nil)

// shareLock is SELECT … FOR SHARE: it conflicts with the UPDATE that begins a
// level deletion (status → 'deleting'), so a level the import resolved stays
// as it was read until the import commits; it does not conflict with other
// imports' share locks or with the FK checks of the rows they insert.
var shareLock = clause.Locking{Strength: clause.LockingStrengthShare}

// InPlacementTx runs fn in one tenant-bound transaction (RLS via
// pkg/authctx). The freeze trigger's refusal, should it ever fire, becomes
// LEVEL_BEING_DELETED for the whole call (withTenant); the share locks make
// that unreachable in practice, because the use case reads the level's status
// under the lock before writing anything below it.
func (r *Repository) InPlacementTx(ctx context.Context, fn func(ports.PlacementTx) error) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return fn(placementTx{db: tx})
	})
}

// placementTx is ports.PlacementTx over one GORM transaction (which carries
// the request's context).
type placementTx struct{ db *gorm.DB }

func (p placementTx) CycleExists(cycleID string) (bool, error) {
	if !isUUID(cycleID) {
		return false, nil
	}
	var n int64
	if err := p.db.Model(&cycleModel{}).Where("id = ?", cycleID).Count(&n).Error; err != nil {
		return false, translateLockErr(err, "looking up the cycle")
	}
	return n > 0, nil
}

func (p placementTx) LockLevel(id string) (domain.Level, bool, error) {
	if !isUUID(id) {
		return domain.Level{}, false, nil
	}
	return p.lockLevel("id = ?", id)
}

func (p placementTx) LockLevelByName(name string) (domain.Level, bool, error) {
	return p.lockLevel("config_fold_name(name) = config_fold_name(?)", name)
}

func (p placementTx) lockLevel(where string, args ...any) (domain.Level, bool, error) {
	var m levelModel
	err := p.db.Clauses(shareLock).Where(where, args...).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Level{}, false, nil
	}
	if err != nil {
		return domain.Level{}, false, translateLockErr(err, "looking up the level")
	}
	l, err := toDomainLevel(m)
	return l, err == nil, err
}

// InsertLevel is INSERT … ON CONFLICT DO NOTHING with NO target. levels has two
// unique keys — the original (tenant_id, name) and the folded one of migration
// 0019 — and a targeted ON CONFLICT only absorbs its own: two imports that both
// passed the pre-check would then fail on the other with "duplicate key"
// (caught by CI, reproduced by TestInsertLevelNeverFailsUnderAStampede). An
// exact duplicate is also a folded one, so whichever key fires, the level is
// then found by its folded name and returned.
func (p placementTx) InsertLevel(l domain.Level) (domain.Level, bool, error) {
	m := fromDomainLevel(l)
	res := p.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
	if res.Error != nil {
		return domain.Level{}, false, translateCreateErr(res.Error, "creating level", "a level with this name already exists")
	}
	if res.RowsAffected == 1 {
		return l, true, nil
	}
	stored, found, err := p.LockLevelByName(l.Name())
	if err != nil {
		return domain.Level{}, false, err
	}
	if !found {
		return domain.Level{}, false, glerrors.New(glerrors.KindConflict, "the level changed during the import; retry")
	}
	return stored, false, nil
}

func (p placementTx) GradeByOrdinal(levelID string, ordinal int) (domain.GradeLevel, bool, error) {
	var m gradeLevelModel
	err := p.db.Where("level_id = ? AND ordinal = ?", levelID, ordinal).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.GradeLevel{}, false, nil
	}
	if err != nil {
		return domain.GradeLevel{}, false, translateLockErr(err, "looking up the grade")
	}
	g, err := toDomainGradeLevel(m)
	return g, err == nil, err
}

// GradeNameTaken compares as the unique index of migration 0006 does:
// lower(name).
func (p placementTx) GradeNameTaken(levelID, name string) (bool, error) {
	var n int64
	if err := p.db.Model(&gradeLevelModel{}).Where("level_id = ? AND lower(name) = lower(?)", levelID, name).Count(&n).Error; err != nil {
		return false, translateLockErr(err, "looking up the grade name")
	}
	return n > 0, nil
}

// InsertGrade is INSERT … ON CONFLICT DO NOTHING with no target, so it yields
// to BOTH unique keys of grade_levels: the ordinal (the grade exists — it is
// returned) and the name (held at another ordinal — ErrGradeNameTaken).
func (p placementTx) InsertGrade(g domain.GradeLevel) (domain.GradeLevel, bool, error) {
	m := fromDomainGradeLevel(g)
	res := p.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
	if res.Error != nil {
		return domain.GradeLevel{}, false, translateCreateErr(res.Error, "creating grade level", "a grade level with this ordinal already exists in the level")
	}
	if res.RowsAffected == 1 {
		return g, true, nil
	}
	stored, found, err := p.GradeByOrdinal(g.LevelID(), g.Ordinal())
	if err != nil {
		return domain.GradeLevel{}, false, err
	}
	if !found {
		return domain.GradeLevel{}, false, ports.ErrGradeNameTaken
	}
	return stored, false, nil
}

// GroupByName matches the stored name trimmed and upper-cased, so a group
// typed "a " by hand is the import's "A". Several such spellings can coexist
// (the unique key is exact); the first by name wins, deterministically.
func (p placementTx) GroupByName(cycleID, gradeLevelID, name string) (domain.Group, bool, error) {
	var m groupModel
	err := p.db.Where("cycle_id = ? AND grade_level_id = ? AND upper(btrim(name)) = ?", cycleID, gradeLevelID, name).
		Order("name").Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Group{}, false, nil
	}
	if err != nil {
		return domain.Group{}, false, translateLockErr(err, "looking up the group")
	}
	g, err := toDomainGroup(m)
	return g, err == nil, err
}

// InsertGroup is INSERT … ON CONFLICT DO NOTHING on the group's one unique
// key (tenant, cycle, grade, name); an existing group is returned untouched.
func (p placementTx) InsertGroup(g domain.Group) (domain.Group, bool, error) {
	m := fromDomainGroup(g)
	res := p.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
	if res.Error != nil {
		return domain.Group{}, false, translateCreateErr(res.Error, "creating group", "a group with this name already exists in the cycle and grade level")
	}
	if res.RowsAffected == 1 {
		return g, true, nil
	}
	stored, found, err := p.GroupByName(g.CycleID(), g.GradeLevelID(), g.Name())
	if err != nil {
		return domain.Group{}, false, err
	}
	if !found {
		return domain.Group{}, false, glerrors.New(glerrors.KindConflict, "the group changed during the import; retry")
	}
	return stored, false, nil
}

// translateLockErr maps a failed read inside the import's transaction: a
// deadlock or serialization failure between two imports is a retryable
// KindConflict, anything else KindInternal.
func translateLockErr(err error, msg string) error {
	switch pgCode(err) {
	case deadlockDetected, serializationFailure:
		return glerrors.Wrap(glerrors.KindConflict, "the request collided with a concurrent change; retry", err)
	default:
		return glerrors.Wrap(glerrors.KindInternal, msg, err)
	}
}
