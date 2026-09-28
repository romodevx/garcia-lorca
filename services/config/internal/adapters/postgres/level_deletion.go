package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Compile-time proof the adapter satisfies the level-deletion port.
var _ ports.LevelDeletionRepository = (*Repository)(nil)

// levelTrash is config's half of the deletion archive (migration 0011).
var levelTrash = gormarchive.New("config_deletion_archive")

// The kinds config reports, one per table it archives.
const (
	kindLevels              = "levels"
	kindGradeLevels         = "gradeLevels"
	kindSubjects            = "subjects"
	kindGroups              = "groups"
	kindTerms               = "terms"
	kindTermMonths          = "termMonths"
	kindTeachingAssignments = "teachingAssignments"
	kindSchoolEvents        = "schoolEvents"
)

// Sub-selects that resolve the level's structure. They are evaluated per
// statement, which is why levelSteps deletes children before the rows these
// read.
const (
	gradesOfLevel   = "SELECT id FROM grade_levels WHERE level_id = ?"
	groupsOfLevel   = "SELECT id FROM groups WHERE grade_level_id IN (" + gradesOfLevel + ")"
	subjectsOfLevel = "SELECT id FROM subjects WHERE grade_level_id IN (" + gradesOfLevel + ")"
)

// levelSteps is the deletion, children first: every step's predicate still
// finds the rows it reads because those go later.
func levelSteps(levelID string) []gormarchive.Step {
	step := func(kind, table, where string) gormarchive.Step {
		return gormarchive.Step{Kind: kind, Table: table, Where: where, Args: levelArgs(where, levelID)}
	}
	return []gormarchive.Step{
		step(kindTermMonths, "term_months", "level_id = ?"),
		step(kindTeachingAssignments, "teaching_assignments",
			"group_id IN ("+groupsOfLevel+") OR subject_id IN ("+subjectsOfLevel+")"),
		step(kindSchoolEvents, "school_events", "level_id = ? OR group_id IN ("+groupsOfLevel+")"),
		step(kindGroups, "groups", "grade_level_id IN ("+gradesOfLevel+")"),
		step(kindSubjects, "subjects", "grade_level_id IN ("+gradesOfLevel+")"),
		step(kindTerms, "terms", "level_id = ?"),
		step(kindGradeLevels, "grade_levels", "level_id = ?"),
		step(kindLevels, "levels", "id = ?"),
	}
}

// levelArgs binds the level id to every placeholder of a predicate: each
// sub-select above names the level once.
func levelArgs(where, levelID string) []any {
	args := make([]any, strings.Count(where, "?"))
	for i := range args {
		args[i] = levelID
	}
	return args
}

// maxListRows bounds the trash listing (every list read here is capped).
const maxListRows = 500

// restoreStep is one table's way back: parents it must still find.
type restoreStep struct {
	kind    string
	table   string
	parents []gormarchive.Parent
}

// levelRestore is the way back, parents first. The level itself goes first and
// alone decides whether the batch can come back (see RestoreLevelBatch).
var levelRestore = []restoreStep{
	{kindGradeLevels, "grade_levels", []gormarchive.Parent{{Column: "level_id", Table: "levels"}}},
	{kindSubjects, "subjects", []gormarchive.Parent{{Column: "grade_level_id", Table: "grade_levels"}}},
	{kindTerms, "terms", []gormarchive.Parent{{Column: "cycle_id", Table: "cycles"}, {Column: "level_id", Table: "levels"}}},
	{kindTermMonths, "term_months", []gormarchive.Parent{{Column: "term_id", Table: "terms"}}},
	{kindGroups, "groups", []gormarchive.Parent{{Column: "cycle_id", Table: "cycles"}, {Column: "grade_level_id", Table: "grade_levels"}}},
	{kindTeachingAssignments, "teaching_assignments", []gormarchive.Parent{
		{Column: "cycle_id", Table: "cycles"}, {Column: "group_id", Table: "groups"},
		{Column: "subject_id", Table: "subjects"}, {Column: "teacher_id", Table: "teachers"},
	}},
	{kindSchoolEvents, "school_events", []gormarchive.Parent{{Column: "cycle_id", Table: "cycles"}}},
}

