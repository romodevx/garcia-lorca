package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// Compile-time proof the adapter satisfies the level-archive port.
var _ ports.LevelArchiveRepository = (*Repository)(nil)

// levelTrash is uniforms' half of the deletion archive (migration 0010).
var levelTrash = gormarchive.New("uniforms_deletion_archive")

const (
	// kindItems is the one kind uniforms reports (ADR 0021).
	kindItems = "uniformItems"
	// itemLevelSnapshot names the archived level of each garment.
	itemLevelSnapshot = "uniform_items#level"
	// deletedLevelKey is the legacy key a garment of a deleted level carries.
	// A NULL level_id beside a non-empty legacy key is the existing
	// "unresolved" state (domain.LevelUnresolved, ADR 0014): the garment is
	// kept, offered to nobody and shown with no level until staff re-scope it.
	// Saving any real level clears the key, which is how a restore tells a
	// garment still waiting for its level from one staff already moved.
	deletedLevelKey = "nivel-eliminado"
	// itemSkipReason is the sentence a garment that was re-scoped gets.
	itemSkipReason = "la prenda ya se reasignó a otro nivel"
)

// itemsOfLevel is the garments scoped to the level.
func itemsOfLevel(levelID string) gormarchive.Step {
	return gormarchive.Step{Kind: kindItems, Table: "uniform_items", Where: "level_id = ?", Args: []any{levelID}}
}

// PreviewLevelDeletion counts the garments scoped to the level, changing
// nothing.
func (r *Repository) PreviewLevelDeletion(ctx context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = gormarchive.Count(tx, itemsOfLevel(s.LevelID)); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting the level's garments", err)
		}
		return nil
	})
	return levelarchive.Counts{kindItems: n}, err
}

// ArchiveLevel snapshots each garment's level and leaves it unresolved, once
// per batch. No garment is deleted.
func (r *Repository) ArchiveLevel(ctx context.Context, batchID string, s levelarchive.Scope) (levelarchive.Counts, error) {
	var counts levelarchive.Counts
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			if _, err := levelTrash.Snapshot(tx, batchID, 0, itemLevelSnapshot,
				"SELECT id, level_id, level FROM uniform_items WHERE level_id = ?", s.LevelID); err != nil {
				return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the garments' level", err)
			}
			res := tx.Exec("UPDATE uniform_items SET level_id = NULL, level = ?, updated_at = now() WHERE level_id = ?",
				deletedLevelKey, s.LevelID)
			if res.Error != nil {
				return nil, glerrors.Wrap(glerrors.KindInternal, "unscoping the level's garments", res.Error)
			}
			return levelarchive.Counts{kindItems: res.RowsAffected}, nil
		})
		return err
	})
	return counts, err
}

// RestoreLevelBatch gives each garment its level back — only while it still
// waits unresolved; one staff re-scoped meanwhile keeps their choice and is
// reported as skipped. "Keep the new value" is final, not a clash to retry, so
// the snapshot leaves the trash once applied: a retry answers the first
// report and re-applies nothing.
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			out := levelarchive.NewReport()
			total, err := levelTrash.Archived(tx, batchID, itemLevelSnapshot)
			if err != nil {
				return out, glerrors.Wrap(glerrors.KindInternal, "reading the garments' archive", err)
			}
			res := tx.Exec(`UPDATE uniform_items i
				   SET level_id = (a."row"->>'level_id')::uuid, level = a."row"->>'level', updated_at = now()
				  FROM `+levelTrash.Table()+` a
				 WHERE a.batch_id = ? AND a.table_name = ?
				   AND i.id = (a."row"->>'id')::uuid
				   AND i.level_id IS NULL AND i.level = ?`, batchID, itemLevelSnapshot, deletedLevelKey)
			if res.Error != nil {
				return out, glerrors.Wrap(glerrors.KindInternal, "restoring the garments' level", res.Error)
			}
			out.Record(kindItems, res.RowsAffected, total-res.RowsAffected, itemSkipReason)
			if err := levelTrash.Clear(tx, batchID, itemLevelSnapshot); err != nil {
				return out, glerrors.Wrap(glerrors.KindInternal, "clearing the garments' applied snapshot", err)
			}
			return out, nil
		})
		return err
	})
	return rep, err
}

// PurgeLevelBatch drops one batch's archive, marker included.
func (r *Repository) PurgeLevelBatch(ctx context.Context, batchID string) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.PurgeBatch(tx, batchID); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging a batch of the uniforms trash", err)
		}
		return nil
	})
	return n, err
}

// PurgeLevelArchives drops every archive row filed before cutoff.
func (r *Repository) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the uniforms trash", err)
		}
		return nil
	})
	return n, err
}
