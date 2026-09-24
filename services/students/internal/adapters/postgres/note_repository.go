package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// maxNotesListed bounds one pupil's history read, as every list read here is.
const maxNotesListed = 500

// AddNote stores a new note. A pupil this tenant cannot see is KindNotFound:
// an FK check does not pass through RLS, so the visibility check is what stops
// a note being written against another tenant's pupil.
func (r *Repository) AddNote(ctx context.Context, n domain.Note) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var visible int64
		if err := tx.Model(&studentModel{}).Where("id = ?", n.StudentID()).Count(&visible).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking the pupil", err)
		}
		if visible == 0 {
			return glerrors.ErrNotFound
		}
		row := noteModel{
			ID: n.ID(), TenantID: r.tenantID, StudentID: n.StudentID(), Body: n.Body(),
			AuthorUserID: n.Author().UserID, AuthorName: n.Author().Name, CreatedAt: n.CreatedAt(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving the note", err)
		}
		return nil
	})
}

// NotesOf returns a pupil's notes, voided ones included, newest first.
func (r *Repository) NotesOf(ctx context.Context, studentID string) ([]domain.Note, error) {
	var out []domain.Note
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []noteModel
		if err := tx.Where("student_id = ?", studentID).
			Order("created_at DESC, id DESC").Limit(maxNotesListed).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing notes", err)
		}
		out = make([]domain.Note, 0, len(rows))
		for _, m := range rows {
			out = append(out, noteToDomain(m))
		}
		return nil
	})
	return out, err
}

// NoteByID returns one note, or KindNotFound — also for an id that is not a
// uuid, which could not name a note and would otherwise fail the query.
func (r *Repository) NoteByID(ctx context.Context, id string) (domain.Note, error) {
	parsed, parseErr := uuid.Parse(id)
	if parseErr != nil {
		return domain.Note{}, glerrors.ErrNotFound
	}
	var out domain.Note
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m noteModel
		// The canonical form: uuid.Parse also takes "urn:uuid:…", which
		// Postgres does not.
		err := tx.Where("id = ?", parsed.String()).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return glerrors.ErrNotFound
		}
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the note", err)
		}
		out = noteToDomain(m)
		return nil
	})
	return out, err
}

// VoidNote records a note's void. `voided_at IS NULL` rides in the WHERE: two
// people voiding at once, or a stale page, void it once, and the second is told
// (KindConflict) rather than overwriting who voided it.
func (r *Repository) VoidNote(ctx context.Context, n domain.Note) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&noteModel{}).Where("id = ? AND voided_at IS NULL", n.ID()).
			Updates(map[string]any{"voided_at": n.VoidedAt(), "voided_by": n.VoidedBy()})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "voiding the note", res.Error)
		}
		if res.RowsAffected == 1 {
			return nil
		}
		var exists int64
		if err := tx.Model(&noteModel{}).Where("id = ?", n.ID()).Count(&exists).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the note", err)
		}
		if exists == 0 {
			return glerrors.ErrNotFound
		}
		return glerrors.New(glerrors.KindConflict, "this note was already voided")
	})
}

func noteToDomain(m noteModel) domain.Note {
	var voidedAt time.Time
	if m.VoidedAt != nil {
		voidedAt = *m.VoidedAt
	}
	voidedBy := ""
	if m.VoidedBy != nil {
		voidedBy = *m.VoidedBy
	}
	return domain.RestoreNote(m.ID, m.TenantID, m.StudentID, m.Body,
		domain.NoteAuthor{UserID: m.AuthorUserID, Name: m.AuthorName}, m.CreatedAt, voidedAt, voidedBy)
}