// skipReason is the one sentence a skipped structure row gets.
const skipReason = "ya no cabe: choca con lo que se capturó después o su ciclo, grupo o docente ya no existe"

// levelDeletionModel is a row of level_deletions.
type levelDeletionModel struct {
	BatchID         string    `gorm:"column:batch_id;primaryKey"`
	TenantID        string    `gorm:"column:tenant_id"`
	LevelID         string    `gorm:"column:level_id"`
	LevelName       string    `gorm:"column:level_name"`
	Counts          string    `gorm:"column:counts;type:jsonb"`
	DeletedBy       string    `gorm:"column:deleted_by"`
	DeletedByName   string    `gorm:"column:deleted_by_name"`
	DeletedAt       time.Time `gorm:"column:deleted_at"`
	RestorableUntil time.Time `gorm:"column:restorable_until"`
	Status          string    `gorm:"column:status"`
	PendingSteps    string    `gorm:"column:pending_steps;->"` // text[] read as its literal
	Skipped         string    `gorm:"column:skipped;->"`
	RestoringFrom   *string   `gorm:"column:restoring_from;->"`
}

func (levelDeletionModel) TableName() string { return "level_deletions" }

func toDomainLevelDeletion(m levelDeletionModel) (domain.LevelDeletion, error) {
	counts := levelarchive.Counts{}
	if err := json.Unmarshal([]byte(m.Counts), &counts); err != nil {
		return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "decoding a deletion's counts", err)
	}
	skipped := []levelarchive.Skip{}
	if m.Skipped != "" {
		if err := json.Unmarshal([]byte(m.Skipped), &skipped); err != nil {
			return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "decoding a deletion's skips", err)
		}
	}
	d := domain.LevelDeletion{
		BatchID: m.BatchID, LevelID: m.LevelID, LevelName: m.LevelName, Counts: counts,
		DeletedBy: m.DeletedBy, DeletedByName: m.DeletedByName,
		DeletedAt: m.DeletedAt.UTC(), RestorableUntil: m.RestorableUntil.UTC(),
		Status: domain.LevelDeletionStatus(m.Status), PendingSteps: parseTextArray(m.PendingSteps), Skipped: skipped,
	}
	if m.RestoringFrom != nil {
		d.RestoringFrom = domain.LevelDeletionStatus(*m.RestoringFrom)
	}
	return d, nil
}

// parseTextArray reads a Postgres text[] literal of plain words ({a,b}); the
// steps it holds are fixed identifiers, never quoted.
func parseTextArray(lit string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(lit, "{"), "}")
	if inner == "" {
		return []string{}
	}
	return strings.Split(inner, ",")
}

// levelStatusDeleting / levelStatusActive are levels.status (migration 0012).
const (
	levelStatusActive   = "active"
	levelStatusDeleting = "deleting"
)

// levelBeingDeletedMessage is what migration 0012's triggers raise.
const levelBeingDeletedMessage = "LEVEL_BEING_DELETED"

// objectNotInPrerequisiteState is the SQLSTATE those triggers raise.
const objectNotInPrerequisiteState = "55000"

// asLevelBeingDeleted turns the freeze trigger's refusal into the coded
// conflict; any other error passes through unchanged.
func asLevelBeingDeleted(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == objectNotInPrerequisiteState && pgErr.Message == levelBeingDeletedMessage {
		return levelarchive.LevelBeingDeleted()
	}
	return err
}

// PreviewLevelDeletion resolves the level's structure and counts, changing
// nothing.
func (r *Repository) PreviewLevelDeletion(ctx context.Context, levelID string) (ports.LevelStructure, error) {
	var out ports.LevelStructure
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var level levelModel
		if err := tx.Where("id = ?", levelID).First(&level).Error; err != nil {
			return translateFindErr(err)
		}
		out = ports.LevelStructure{
			LevelID: level.ID, LevelName: level.Name, Deleting: level.Status == levelStatusDeleting,
			Counts: levelarchive.Counts{},
		}
		for _, s := range levelSteps(levelID) {
			n, err := gormarchive.Count(tx, s)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the level's structure", err)
			}
			out.Counts[s.Kind] = n
		}
		for _, q := range []struct {
			sql  string
			dest *[]string
		}{
			{gradesOfLevel + " ORDER BY id", &out.GradeLevelIDs},
			{groupsOfLevel + " ORDER BY id", &out.GroupIDs},
			{subjectsOfLevel + " ORDER BY id", &out.SubjectIDs},
			{"SELECT id FROM terms WHERE level_id = ? ORDER BY id", &out.TermIDs},
		} {
			if err := tx.Raw(q.sql, levelArgs(q.sql, levelID)...).Scan(q.dest).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "resolving the level's structure", err)
			}
		}
		return nil
	})
	return out, err
}

