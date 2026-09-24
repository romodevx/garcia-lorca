package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// maxNoteChars is the longest note, in letters (not bytes).
const maxNoteChars = 2000

// AuthorVoidWindow is how long the author may take a note back. After it the
// note is part of the pupil's history, and only whoever may void any note can.
const AuthorVoidWindow = 15 * time.Minute

// NoteAuthor is who wrote a note: the account, and its name as it read then.
type NoteAuthor struct {
	UserID string
	Name   string
}

// Note is one entry in a pupil's history, written by staff.
//
// A note is never edited: a history that can be rewritten is not a history. A
// wrong note is VOIDED — kept, marked, and hidden from the ordinary list — so
// what the school once wrote about a child can always be accounted for.
type Note struct {
	id        string
	tenantID  string
	studentID string
	body      string
	author    NoteAuthor
	createdAt time.Time
	voidedAt  time.Time
	voidedBy  string
}

// NewNote validates a note as written: a body of 1 to 2000 letters, trimmed,
// and an author account.
func NewNote(id, tenantID, studentID, body string, author NoteAuthor, at time.Time) (Note, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > maxNoteChars {
		return Note{}, glerrors.New(glerrors.KindInvalid, "a note has 1 to 2000 characters")
	}
	if strings.TrimSpace(author.UserID) == "" {
		return Note{}, glerrors.New(glerrors.KindInvalid, "a note needs its author")
	}
	return Note{
		id: id, tenantID: tenantID, studentID: studentID, body: trimmed,
		author: NoteAuthor{UserID: author.UserID, Name: strings.TrimSpace(author.Name)}, createdAt: at,
	}, nil
}

// RestoreNote rebuilds a stored note without judging it.
func RestoreNote(id, tenantID, studentID, body string, author NoteAuthor, createdAt, voidedAt time.Time, voidedBy string) Note {
	return Note{
		id: id, tenantID: tenantID, studentID: studentID, body: body, author: author,
		createdAt: createdAt, voidedAt: voidedAt, voidedBy: voidedBy,
	}
}

// Void marks the note as withdrawn by `by` at `at`. The author may, within
// AuthorVoidWindow of writing it; after that, and for anyone else's note, only
// a caller who may void any note (voidsAny). A voided note is not voided again.
func (n Note) Void(by string, at time.Time, voidsAny bool) (Note, error) {
	if n.IsVoided() {
		return Note{}, glerrors.New(glerrors.KindConflict, "this note was already voided")
	}
	ownInWindow := by == n.author.UserID && !at.After(n.createdAt.Add(AuthorVoidWindow))
	if !ownInWindow && !voidsAny {
		return Note{}, glerrors.New(glerrors.KindForbidden, "only its author, within 15 minutes, may void this note")
	}
	n.voidedAt = at
	n.voidedBy = by
	return n, nil
}

// ID is the note's identity.
func (n Note) ID() string { return n.id }

// TenantID is the school the note belongs to.
func (n Note) TenantID() string { return n.tenantID }

// StudentID is the pupil the note is about.
func (n Note) StudentID() string { return n.studentID }

// Body is what was written.
func (n Note) Body() string { return n.body }

// Author is who wrote it.
func (n Note) Author() NoteAuthor { return n.author }

// CreatedAt is when it was written.
func (n Note) CreatedAt() time.Time { return n.createdAt }

// IsVoided reports whether the note was withdrawn.
func (n Note) IsVoided() bool { return !n.voidedAt.IsZero() }

// VoidedAt is when it was withdrawn; zero if it was not.
func (n Note) VoidedAt() time.Time { return n.voidedAt }

// VoidedBy is the account that withdrew it; empty if it was not.
func (n Note) VoidedBy() string { return n.voidedBy }
