package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// memNotes behaves as the repository does: a note on a pupil it does not know
// is KindNotFound (that check is the repository's, not the use case's).
type memNotes struct {
	byID      map[string]domain.Note
	students  map[string]bool
	revisions []domain.NoteRevision
}

func (m *memNotes) AddNote(_ context.Context, n domain.Note) error {
	if !m.students[n.StudentID()] {
		return glerrors.ErrNotFound
	}
	m.byID[n.ID()] = n
	return nil
}

func (m *memNotes) NotesOf(_ context.Context, studentID string) ([]domain.Note, error) {
	var out []domain.Note
	for _, n := range m.byID {
		if n.StudentID() == studentID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt().After(out[j].CreatedAt()) })
	return out, nil
}

func (m *memNotes) NoteByID(_ context.Context, id string) (domain.Note, error) {
	n, ok := m.byID[id]
	if !ok {
		return domain.Note{}, glerrors.ErrNotFound
	}
	return n, nil
}

func (m *memNotes) VoidNote(_ context.Context, n domain.Note) error {
	m.byID[n.ID()] = n
	return nil
}

// EditNote keeps the revision, as the repository does in the same transaction.
func (m *memNotes) EditNote(_ context.Context, n domain.Note, rev domain.NoteRevision) error {
	m.byID[n.ID()] = n
	m.revisions = append(m.revisions, rev)
	return nil
}

type noteFixture struct {
	svc     *usecase.NoteService
	notes   *memNotes
	clock   time.Time
	student string
}

func newNoteFixture(t *testing.T) *noteFixture {
	t.Helper()
	students, repo, _ := newService(t)
	created, err := students.CreateApplicant(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	f := &noteFixture{notes: &memNotes{byID: map[string]domain.Note{}, students: map[string]bool{created.ID(): true}}, clock: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC), student: created.ID()}
	ids := 0
	f.svc = usecase.NewNoteService(repo, f.notes, tenant,
		func() string { ids++; return fmt.Sprintf("b6000000-0000-0000-0000-%012d", ids) },
		func() time.Time { return f.clock })
	return f
}

var gabriela = domain.NoteAuthor{UserID: "u-teacher", Name: "Gabriela Rojas"}

func TestANoteIsWrittenByWhoeverIsSignedIn(t *testing.T) {
	f := newNoteFixture(t)
	n, err := f.svc.AddNote(context.Background(), f.student, "Llegó con fiebre", gabriela)
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if n.Author() != gabriela || !n.CreatedAt().Equal(f.clock) || n.StudentID() != f.student {
		t.Errorf("note = %+v", n)
	}
}

func TestANoteOnAnUnknownPupilIsNotFound(t *testing.T) {
	f := newNoteFixture(t)
	if _, err := f.svc.AddNote(context.Background(), "nope", "x", gabriela); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

// The ordinary history hides voided notes; whoever may void any sees them,
// marked, so a withdrawn note can still be accounted for.
func TestTheHistoryHidesVoidedNotesExceptFromWhoeverVoidsAny(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	kept, _ := f.svc.AddNote(ctx, f.student, "Se queda", gabriela)
	f.clock = f.clock.Add(time.Minute)
	gone, _ := f.svc.AddNote(ctx, f.student, "Se anula", gabriela)
	if _, err := f.svc.VoidNote(ctx, f.student, gone.ID(), gabriela.UserID, false); err != nil {
		t.Fatalf("VoidNote: %v", err)
	}

	visible, err := f.svc.ListNotes(ctx, f.student, false)
	if err != nil || len(visible) != 1 || visible[0].ID() != kept.ID() {
		t.Errorf("ordinary history = %v, %v", visible, err)
	}
	all, err := f.svc.ListNotes(ctx, f.student, true)
	if err != nil || len(all) != 2 || !all[0].IsVoided() {
		t.Errorf("full history = %v, %v: want both, newest (voided) first", all, err)
	}
}

func TestOnlyTheAuthorVoidsWithinTheWindow(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "x", gabriela)

	if _, err := f.svc.VoidNote(ctx, f.student, n.ID(), "u-other", false); glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Errorf("someone else: want KindForbidden, got %v", err)
	}
	f.clock = f.clock.Add(16 * time.Minute)
	if _, err := f.svc.VoidNote(ctx, f.student, n.ID(), gabriela.UserID, false); glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Errorf("the author after 15 min: want KindForbidden, got %v", err)
	}
	if _, err := f.svc.VoidNote(ctx, f.student, n.ID(), "u-owner", true); err != nil {
		t.Errorf("whoever voids any: %v", err)
	}
}