// BeginLevelDeletion is the saga's lock and its first step. Under the level's
// row lock: an existing record of the batch is answered as is (a retry); a
// level already being deleted is DELETION_IN_PROGRESS; a used token is
// PREVIEW_STALE. Otherwise the level turns deleting and the batch is recorded
// on the database's clock.
func (r *Repository) BeginLevelDeletion(ctx context.Context, in ports.BeginLevelDeletionInput) (domain.LevelDeletion, error) {
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var level levelModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", in.LevelID).First(&level).Error; err != nil {
			return translateFindErr(err)
		}
		if existing, err := getLevelDeletion(tx, in.BatchID); err == nil {
			rec = existing
			return nil
		} else if glerrors.KindOf(err) != glerrors.KindNotFound {
			return err
		}
		if level.Status == levelStatusDeleting {
			return domain.DeletionInProgress()
		}
		var used int64
		if err := tx.Model(&levelDeletionModel{}).Where("token_hash = ?", in.TokenHash).Count(&used).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking the confirm token", err)
		}
		if used > 0 {
			return domain.PreviewTokenUsed()
		}
		// Validates the ids and names; its instants are replaced by now() below.
		d, err := domain.NewLevelDeletion(in.BatchID, in.LevelID, level.Name, in.DeletedBy, in.DeletedByName, nil, time.Time{})
		if err != nil {
			return err
		}
		err = tx.Exec("UPDATE levels SET status = ?, deleting_batch_id = ?, updated_at = now() WHERE id = ?",
			levelStatusDeleting, d.BatchID, in.LevelID).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "freezing the level", err)
		}
		err = tx.Exec(`INSERT INTO level_deletions (batch_id, tenant_id, level_id, level_name, counts, deleted_by,
				deleted_by_name, deleted_at, restorable_until, status, token_hash)
			VALUES (?, ?, ?, ?, '{}'::jsonb, ?, ?, now(), now() + make_interval(secs => ?), ?, ?)`,
			d.BatchID, r.tenantID, d.LevelID, d.LevelName, d.DeletedBy, d.DeletedByName,
			levelarchive.Retention.Seconds(), string(domain.LevelDeletionDeleting), in.TokenHash).Error
		switch {
		case isConstraint(err, "level_deletions_token_once"):
			return domain.PreviewTokenUsed()
		case isConstraint(err, "level_deletions_one_running"):
			return domain.DeletionInProgress()
		case err != nil:
			return glerrors.Wrap(glerrors.KindInternal, "recording the deletion", err)
		}
		rec, err = getLevelDeletion(tx, d.BatchID)
		return err
	})
	return rec, err
}

// ArchiveLevel moves the structure into the trash and completes the begun
// batch, in one transaction. A retried batch answers its recorded counts and
// ledger row.
func (r *Repository) ArchiveLevel(ctx context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	var (
		counts levelarchive.Counts
		rec    domain.LevelDeletion
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		begun, err := getLevelDeletion(tx, in.BatchID)
		if err != nil {
			return err
		}
		if begun.LevelID != in.LevelID {
			return glerrors.New(glerrors.KindInvalid, "the batch belongs to another level")
		}
		counts, err = levelTrash.ArchiveBatch(tx, in.BatchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			return r.archiveLevel(tx, in, begun)
		})
		if err != nil {
			return err
		}
		rec, err = getLevelDeletion(tx, in.BatchID)
		return err
	})
	return counts, rec, err
}

