//go:build integration

package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// RevisionsOf reads the texts a note's edits replaced, newest first, so the
// integration tests can check the trail an edit leaves. Nothing in production
// reads revisions yet (see migrations/students/0018).
func (r *Repository) RevisionsOf(ctx context.Context, noteID string) ([]domain.NoteRevision, error) {
	var out []domain.NoteRevision
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []noteRevisionModel
		if err := tx.Where("note_id = ?", noteID).Order("edited_at DESC, id DESC").Find(&rows).Error; err != nil {
			return err
		}
		for _, m := range rows {
			out = append(out, domain.NoteRevision{
				NoteID: m.NoteID, PreviousBody: m.PreviousBody,
				EditedBy: domain.NoteAuthor{UserID: m.EditedBy, Name: m.EditedByName}, EditedAt: m.EditedAt,
			})
		}
		return nil
	})
	return out, err
}

// ExecAsTenant runs raw SQL as the app role inside this tenant's context.
func (r *Repository) ExecAsTenant(ctx context.Context, sql string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error { return tx.Exec(sql).Error })
}
