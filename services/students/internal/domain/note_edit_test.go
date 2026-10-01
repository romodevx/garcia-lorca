package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestEditingANote(t *testing.T) {
	owner := domain.NoteAuthor{UserID: "u-owner", Name: "Dirección"}
	cases := []struct {
		name     string
		editor   domain.NoteAuthor
		at       time.Time
		editsAny bool
		want     glerrors.Kind
	}{
		{"the author, a week later", author(), written.Add(7 * 24 * time.Hour), false, glerrors.KindUnknown},
		{"someone else", domain.NoteAuthor{UserID: "u-other", Name: "Otro"}, written.Add(time.Minute), false, glerrors.KindForbidden},
		{"whoever may edit any", owner, written.Add(time.Hour), true, glerrors.KindUnknown},
		{"no editor", domain.NoteAuthor{Name: "Nadie"}, written.Add(time.Minute), true, glerrors.KindInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := mustNote(t)
			edited, rev, err := original.Edit("  Fiebre de 38; la mamá pasó por él.  ", tc.editor, tc.at, tc.editsAny)
			if tc.want != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.want {
					t.Errorf("want %v, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Edit: %v", err)
			}
			if edited.Body() != "Fiebre de 38; la mamá pasó por él." {
				t.Errorf("body = %q", edited.Body())
			}
			if edited.Author() != author() || !edited.CreatedAt().Equal(written) {
				t.Errorf("an edit changed who wrote it or when: %+v", edited)
			}
			if !edited.IsEdited() || edited.UpdatedBy() != tc.editor || !edited.UpdatedAt().Equal(tc.at) {
				t.Errorf("edit not recorded: %+v", edited)
			}
			if rev.NoteID != original.ID() || rev.PreviousBody != original.Body() ||
				rev.EditedBy != tc.editor || !rev.EditedAt.Equal(tc.at) {
				t.Errorf("revision = %+v", rev)
			}
		})
	}
}

func TestAnEditIsValidatedLikeANewNote(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     "  ",
		"too long":  strings.Repeat("a", 2001),
		"unchanged": " Llegó con fiebre; se llamó a la mamá. ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := mustNote(t).Edit(body, author(), written.Add(time.Minute), false); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestAVoidedNoteIsNotEdited(t *testing.T) {
	voided, err := mustNote(t).Void("u-teacher", written.Add(time.Minute), false)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if _, _, err := voided.Edit("otra cosa", author(), written.Add(2*time.Minute), true); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict, got %v", err)
	}
}

func TestAStoredEditIsRestored(t *testing.T) {
	at := written.Add(time.Hour)
	by := domain.NoteAuthor{UserID: "u-owner", Name: "Dirección"}
	n := mustNote(t).WithStoredEdit(at, by)
	if !n.IsEdited() || !n.UpdatedAt().Equal(at) || n.UpdatedBy() != by {
		t.Errorf("restored = %+v", n)
	}
	if mustNote(t).IsEdited() {
		t.Error("a fresh note reads as edited")
	}
}
