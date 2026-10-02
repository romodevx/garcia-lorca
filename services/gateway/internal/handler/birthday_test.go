package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// T11: the quick-add drawer sends the birth date; it reaches students as-is.
func TestCreateStudentForwardsTheBirthDate(t *testing.T) {
	svc := &fakeCreator{}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","levelId":"lvl-9","guardianName":"Rosa","birthDate":"2016-02-29"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.got.BirthDate != "2016-02-29" {
		t.Errorf("birth date = %q", svc.got.BirthDate)
	}
}

type fakeTemplates struct {
	current client.BirthdayTemplate
	saved   client.BirthdayTemplate
	savedBy string
	err     error
}

func (f *fakeTemplates) GetBirthdayTemplate(context.Context) (client.BirthdayTemplate, error) {
	return f.current, f.err
}

func (f *fakeTemplates) UpdateBirthdayTemplate(_ context.Context, title, body, updatedBy string) (client.BirthdayTemplate, error) {
	f.saved = client.BirthdayTemplate{Title: title, Body: body, UpdatedAt: "2026-10-01T13:00:00Z"}
	f.savedBy = updatedBy
	return f.saved, f.err
}

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{UserID: userID}))
}

func TestGetBirthdayTemplate(t *testing.T) {
	f := &fakeTemplates{current: client.BirthdayTemplate{Title: "¡Feliz cumpleaños, {nombre}!", Body: "Hola {nombre}", IsDefault: true}}
	h := handler.NewBirthdayTemplateHandler(f)
	rec := httptest.NewRecorder()
	h.Get(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/v1/comms/birthday-template", nil), "u-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["title"] != "¡Feliz cumpleaños, {nombre}!" || got["body"] != "Hola {nombre}" || got["isDefault"] != true || got["updatedAt"] != "" {
		t.Errorf("body = %v", got)
	}
	if _, leaked := got["updatedBy"]; leaked {
		t.Error("the editor's user id is not the screen's business")
	}
}

// Who saved it is the session's principal, never a field the client sends.
func TestUpdateBirthdayTemplateStampsTheCaller(t *testing.T) {
	f := &fakeTemplates{}
	h := handler.NewBirthdayTemplateHandler(f)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/comms/birthday-template",
		strings.NewReader(`{"title":"¡Feliz día!","body":"Felicidades, {nombre}.","updatedBy":"someone-else"}`))
	h.Update(rec, withUser(req, "u-director"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if f.savedBy != "u-director" || f.saved.Body != "Felicidades, {nombre}." || f.saved.Title != "¡Feliz día!" {
		t.Errorf("saved = %+v by %q", f.saved, f.savedBy)
	}
}

func TestUpdateBirthdayTemplateErrors(t *testing.T) {
	h := handler.NewBirthdayTemplateHandler(&fakeTemplates{})
	rec := httptest.NewRecorder()
	h.Update(rec, withUser(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{nope`)), "u-1"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Update(rec, httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"title":"a","body":"b"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no principal status = %d", rec.Code)
	}

	invalid := &fakeTemplates{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	rec = httptest.NewRecorder()
	handler.NewBirthdayTemplateHandler(invalid).Update(rec, withUser(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"title":"a","body":" "}`)), "u-1"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid template status = %d", rec.Code)
	}
}