// A note is addressed through its pupil: a note of another pupil is 404.
func TestANoteOfAnotherPupilIsNotFound(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "x", gabriela)
	if _, err := f.svc.VoidNote(ctx, "another-pupil", n.ID(), gabriela.UserID, true); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

// A note id is a uuid. Anything else is a note that does not exist (404),
// never a query error surfacing as 500.
func TestANoteIdThatIsNotAnIdIsNotFound(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	if _, err := f.svc.VoidNote(ctx, f.student, "abc", gabriela.UserID, true); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("VoidNote(abc): want KindNotFound, got %v", err)
	}
}

func TestAnEditReplacesTheTextAndKeepsTheOldOne(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "Llegó con fiebre", gabriela)
	f.clock = f.clock.Add(48 * time.Hour)

	edited, err := f.svc.EditNote(ctx, f.student, n.ID(), "Llegó con fiebre de 38", "Llegó con fiebre", gabriela, false)
	if err != nil {
		t.Fatalf("EditNote: %v", err)
	}
	if edited.Body() != "Llegó con fiebre de 38" || !edited.UpdatedAt().Equal(f.clock) || edited.UpdatedBy() != gabriela {
		t.Errorf("edited = %+v", edited)
	}
	if len(f.notes.revisions) != 1 || f.notes.revisions[0].PreviousBody != "Llegó con fiebre" {
		t.Errorf("revisions = %+v", f.notes.revisions)
	}
	listed, _ := f.svc.ListNotes(ctx, f.student, false)
	if len(listed) != 1 || listed[0].Body() != "Llegó con fiebre de 38" {
		t.Errorf("listed = %+v", listed)
	}
}

func TestOnlyTheAuthorOrWhoeverEditsAnyEditsANote(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "x", gabriela)
	other := domain.NoteAuthor{UserID: "u-other", Name: "Otro"}
	if _, err := f.svc.EditNote(ctx, f.student, n.ID(), "y", "", other, false); glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Errorf("someone else: want KindForbidden, got %v", err)
	}
	if _, err := f.svc.EditNote(ctx, f.student, n.ID(), "y", "", domain.NoteAuthor{UserID: "u-owner"}, true); err != nil {
		t.Errorf("whoever edits any: %v", err)
	}
}

func TestEditingANoteOfAnotherPupilOrNoNoteIsNotFound(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "x", gabriela)
	if _, err := f.svc.EditNote(ctx, "another-pupil", n.ID(), "y", "", gabriela, true); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("another pupil: want KindNotFound, got %v", err)
	}
	if _, err := f.svc.EditNote(ctx, f.student, "abc", "y", "", gabriela, true); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("no such note: want KindNotFound, got %v", err)
	}
	if len(f.notes.revisions) != 0 {
		t.Errorf("a refused edit kept a revision: %+v", f.notes.revisions)
	}
}

// A stale page — the note no longer says what the editor started from — is
// refused instead of overwriting someone else's edit.
func TestAStaleEditIsAConflict(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "Fiebre", gabriela)
	if _, err := f.svc.EditNote(ctx, f.student, n.ID(), "Fiebre de 38", "Fiebre", domain.NoteAuthor{UserID: "u-owner"}, true); err != nil {
		t.Fatalf("first edit: %v", err)
	}
	_, err := f.svc.EditNote(ctx, f.student, n.ID(), "Fiebre de 39", "Fiebre", gabriela, false)
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("stale edit: want KindConflict, got %v", err)
	}
	if len(f.notes.revisions) != 1 {
		t.Errorf("a stale edit kept a revision: %+v", f.notes.revisions)
	}
}

// A voided note is hidden from whoever cannot void any: editing it is 404 for
// them, never a 409 that confirms it exists.
func TestEditingAVoidedNoteIsNotFoundForWhoeverCannotSeeIt(t *testing.T) {
	f := newNoteFixture(t)
	ctx := context.Background()
	n, _ := f.svc.AddNote(ctx, f.student, "x", gabriela)
	if _, err := f.svc.VoidNote(ctx, f.student, n.ID(), gabriela.UserID, false); err != nil {
		t.Fatalf("VoidNote: %v", err)
	}
	if _, err := f.svc.EditNote(ctx, f.student, n.ID(), "y", "", gabriela, false); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("writer: want KindNotFound, got %v", err)
	}
	if _, err := f.svc.EditNote(ctx, f.student, n.ID(), "y", "", domain.NoteAuthor{UserID: "u-owner"}, true); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("owner: want KindConflict, got %v", err)
	}
}
