//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// A pupil's notes come back newest first; a voided one keeps its mark; another
// tenant can neither read nor write them.
func TestNotesRoundTripNewestFirstAndAreTenantIsolated(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const studentID = "aaaaaaaa-0000-0000-0000-0000000006a1"
	if err := repo.Create(ctx, applicant(t, studentID, tenantA, "NOTA050312MDFRNN01", "1° A")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	at := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	who := domain.NoteAuthor{UserID: "60000000-0000-0000-0000-0000000006a1", Name: "Gabriela Rojas"}
	first, _ := domain.NewNote("b6000000-0000-0000-0000-000000000001", tenantA, studentID, "Primera", who, at)
	second, _ := domain.NewNote("b6000000-0000-0000-0000-000000000002", tenantA, studentID, "Segunda", who, at.Add(time.Hour))
	for _, n := range []domain.Note{first, second} {
		if err := repo.AddNote(ctx, n); err != nil {
			t.Fatalf("AddNote: %v", err)
		}
	}

	got, err := repo.NotesOf(ctx, studentID)
	if err != nil || len(got) != 2 || got[0].Body() != "Segunda" || got[1].Body() != "Primera" {
		t.Fatalf("NotesOf = %v, %v: want newest first", got, err)
	}
	if got[0].Author() != who || !got[0].CreatedAt().Equal(at.Add(time.Hour)) {
		t.Errorf("stored note = %+v", got[0])
	}

	voided, err := first.Void(who.UserID, at.Add(time.Minute), false)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.VoidNote(ctx, voided); err != nil {
		t.Fatalf("VoidNote: %v", err)
	}
	// A second void of the same row (a stale page) changes nothing and says so.
	if err := repo.VoidNote(ctx, voided); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second VoidNote: want KindConflict, got %v", err)
	}
	stored, err := repo.NoteByID(ctx, first.ID())
	if err != nil || !stored.IsVoided() || stored.VoidedBy() != who.UserID {
		t.Errorf("NoteByID = %+v, %v: want voided by the author", stored, err)
	}

	if notes, err := appRepo(t, tenantB).NotesOf(ctx, studentID); err != nil || len(notes) != 0 {
		t.Errorf("tenant B read tenant A's notes (RLS read breach): %v %v", notes, err)
	}
	if _, err := appRepo(t, tenantB).NoteByID(ctx, first.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B read a note by id: %v", err)
	}
	evil, _ := domain.NewNote("b6000000-0000-0000-0000-000000000003", tenantB, studentID, "x", who, at)
	if err := appRepo(t, tenantB).AddNote(ctx, evil); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B wrote a note on tenant A's pupil (RLS write breach): %v", err)
	}
}

// A path value that is not a uuid is a note that does not exist, not a query
// error (22P02) answered as 500.
func TestANoteIdThatIsNotAUUIDIsNotFound(t *testing.T) {
	for _, id := range []string{"abc", "urn:uuid:b6000000-0000-0000-0000-00000000ffff"} {
		if _, err := appRepo(t, tenantA).NoteByID(context.Background(), id); glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("NoteByID(%s): want KindNotFound, got %v", id, err)
		}
	}
}