func (r *Repository) archiveLevel(tx *gorm.DB, in ports.ArchiveLevelInput, begun domain.LevelDeletion) (levelarchive.Counts, error) {
	if begun.Status != domain.LevelDeletionDeleting {
		return nil, glerrors.New(glerrors.KindConflict, "the deletion is not running")
	}
	counts := levelarchive.Counts{}
	for i, s := range levelSteps(in.LevelID) {
		n, err := levelTrash.Move(tx, in.BatchID, i, s)
		if err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the level's structure", err)
		}
		counts[s.Kind] = n
	}
	total := levelarchive.Counts{}
	total.Add(in.Summary)
	total.Add(counts)
	body, err := json.Marshal(total)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "encoding a deletion's counts", err)
	}
	err = tx.Model(&levelDeletionModel{}).Where("batch_id = ?", in.BatchID).
		Updates(map[string]any{"counts": gorm.Expr("?::jsonb", string(body)), "status": string(domain.LevelDeletionDeleted), "updated_at": gorm.Expr("now()")}).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "recording the deletion", err)
	}
	return counts, nil
}

// dbNow is the database's clock — the one restorable_until was written with.
func dbNow(tx *gorm.DB) (time.Time, error) {
	var now time.Time
	if err := tx.Raw("SELECT now()").Scan(&now).Error; err != nil {
		return time.Time{}, glerrors.Wrap(glerrors.KindInternal, "reading the clock", err)
	}
	return now.UTC(), nil
}

// RestoreLevelBatch puts the structure back, parents first, in one
// transaction. The window is judged on the database's clock.
// A batch past its window is refused TRASH_EXPIRED — and the level it froze,
// if still in place, is let go in the same breath (see releaseIfExpired).
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var (
		rep     levelarchive.Report
		expired bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		rec, err := getLevelDeletion(tx, batchID)
		if err != nil {
			return err
		}
		now, err := dbNow(tx)
		if err != nil {
			return err
		}
		if expired, err = releaseIfExpired(tx, rec, now); err != nil || expired {
			return err
		}
		if err := rec.CheckRestorable(now); err != nil {
			return err
		}
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			return restoreStructure(tx, batchID)
		})
		return err
	})
	if err == nil && expired {
		err = domain.TrashExpired()
	}
	return rep, err
}

// releaseIfExpired answers whether a restorable batch's window has closed and,
// when it has, lets go of the level it froze (a failed_partial batch whose
// level config never archived). The caller commits and then refuses with
// TRASH_EXPIRED: the release must outlive the refusal. What the other services
// archived goes with the purge; config's structure never left.
func releaseIfExpired(tx *gorm.DB, rec domain.LevelDeletion, now time.Time) (bool, error) {
	if !rec.Expired(now) || !(rec.Status.InTrash() || rec.Status == domain.LevelDeletionRestoring) {
		return false, nil
	}
	return true, releaseFreeze(tx, rec.LevelID, rec.BatchID)
}

// frozenByOther matches a level none of whose OTHER batches is running or
// failed half-way — the fallback for a freeze no batch owns (pre-0013).
const frozenByOther = `NOT EXISTS (SELECT 1 FROM level_deletions o WHERE o.level_id = levels.id AND o.batch_id <> ?
	AND (o.status IN ('deleting', 'failed_partial') OR o.restoring_from = 'failed_partial'))`

// releaseFreeze puts a level back to active only if batchID owns its freeze
// (compare-and-set on levels.deleting_batch_id): an old batch closing never
// lifts the freeze of a newer deletion. A level gone (archived) is a no-op.
func releaseFreeze(tx *gorm.DB, levelID, batchID string) error {
	err := tx.Exec(`UPDATE levels SET status = ?, deleting_batch_id = NULL, updated_at = now()
		WHERE id = ? AND status = ? AND (deleting_batch_id = ? OR (deleting_batch_id IS NULL AND `+frozenByOther+`))`,
		levelStatusActive, levelID, levelStatusDeleting, batchID, batchID).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "unfreezing the level", err)
	}
	return nil
}

// claimFreeze (re)freezes a level for batchID: an active level, or one whose
// freeze batchID already owns or nobody owns. A freeze another batch holds is
// left alone.
func claimFreeze(tx *gorm.DB, levelID, batchID string) error {
	err := tx.Exec(`UPDATE levels SET status = ?, deleting_batch_id = ?, updated_at = now()
		WHERE id = ? AND (status = ? OR deleting_batch_id IS NULL) AND deleting_batch_id IS DISTINCT FROM ?`,
		levelStatusDeleting, batchID, levelID, levelStatusActive, batchID).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "freezing the level", err)
	}
	return nil
}

