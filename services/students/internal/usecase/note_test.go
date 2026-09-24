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
	byID     map[string]domain.Note
	students map[string]bool
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
