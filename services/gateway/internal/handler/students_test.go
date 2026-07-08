package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeCreator struct {
	got client.CreateApplicantInput
	res client.StudentResult
	err error
}

func (f *fakeCreator) CreateApplicant(_ context.Context, in client.CreateApplicantInput) (client.StudentResult, error) {
	f.got = in
	return f.res, f.err
}

func postStudents(t *testing.T, h *handler.StudentsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/students", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

func TestCreateStudentReturns201AndBody(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{
		ID:              "stu-1",
		CURP:            "HEGA050312MDFRNN08",
		FirstName:       "Ana",
		Level:           "primaria",
		AdmissionStatus: "pending",
	}}
	h := handler.NewStudentsHandler(svc)

	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria","gradeGroup":"4° A","guardianName":"Rosa"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.got.CURP != "HEGA050312MDFRNN08" || svc.got.Level != "primaria" {
		t.Errorf("forwarded input = %+v", svc.got)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "stu-1" || out["admissionStatus"] != "pending" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateStudentInvalidBodyIs400(t *testing.T) {
	h := handler.NewStudentsHandler(&fakeCreator{})
	rec := postStudents(t, h, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStudentDuplicateIs409(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestCreateStudentInvalidArgumentIs400(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"bad","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStudentInternalErrorIs500(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeUnavailable, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