// BeginLevelRestore marks a batch `restoring` before a restore's first step —
// compare-and-set from deleted, failed_partial or restored_partial, under the
// batch's row lock, on the database's clock — so the sweep never purges it
// mid-way. A batch already restoring is RESTORE_IN_PROGRESS; one past its
// window is TRASH_EXPIRED (and lets its level go, see releaseIfExpired).
func (r *Repository) BeginLevelRestore(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	var (
		rec     domain.LevelDeletion
		expired bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		cur, err := lockLevelDeletion(tx, batchID)
		if err != nil {
			return err
		}
		now, err := dbNow(tx)
		if err != nil {
			return err
		}
		if cur.Status.InTrash() {
			if expired, err = releaseIfExpired(tx, cur, now); err != nil || expired {
				return err
			}
		}
		if err := cur.CheckRestoreStart(now); err != nil {
			return err
		}
		err = tx.Exec(`UPDATE level_deletions SET restoring_from = status, status = ?, restoring_since = now(), updated_at = now()
			WHERE batch_id = ?`, string(domain.LevelDeletionRestoring), batchID).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "starting the restore", err)
		}
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	if err == nil && expired {
		err = domain.TrashExpired()
	}
	return rec, err
}

// AbandonLevelRestore puts a batch whose restore failed back where it was
// (restoring_from). A batch not restoring is answered as is.
func (r *Repository) AbandonLevelRestore(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Exec(`UPDATE level_deletions SET status = restoring_from, restoring_from = NULL, restoring_since = NULL,
				updated_at = now()
			WHERE batch_id = ? AND status = ?`, batchID, string(domain.LevelDeletionRestoring)).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "abandoning the restore", err)
		}
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	return rec, err
}

// lockLevelDeletion reads a batch under its row lock.
func lockLevelDeletion(tx *gorm.DB, batchID string) (domain.LevelDeletion, error) {
	var m levelDeletionModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("batch_id = ?", batchID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.LevelDeletion{}, glerrors.ErrNotFound
	}
	if err != nil {
		return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "reading the deletion", err)
	}
	return toDomainLevelDeletion(m)
}

func restoreStructure(tx *gorm.DB, batchID string) (levelarchive.Report, error) {
	rep := levelarchive.NewReport()
	// The level was filed while its deletion held it (status deleting); it comes
	// back writable. Rows filed before migration 0012 had no status at all.
	err := tx.Exec(`UPDATE `+levelTrash.Table()+` SET "row" = "row" || jsonb_build_object('status', ?::text, 'deleting_batch_id', NULL)
		WHERE batch_id = ? AND table_name = 'levels'`, levelStatusActive, batchID).Error
	if err != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "unfreezing the archived level", err)
	}
	restored, skipped, err := levelTrash.Reinsert(tx, batchID, "levels")
	if err != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "restoring the level", err)
	}
	if skipped > 0 {
		// The only unique key a level has is its name: somebody created a level
		// with it while this one sat in the trash. Everything below hangs off the
		// level, so nothing comes back until the director renames one of them.
		return rep, domain.LevelNameTaken()
	}
	rep.Record(kindLevels, restored, 0, "")
	for _, s := range levelRestore {
		restored, skipped, err := levelTrash.Reinsert(tx, batchID, s.table, s.parents...)
		if err != nil {
			return rep, glerrors.Wrap(glerrors.KindInternal, "restoring the level's structure", err)
		}
		rep.Record(s.kind, restored, skipped, skipReason)
	}
	return rep, nil
}

// MarkLevelDeletionRestored closes a batch every service has restored:
// restored, or restored_partial when some rows did not fit (it stays in the
// trash for a retry). Either way the level is writable again — unless a newer
// deletion owns its freeze.
func (r *Repository) MarkLevelDeletionRestored(ctx context.Context, batchID string, partial bool) (domain.LevelDeletion, error) {
	to := domain.LevelDeletionRestored
	if partial {
		to = domain.LevelDeletionRestoredPartial
	}
	return r.moveDeletion(ctx, batchID, to, levelStatusActive, nil, domain.LevelDeletionRestoring,
		domain.LevelDeletionDeleted, domain.LevelDeletionFailedPartial, domain.LevelDeletionRestoredPartial)
}

