package handler

import (
	"context"
	"net/http"
	"slices"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// permVoidAnyNote lets a caller void any note, at any time (the owner, task 6).
const permVoidAnyNote = "students:notes_void"

// NotesService is the notes surface of the students service.
type NotesService interface {
	AddStudentNote(ctx context.Context, studentID, body string, author client.NoteAuthor) (client.StudentNote, error)
	ListStudentNotes(ctx context.Context, studentID string, includeVoided bool) ([]client.StudentNote, error)
	VoidStudentNote(ctx context.Context, studentID, noteID, voidedBy string, voidsAny bool) (client.StudentNote, error)
}

// UserNamer names the caller for the note's byline.
type UserNamer interface {
	GetUser(ctx context.Context, id string) (client.User, error)
}

// NotesHandler serves the notes staff keep about a pupil. The author is the
// authenticated caller — never a field of the request body.
type NotesHandler struct {
	svc   NotesService
	names UserNamer
}

// NewNotesHandler wires the handler to the students service and iam's directory.
func NewNotesHandler(svc NotesService, names UserNamer) *NotesHandler {
	return &NotesHandler{svc: svc, names: names}
}

type addNoteRequest struct {
	Body string `json:"body"`
}

// Add handles POST /api/v1/students/{id}/notes.
func (h *NotesHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req addNoteRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	p, _ := middleware.PrincipalFrom(r.Context())
	author := client.NoteAuthor{UserID: p.UserID, Name: h.nameOf(r.Context(), p)}
	note, err := h.svc.AddStudentNote(r.Context(), r.PathValue("id"), req.Body, author)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, note)
}

// List handles GET /api/v1/students/{id}/notes. Voided notes are included only
// for a caller who may void any note, marked as such.
func (h *NotesHandler) List(w http.ResponseWriter, r *http.Request) {
	notes, err := h.svc.ListStudentNotes(r.Context(), r.PathValue("id"), callerHolds(r, permVoidAnyNote))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// What the school writes about a child is not left in a shared computer's cache.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

// Void handles POST /api/v1/students/{id}/notes/{noteId}/void.
func (h *NotesHandler) Void(w http.ResponseWriter, r *http.Request) {
	p, _ := middleware.PrincipalFrom(r.Context())
	note, err := h.svc.VoidStudentNote(r.Context(), r.PathValue("id"), r.PathValue("noteId"),
		p.UserID, callerHolds(r, permVoidAnyNote))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

// nameOf is the caller's display name, else their email: a byline is never
// blank. A failed lookup falls back too, so naming cannot block a note.
func (h *NotesHandler) nameOf(ctx context.Context, p middleware.Principal) string {
	u, err := h.names.GetUser(ctx, p.UserID)
	if err == nil && u.DisplayName != "" {
		return u.DisplayName
	}
	if err == nil && u.Email != "" {
		return u.Email
	}
	return p.Email
}

func callerHolds(r *http.Request, perm string) bool {
	p, ok := middleware.PrincipalFrom(r.Context())
	return ok && slices.Contains(p.Permissions, perm)
}
