package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type fakeNotes struct {
	gotStudent, gotBody, gotNote, gotVoidedBy string
	gotAuthor                                 client.NoteAuthor
	gotIncludeVoided, gotVoidsAny             bool
}

func (f *fakeNotes) AddStudentNote(_ context.Context, studentID, body string, author client.NoteAuthor) (client.StudentNote, error) {
	f.gotStudent, f.gotBody, f.gotAuthor = studentID, body, author
	return client.StudentNote{ID: "n-1", Body: body, AuthorName: author.Name}, nil
}

func (f *fakeNotes) ListStudentNotes(_ context.Context, studentID string, includeVoided bool) ([]client.StudentNote, error) {
	f.gotStudent, f.gotIncludeVoided = studentID, includeVoided
	return []client.StudentNote{{ID: "n-1", Body: "x"}}, nil
}

func (f *fakeNotes) VoidStudentNote(_ context.Context, studentID, noteID, voidedBy string, voidsAny bool) (client.StudentNote, error) {
	f.gotStudent, f.gotNote, f.gotVoidedBy, f.gotVoidsAny = studentID, noteID, voidedBy, voidsAny
	return client.StudentNote{ID: noteID, Voided: true}, nil
}

type fakeNamer struct{ user client.User }

func (f fakeNamer) GetUser(context.Context, string) (client.User, error) { return f.user, nil }

func notesRequest(method, path, body string, perms ...string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetPathValue("id", "stu-1")
	req.SetPathValue("noteId", "n-1")
	return req.WithContext(middleware.WithPrincipal(req.Context(),
		middleware.Principal{UserID: "u-me", Email: "gaby@colegio.mx", Permissions: perms}))
}

// The author is the signed-in caller, named from iam. A body claiming another
// author is ignored.
func TestANoteIsSignedByTheCaller(t *testing.T) {
	svc := &fakeNotes{}
	h := handler.NewNotesHandler(svc, fakeNamer{user: client.User{DisplayName: "Gabriela Rojas"}})
	rec := httptest.NewRecorder()
	h.Add(rec, notesRequest(http.MethodPost, "/api/v1/students/stu-1/notes",
		`{"body":"Llegó con fiebre","authorUserId":"u-owner","authorName":"La directora"}`, "students:notes_write"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if svc.gotAuthor != (client.NoteAuthor{UserID: "u-me", Name: "Gabriela Rojas"}) || svc.gotStudent != "stu-1" || svc.gotBody != "Llegó con fiebre" {
		t.Errorf("forwarded = %+v", svc)
	}
}

// An account without a display name is named by its email, never left blank.
func TestANoteAuthorWithoutANameIsTheirEmail(t *testing.T) {
	svc := &fakeNotes{}
	h := handler.NewNotesHandler(svc, fakeNamer{user: client.User{Email: "gaby@colegio.mx"}})
	h.Add(httptest.NewRecorder(), notesRequest(http.MethodPost, "/", `{"body":"x"}`, "students:notes_write"))
	if svc.gotAuthor.Name != "gaby@colegio.mx" {
		t.Errorf("author name = %q", svc.gotAuthor.Name)
	}
}

func TestVoidedNotesAreListedOnlyForWhoeverVoidsAny(t *testing.T) {
	svc := &fakeNotes{}
	h := handler.NewNotesHandler(svc, fakeNamer{})
	h.List(httptest.NewRecorder(), notesRequest(http.MethodGet, "/", "", "students:read"))
	if svc.gotIncludeVoided {
		t.Error("students:read alone listed voided notes")
	}
	rec := httptest.NewRecorder()
	h.List(rec, notesRequest(http.MethodGet, "/", "", "students:read", "students:notes_void"))
	if !svc.gotIncludeVoided {
		t.Error("students:notes_void did not list voided notes")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestVoidingCarriesTheCallerAndWhetherTheyVoidAny(t *testing.T) {
	svc := &fakeNotes{}
	h := handler.NewNotesHandler(svc, fakeNamer{})
	h.Void(httptest.NewRecorder(), notesRequest(http.MethodPost, "/", "", "students:notes_write"))
	if svc.gotVoidedBy != "u-me" || svc.gotNote != "n-1" || svc.gotVoidsAny {
		t.Errorf("author void = %+v", svc)
	}
	h.Void(httptest.NewRecorder(), notesRequest(http.MethodPost, "/", "", "students:notes_void"))
	if !svc.gotVoidsAny {
		t.Error("students:notes_void did not void any")
	}
}