// CancelLevelDeletion closes a saga whose compensation fully succeeded: nothing
// was deleted, and the level (if it exists) is writable again.
func (r *Repository) CancelLevelDeletion(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	return r.moveDeletion(ctx, batchID, domain.LevelDeletionCancelled, levelStatusActive, nil,
		domain.LevelDeletionDeleting, domain.LevelDeletionDeleted)
}

// FailLevelDeletion records a saga its compensation could not fully undo: in
// the trash as failed_partial, naming the pending steps and what was skipped.
// The level (if it exists) stays frozen until the batch is restored.
func (r *Repository) FailLevelDeletion(ctx context.Context, batchID string, pending []string, skipped []levelarchive.Skip) (domain.LevelDeletion, error) {
	if skipped == nil {
		skipped = []levelarchive.Skip{}
	}
	body, err := json.Marshal(skipped)
	if err != nil {
		return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "encoding a deletion's skips", err)
	}
	extra := map[string]any{"pending_steps": gorm.Expr("?::text[]", textArray(pending)), "skipped": gorm.Expr("?::jsonb", string(body))}
	return r.moveDeletion(ctx, batchID, domain.LevelDeletionFailedPartial, levelStatusDeleting, extra,
		domain.LevelDeletionDeleting, domain.LevelDeletionDeleted, domain.LevelDeletionFailedPartial)
}

// MarkLevelDeletionPurged closes a batch every service has purged. A batch
// still in the trash becomes purged; one already restored or cancelled keeps
// its outcome. Both leave the sweep. A level the batch still froze (a
// failed_partial whose config structure never left) is let go: what the other
// services archived is gone, the structure stays.
func (r *Repository) MarkLevelDeletionPurged(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		cur, err := getLevelDeletion(tx, batchID)
		if err != nil {
			return err
		}
		if err := releaseFreeze(tx, cur.LevelID, batchID); err != nil {
			return err
		}
		err = tx.Exec(`UPDATE level_deletions SET purged_at = now(), updated_at = now(),
				status = CASE WHEN status IN ?  THEN ? ELSE status END
			WHERE batch_id = ? AND purged_at IS NULL`,
			[]string{string(domain.LevelDeletionDeleted), string(domain.LevelDeletionFailedPartial), string(domain.LevelDeletionRestoredPartial)},
			string(domain.LevelDeletionPurged), batchID).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "closing a purged deletion", err)
		}
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	return rec, err
}

