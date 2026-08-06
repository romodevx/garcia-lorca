package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeGrades is a hand-written double for the grade surface the handler needs.
// It records the last input and returns canned results/errors so each handler is
// exercised in isolation (no Connect round-trip).
type fakeGrades struct {
	captAssignment string
	captEnrollment string
	captTerm       string
	captValue      float64
	entry          client.GradeEntry

	rosterAssignment string
	rosterTerm       string
	roster           []client.RosterRow

	listEnrollment string
	listTerm       string
	entries        []client.GradeEntry

	err error
}

func (f *fakeGrades) CaptureGrade(_ context.Context, assignmentID, enrollmentID, termID string, value float64) (client.GradeEntry, error) {
	f.captAssignment, f.captEnrollment, f.captTerm, f.captValue = assignmentID, enrollmentID, termID, value
	if f.err != nil {
		return client.GradeEntry{}, f.err
	}
	// subject_id / teacher_id are DERIVED server-side; the fake echoes canned
	// values to prove the edge surfaces them without ever accepting them.
	return client.GradeEntry{ID: "grd-1", EnrollmentID: enrollmentID, SubjectID: "sub-1", TermID: termID, TeacherID: "tch-1", Value: value}, nil
}

func (f *fakeGrades) GetRoster(_ context.Context, assignmentID, termID string) ([]client.RosterRow, error) {
	f.rosterAssignment, f.rosterTerm = assignmentID, termID
	return f.roster, f.err
}

func (f *fakeGrades) ListGradeEntries(_ context.Context, enrollmentID, termID string) ([]client.GradeEntry, error) {
	f.listEnrollment, f.listTerm = enrollmentID, termID
	return f.entries, f.err
}

func TestCaptureGradeReturns200AndBody(t *testing.T) {
	svc := &fakeGrades{}
	h := handler.NewGradesHandler(svc)

	rec := doReq(t, h.CaptureGrade, http.MethodPost, "/api/v1/grades",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":8.5}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.captAssignment != "asg-1" || svc.captEnrollment != "enr-1" || svc.captTerm != "trm-1" || svc.captValue != 8.5 {
		t.Errorf("forwarded input = %q %q %q %v", svc.captAssignment, svc.captEnrollment, svc.captTerm, svc.captValue)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// subject/teacher are derived server-side and surfaced on the read model.
	if out["id"] != "grd-1" || out["value"] != 8.5 || out["subjectId"] != "sub-1" || out["teacherId"] != "tch-1" {
		t.Errorf("body = %v", out)
	}
}

func TestCaptureGradeInvalidBodyIs400(t *testing.T) {
	h := handler.NewGradesHandler(&fakeGrades{})
	rec := doReq(t, h.CaptureGrade, http.MethodPost, "/api/v1/grades", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCaptureGradeBadEnrollmentIs400(t *testing.T) {
	// The capture knot rejects a bad enrollment/term with InvalidArgument.
	svc := &fakeGrades{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewGradesHandler(svc)
	rec := doReq(t, h.CaptureGrade, http.MethodPost, "/api/v1/grades",
		`{"assignmentId":"asg-1","enrollmentId":"nope","termId":"trm-1","value":8.5}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCaptureGradeOutOfRangeValueIs400(t *testing.T) {
	// value 11 violates the 0–10 domain invariant → InvalidArgument → 400.
	svc := &fakeGrades{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewGradesHandler(svc)
	rec := doReq(t, h.CaptureGrade, http.MethodPost, "/api/v1/grades",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":11}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetRosterReturnsRows(t *testing.T) {
	svc := &fakeGrades{roster: []client.RosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Value: 8.5, HasValue: true},
		{EnrollmentID: "enr-2", StudentID: "stu-2", HasValue: false},
	}}
	h := handler.NewGradesHandler(svc)

	rec := doReq(t, h.GetRoster, http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1&termId=trm-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.rosterAssignment != "asg-1" || svc.rosterTerm != "trm-1" {
		t.Errorf("forwarded filters = %q %q", svc.rosterAssignment, svc.rosterTerm)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Roster) != 2 {
		t.Fatalf("roster len = %d, want 2", len(out.Roster))
	}
	if out.Roster[0]["enrollmentId"] != "enr-1" || out.Roster[0]["studentId"] != "stu-1" ||
		out.Roster[0]["value"] != 8.5 || out.Roster[0]["hasValue"] != true {
		t.Errorf("row0 = %v", out.Roster[0])
	}
	if out.Roster[1]["hasValue"] != false {
		t.Errorf("row1 hasValue = %v, want false", out.Roster[1]["hasValue"])
	}
}

func TestGetRosterBadTermIs400(t *testing.T) {
	svc := &fakeGrades{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewGradesHandler(svc)
	rec := doReq(t, h.GetRoster, http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1&termId=nope", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestListGradeEntriesReturnsItems(t *testing.T) {
	svc := &fakeGrades{entries: []client.GradeEntry{
		{ID: "grd-1", EnrollmentID: "enr-1", SubjectID: "sub-1", TermID: "trm-1", TeacherID: "tch-1", Value: 8.5},
	}}
	h := handler.NewGradesHandler(svc)

	rec := doReq(t, h.ListGradeEntries, http.MethodGet, "/api/v1/grades?enrollmentId=enr-1&termId=trm-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.listEnrollment != "enr-1" || svc.listTerm != "trm-1" {
		t.Errorf("forwarded filters = %q %q", svc.listEnrollment, svc.listTerm)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.GradeEntries) != 1 || out.GradeEntries[0]["id"] != "grd-1" || out.GradeEntries[0]["value"] != 8.5 {
		t.Errorf("body = %v", out)
	}
}
