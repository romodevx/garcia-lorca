package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

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
}

func (levelDeletionModel) TableName() string { return "level_deletions" }

func toDomainLevelDeletion(m levelDeletionModel) (domain.LevelDeletion, error) {
	counts := levelarchive.Counts{}
	if err := json.Unmarshal([]byte(m.Counts), &counts); err != nil {
		return domain.LevelDeletion{}, glerrors.Wrap(glerrors.KindInternal, "decoding a deletion's counts", err)
	}
	return domain.LevelDeletion{
		BatchID: m.BatchID, LevelID: m.LevelID, LevelName: m.LevelName, Counts: counts,
		DeletedBy: m.DeletedBy, DeletedByName: m.DeletedByName,
		DeletedAt: m.DeletedAt.UTC(), RestorableUntil: m.RestorableUntil.UTC(),
		Status: domain.LevelDeletionStatus(m.Status),
	}, nil
}

func fromDomainLevelDeletion(tenantID string, d domain.LevelDeletion) (levelDeletionModel, error) {
	counts, err := json.Marshal(d.Counts)
	if err != nil {
		return levelDeletionModel{}, glerrors.Wrap(glerrors.KindInternal, "encoding a deletion's counts", err)
	}
	return levelDeletionModel{
		BatchID: d.BatchID, TenantID: tenantID, LevelID: d.LevelID, LevelName: d.LevelName,
		Counts: string(counts), DeletedBy: d.DeletedBy, DeletedByName: d.DeletedByName,
		DeletedAt: d.DeletedAt, RestorableUntil: d.RestorableUntil, Status: string(d.Status),
	}, nil
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
		out = ports.LevelStructure{LevelID: level.ID, LevelName: level.Name, Counts: levelarchive.Counts{}}
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

// ArchiveLevel moves the structure into the trash and records the batch, in
// one transaction. A retried batch answers its recorded counts and ledger row.
func (r *Repository) ArchiveLevel(ctx context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	var (
		counts levelarchive.Counts
		rec    domain.LevelDeletion
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, in.BatchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			return r.archiveLevel(tx, in)
		})
		if err != nil {
			return err
		}
		rec, err = getLevelDeletion(tx, in.BatchID)
		return err
	})
	return counts, rec, err
}

func (r *Repository) archiveLevel(tx *gorm.DB, in ports.ArchiveLevelInput) (levelarchive.Counts, error) {
	var level levelModel
	if err := tx.Where("id = ?", in.LevelID).First(&level).Error; err != nil {
		return nil, translateFindErr(err)
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
	rec, err := domain.NewLevelDeletion(in.BatchID, in.LevelID, level.Name, in.DeletedBy, in.DeletedByName, total, in.At)
	if err != nil {
		return nil, err
	}
	model, err := fromDomainLevelDeletion(r.tenantID, rec)
	if err != nil {
		return nil, err
	}
	if err := tx.Create(&model).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "recording the deletion", err)
	}
	return counts, nil
}

// RestoreLevelBatch puts the structure back, parents first, in one
// transaction.
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string, now time.Time) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		rec, err := getLevelDeletion(tx, batchID)
		if err != nil {
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
	return rep, err
}

func restoreStructure(tx *gorm.DB, batchID string) (levelarchive.Report, error) {
	rep := levelarchive.NewReport()
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

// MarkLevelDeletionRestored closes a batch every service has restored.
func (r *Repository) MarkLevelDeletionRestored(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	var rec domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&levelDeletionModel{}).
			Where("batch_id = ? AND status = ?", batchID, string(domain.LevelDeletionDeleted)).
			Updates(map[string]any{"status": string(domain.LevelDeletionRestored), "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "closing the deletion", res.Error)
		}
		var err error
		rec, err = getLevelDeletion(tx, batchID)
		return err
	})
	return rec, err
}

// ListLevelDeletions is the trash: batches still deleted, newest first.
func (r *Repository) ListLevelDeletions(ctx context.Context) ([]domain.LevelDeletion, error) {
	var out []domain.LevelDeletion
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []levelDeletionModel
		if err := tx.Where("status = ?", string(domain.LevelDeletionDeleted)).
			Order("deleted_at DESC").Limit(maxListRows).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the trash", err)
		}
		out = make([]domain.LevelDeletion, 0, len(rows))
		for _, m := range rows {
			d, err := toDomainLevelDeletion(m)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return nil
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