// SweepLevelDeletions first turns every batch stuck in `deleting` for longer
// than stalledAfter (a gateway that died mid-saga) into failed_partial with
// every step pending — restorable from the trash — and every batch stuck in
// `restoring` longer than domain.RestoreStallAfter back to where its restore
// began; then lists the batches past restorable_until whose archives were not
// purged yet, never one running (deleting or restoring). Database clock.
func (r *Repository) SweepLevelDeletions(ctx context.Context, stalledAfter time.Duration) ([]domain.LevelDeletion, int64, error) {
	var (
		out     []domain.LevelDeletion
		stalled int64
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE level_deletions SET status = ?, pending_steps = ?, updated_at = now()
			WHERE status = ? AND deleted_at < now() - make_interval(secs => ?)`,
			string(domain.LevelDeletionFailedPartial), textArray(domain.LevelDeletionSteps),
			string(domain.LevelDeletionDeleting), stalledAfter.Seconds())
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "unsticking dead deletions", res.Error)
		}
		stalled = res.RowsAffected
		err := tx.Exec(`UPDATE level_deletions SET status = restoring_from, restoring_from = NULL, restoring_since = NULL,
				updated_at = now()
			WHERE status = ? AND restoring_since < now() - make_interval(secs => ?)`,
			string(domain.LevelDeletionRestoring), domain.RestoreStallAfter.Seconds()).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "unsticking dead restores", err)
		}
		var rows []levelDeletionModel
		running := []string{string(domain.LevelDeletionDeleting), string(domain.LevelDeletionRestoring)}
		err = tx.Where("purged_at IS NULL AND status NOT IN ? AND restorable_until < now()", running).
			Order("restorable_until").Limit(maxListRows).Find(&rows).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing expired deletions", err)
		}
		out, err = toDomainLevelDeletions(rows)
		return err
	})
	return out, stalled, err
}

// PurgeLevelBatch drops config's archive of one batch.
func (r *Repository) PurgeLevelBatch(ctx context.Context, batchID string) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.PurgeBatch(tx, batchID); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging a batch of the trash", err)
		}
		return nil
	})
	return n, err
}

// moveDeletion moves a batch to status `to` when it is in one of `from`, sets
// the level's status (if the level exists) — compare-and-set on the batch that
// owns the freeze (releaseFreeze / claimFreeze) — and answers the record. A batch in
// no `from` state is left as is and answered (a retry of the same move is
// harmless; the caller reads the status).
func (r *Repository) moveDeletion(ctx context.Context, batchID string, to domain.LevelDeletionStatus, levelStatus string,
	extra map[string]any, from ...domain.LevelDeletionStatus) (domain.LevelDeletion, error) {
	lifecycle := releaseFreeze
	if levelStatus == levelStatusDeleting {
		lifecycle = claimFreeze
	}
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		cur, err := getLevelDeletion(tx, batchID)
		if err != nil {
			return err
		}
		if !statusIn(cur.Status, from) {
			rec = cur
			return nil
		}
		set := map[string]any{"status": string(to), "updated_at": gorm.Expr("now()"),
			"restoring_from": nil, "restoring_since": nil}
		for k, v := range extra {
			set[k] = v
		}
		// By table, not model: pending_steps and skipped are read-only on the model.
		if err := tx.Table("level_deletions").Where("batch_id = ?", batchID).Updates(set).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "moving the deletion", err)
		}
		if err := lifecycle(tx, cur.LevelID, batchID); err != nil {
			return err
		}
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	return rec, err
}

func statusIn(s domain.LevelDeletionStatus, set []domain.LevelDeletionStatus) bool {
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}

// textArray renders fixed identifiers as a Postgres text[] literal.
func textArray(items []string) string { return "{" + strings.Join(items, ",") + "}" }

func toDomainLevelDeletions(rows []levelDeletionModel) ([]domain.LevelDeletion, error) {
	out := make([]domain.LevelDeletion, 0, len(rows))
	for _, m := range rows {
		d, err := toDomainLevelDeletion(m)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ListLevelDeletions is the trash: deleted, failed_partial and
// restored_partial batches — and any being restored right now — newest first.
func (r *Repository) ListLevelDeletions(ctx context.Context) ([]domain.LevelDeletion, error) {
	var out []domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []levelDeletionModel
		inTrash := []string{string(domain.LevelDeletionDeleted), string(domain.LevelDeletionFailedPartial),
			string(domain.LevelDeletionRestoredPartial), string(domain.LevelDeletionRestoring)}
		if err := tx.Where("status IN ?", inTrash).
			Order("deleted_at DESC").Limit(maxListRows).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the trash", err)
		}
		var err error
		out, err = toDomainLevelDeletions(rows)
		return err
	})
	return out, err
}

// GetLevelDeletion fetches one batch, or KindNotFound.
func (r *Repository) GetLevelDeletion(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	return rec, err
}

func getLevelDeletion(tx *gorm.DB, batchID string) (domain.LevelDeletion, error) {
	var m levelDeletionModel
	if err := tx.Where("batch_id = ?", batchID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.LevelDeletion{}, glerrors.ErrNotFound
		}
		return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "reading the deletion", err)
	}
	return toDomainLevelDeletion(m)
}

// PurgeLevelArchives drops archives filed before cutoff and marks every batch
// deleted before it purged, in one transaction.
func (r *Repository) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the trash", err)
		}
		err = tx.Model(&levelDeletionModel{}).
			Where("status = ? AND deleted_at < ?", string(domain.LevelDeletionDeleted), cutoff).
			Updates(map[string]any{"status": string(domain.LevelDeletionPurged), "updated_at": gorm.Expr("now()")}).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "closing purged deletions", err)
		}
		return nil
	})
	return n, err
}
