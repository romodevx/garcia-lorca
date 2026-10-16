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
	if err := repo.Create(ctx, applicant(t, studentID, tenantA, "NOTA050312MDFRNN01", "1° A"), testNumberYear); err != nil {
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

// An edit replaces the text and records who and when; the replaced text is
// kept as a revision; a stale edit (the note changed meanwhile) or an edit of a
// voided note is KindConflict; another tenant cannot edit nor read revisions.
func TestANoteEditKeepsItsRevisionAndIsTenantIsolated(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const studentID = "aaaaaaaa-0000-0000-0000-0000000006b1"
	if err := repo.Create(ctx, applicant(t, studentID, tenantA, "NOTB050312MDFRNN01", "1° A"), testNumberYear); err != nil {
		t.Fatalf("Create: %v", err)
	}
	at := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	who := domain.NoteAuthor{UserID: "60000000-0000-0000-0000-0000000006b1", Name: "Gabriela Rojas"}
	note, _ := domain.NewNote("b6000000-0000-0000-0000-0000000000b1", tenantA, studentID, "Fiebre", who, at)
	if err := repo.AddNote(ctx, note); err != nil {
		t.Fatalf("AddNote: %v", err)
	}

	edited, rev, err := note.Edit("Fiebre de 38", who, at.Add(time.Hour), false)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.EditNote(ctx, edited, rev); err != nil {
		t.Fatalf("EditNote: %v", err)
	}
	stored, err := repo.NoteByID(ctx, note.ID())
	if err != nil || stored.Body() != "Fiebre de 38" || !stored.UpdatedAt().Equal(at.Add(time.Hour)) || stored.UpdatedBy() != who {
		t.Fatalf("NoteByID = %+v, %v: want the edit recorded", stored, err)
	}
	listed, err := repo.NotesOf(ctx, studentID)
	if err != nil || len(listed) != 1 || !listed[0].IsEdited() {
		t.Errorf("NotesOf = %+v, %v: want the edited note", listed, err)
	}
	revs, err := repo.RevisionsOf(ctx, note.ID())
	if err != nil || len(revs) != 1 || revs[0].PreviousBody != "Fiebre" || revs[0].EditedBy != who {
		t.Errorf("RevisionsOf = %+v, %v: want the replaced text", revs, err)
	}

	// The same edit again: the note no longer says "Fiebre" — a stale page.
	if err := repo.EditNote(ctx, edited, rev); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("stale EditNote: want KindConflict, got %v", err)
	}
	if revs, _ := repo.RevisionsOf(ctx, note.ID()); len(revs) != 1 {
		t.Errorf("a refused edit kept a revision: %+v", revs)
	}

	again, rev2, _ := stored.Edit("Fiebre de 39", who, at.Add(2*time.Hour), false)
	if err := appRepo(t, tenantB).EditNote(ctx, again, rev2); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B edited tenant A's note (RLS write breach): %v", err)
	}
	if revs, err := appRepo(t, tenantB).RevisionsOf(ctx, note.ID()); err != nil || len(revs) != 0 {
		t.Errorf("tenant B read tenant A's revisions (RLS read breach): %v %v", revs, err)
	}

	// Append-only: the app role can neither rewrite nor erase the trail, and is
	// told so loudly (a privilege error), not by an UPDATE that matches no row.
	for _, stmt := range []string{`UPDATE student_note_revisions SET previous_body = 'x'`, `DELETE FROM student_note_revisions`} {
		if err := repo.ExecAsTenant(ctx, stmt); err == nil {
			t.Errorf("%s: want a privilege error, got none", stmt)
		}
	}
	if revs, _ := repo.RevisionsOf(ctx, note.ID()); len(revs) != 1 || revs[0].PreviousBody != "Fiebre" {
		t.Errorf("the revision trail was rewritten or erased: %+v", revs)
	}

	voided, _ := stored.Void(who.UserID, at.Add(3*time.Hour), true)
	if err := repo.VoidNote(ctx, voided); err != nil {
		t.Fatalf("VoidNote: %v", err)
	}
	if err := repo.EditNote(ctx, again, rev2); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("edit of a voided note: want KindConflict, got %v", err)
	}
}
