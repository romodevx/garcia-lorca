package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var written = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

func author() domain.NoteAuthor {
	return domain.NoteAuthor{UserID: "u-teacher", Name: "Gabriela Rojas"}
}

func mustNote(t *testing.T) domain.Note {
	t.Helper()
	n, err := domain.NewNote("n-1", "t-1", "st-1", "  Llegó con fiebre; se llamó a la mamá.  ", author(), written)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}

func TestNewNoteKeepsTheTrimmedBodyAndItsAuthor(t *testing.T) {
	n := mustNote(t)
	if n.Body() != "Llegó con fiebre; se llamó a la mamá." {
		t.Errorf("body = %q", n.Body())
	}
	if n.Author() != author() || !n.CreatedAt().Equal(written) || n.IsVoided() {
		t.Errorf("note = %+v", n)
	}
}

func TestNewNoteRefusesWhatIsNotANote(t *testing.T) {
	cases := map[string]struct {
		body   string
		author domain.NoteAuthor
	}{
		"empty body":    {"   ", author()},
		"too long":      {strings.Repeat("a", 2001), author()},
		"no author":     {"hola", domain.NoteAuthor{Name: "Alguien"}},
		"author w/o id": {"hola", domain.NoteAuthor{UserID: " ", Name: "X"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewNote("n-1", "t-1", "st-1", tc.body, tc.author, written); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// 2000 LETTERS: an accented note is not refused under the limit.
func TestNoteLimitCountsCharacters(t *testing.T) {
	if _, err := domain.NewNote("n-1", "t-1", "st-1", strings.Repeat("ñ", 2000), author(), written); err != nil {
		t.Errorf("2000 accented letters refused: %v", err)
	}
}

func TestVoidingANote(t *testing.T) {
	cases := []struct {
		name     string
		by       string
		at       time.Time
		voidsAny bool
		want     glerrors.Kind
	}{
		{"the author, within 15 minutes", "u-teacher", written.Add(15 * time.Minute), false, glerrors.KindUnknown},
		{"the author, after 15 minutes", "u-teacher", written.Add(15*time.Minute + time.Second), false, glerrors.KindForbidden},
		{"someone else, at once", "u-other", written.Add(time.Minute), false, glerrors.KindForbidden},
		{"whoever may void any, a day later", "u-owner", written.Add(24 * time.Hour), true, glerrors.KindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			voided, err := mustNote(t).Void(tc.by, tc.at, tc.voidsAny)
			if tc.want == glerrors.KindUnknown {
				if err != nil {
					t.Fatalf("Void: %v", err)
				}
				if !voided.IsVoided() || voided.VoidedBy() != tc.by || !voided.VoidedAt().Equal(tc.at) {
					t.Errorf("voided = %+v", voided)
				}
				return
			}
			if glerrors.KindOf(err) != tc.want {
				t.Errorf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestAVoidedNoteIsNotVoidedTwice(t *testing.T) {
	voided, err := mustNote(t).Void("u-teacher", written.Add(time.Minute), false)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if _, err := voided.Void("u-owner", written.Add(time.Hour), true); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second void: want KindConflict, got %v", err)
	}
}
