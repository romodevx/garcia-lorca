package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies the level-archive port.
var _ ports.LevelArchiveRepository = (*Repository)(nil)

// levelTrash is finance's half of the deletion archive (migration 0019).
var levelTrash = gormarchive.New("finance_deletion_archive")

// The kinds finance reports. Only tuition plans are ever archived; charges and
// payments are counted by the preview so the director sees the money that
// STAYS, and are never archived, deleted or altered.
const (
	kindTuitionPlans = "tuitionPlans"
	kindCharges      = "charges"
	kindPayments     = "payments"
)

// restoreSkipReason is the one sentence a skipped tuition plan gets.
const restoreSkipReason = "ya hay otra colegiatura para ese ciclo y nivel"

// tuitionPlansStep is the whole of finance's deletion: the level's tabulador
// across every cycle. No charge references a plan, so no kept row depends on
// one that leaves.
func tuitionPlansStep(s levelarchive.Scope) gormarchive.Step {
	return gormarchive.Step{Kind: kindTuitionPlans, Table: "tuition_plans", Where: "level_id = ?", Args: []any{s.LevelID}}
}

// keptMoney counts, never moves, the level's pupils' charges and payments.
func keptMoney(s levelarchive.Scope) []gormarchive.Step {
	return []gormarchive.Step{
		{Kind: kindCharges, Table: "charges", Where: "student_id IN ?", Args: []any{s.StudentIDs}},
		{Kind: kindPayments, Table: "payments",
			Where: "charge_id IN (SELECT id FROM charges WHERE student_id IN ?)", Args: []any{s.StudentIDs}},
	}
}

// PreviewLevelDeletion counts the plans the deletion would archive and the
// money that stays, changing nothing.
func (r *Repository) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error) {
	counts := levelarchive.Counts{}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, s := range append([]gormarchive.Step{tuitionPlansStep(scope)}, keptMoney(scope)...) {
			n, err := gormarchive.Count(tx, s)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the level's finance rows", err)
			}
			counts[s.Kind] = n
		}
		return nil
	})
	return counts, err
}

// ArchiveLevel moves the level's tuition plans into the trash, in one
// transaction. A retried batch answers its recorded counts.
func (r *Repository) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	var counts levelarchive.Counts
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			n, err := levelTrash.Move(tx, batchID, 0, tuitionPlansStep(scope))
			if err != nil {
				return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the level's tuition plans", err)
			}
			return levelarchive.Counts{kindTuitionPlans: n}, nil
		})
		return err
	})
	return counts, err
}

// RestoreLevelBatch puts the plans back, skipping a (cycle, level) that got a
// new plan meanwhile.
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			out := levelarchive.NewReport()
			restored, skipped, err := levelTrash.Reinsert(tx, batchID, "tuition_plans")
			if err != nil {
				return out, glerrors.Wrap(glerrors.KindInternal, "restoring the level's tuition plans", err)
			}
			out.Record(kindTuitionPlans, restored, skipped, restoreSkipReason)
			return out, nil
		})
		return err
	})
	return rep, err
}

// PurgeLevelArchives drops the archives filed before cutoff.
func (r *Repository) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the finance trash", err)
		}
		return nil
	})
	return n, err
}
