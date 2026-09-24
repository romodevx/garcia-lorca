package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// studentFinder is the one thing notes need from the pupils' repository.
type studentFinder interface {
	FindByID(ctx context.Context, id string) (*domain.Student, error)
}

// NoteService writes, lists and voids the notes staff keep about a pupil.
// Who wrote a note is always the caller the gateway authenticated, never
// something a request body says.
type NoteService struct {
	students studentFinder
	notes    ports.NoteRepository
	tenant   string
	newID    func() string
	now      func() time.Time
}

// NewNoteService wires the use case.
func NewNoteService(students studentFinder, notes ports.NoteRepository, tenant string, newID func() string, now func() time.Time) *NoteService {
	return &NoteService{students: students, notes: notes, tenant: tenant, newID: newID, now: now}
}

// AddNote writes a note about a pupil, by `author`, now.
func (s *NoteService) AddNote(ctx context.Context, studentID, body string, author domain.NoteAuthor) (domain.Note, error) {
	if _, err := s.students.FindByID(ctx, studentID); err != nil {
		return domain.Note{}, err
	}
	note, err := domain.NewNote(s.newID(), s.tenant, studentID, body, author, s.now())
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.notes.AddNote(ctx, note); err != nil {
		return domain.Note{}, err
	}
	return note, nil
}

// ListNotes returns a pupil's notes, newest first. Voided notes are left out
// unless includeVoided (whoever may void any note sees them, marked).
func (s *NoteService) ListNotes(ctx context.Context, studentID string, includeVoided bool) ([]domain.Note, error) {
	if _, err := s.students.FindByID(ctx, studentID); err != nil {
		return nil, err
	}
	all, err := s.notes.NotesOf(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if includeVoided {
		return all, nil
	}
	kept := make([]domain.Note, 0, len(all))
	for _, n := range all {
		if !n.IsVoided() {
			kept = append(kept, n)
		}
	}
	return kept, nil
}

// VoidNote withdraws a note of `studentID`. The rules (the author within the
// window, anyone else only with voidsAny) are the domain's. A note of another
// pupil is KindNotFound, which never confirms it exists.
func (s *NoteService) VoidNote(ctx context.Context, studentID, noteID, by string, voidsAny bool) (domain.Note, error) {
	note, err := s.notes.NoteByID(ctx, noteID)
	if err != nil {
		return domain.Note{}, err
	}
	if note.StudentID() != studentID {
		return domain.Note{}, glerrors.ErrNotFound
	}
	voided, err := note.Void(by, s.now(), voidsAny)
	if err != nil {
		return domain.Note{}, err
	}
	if err := s.notes.VoidNote(ctx, voided); err != nil {
		return domain.Note{}, err
	}
	return voided, nil
}
