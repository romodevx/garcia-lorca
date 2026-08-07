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

// fakeAttendance is a hand-written double for the attendance surface the handler
// needs. It records the last input and returns canned results/errors so each
// handler is exercised in isolation (no Connect round-trip).
type fakeAttendance struct {
	rosterGroup string
	rosterDate  string
	roster      []client.AttendanceRosterRow

	saveGroup string
	saveDate  string
	saveMarks []client.Mark
	absent    int32
	marked    int32

	listEnrollment string
	listFrom       string
	listTo         string
	records        []client.AttendanceRecord

	err error
}

func (f *fakeAttendance) GetDailyRoster(_ context.Context, groupID, date string) ([]client.AttendanceRosterRow, error) {
	f.rosterGroup, f.rosterDate = groupID, date
	return f.roster, f.err
}

func (f *fakeAttendance) SaveDailyAttendance(_ context.Context, groupID, date string, marks []client.Mark) (int32, int32, error) {
	f.saveGroup, f.saveDate, f.saveMarks = groupID, date, marks
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.absent, f.marked, nil
}

func (f *fakeAttendance) ListAttendance(_ context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error) {
	f.listEnrollment, f.listFrom, f.listTo = enrollmentID, from, to
	return f.records, f.err
}

// --- roster ----------------------------------------------------------------

func TestAttendanceGetRosterForwardsQueryAndReturnsRows(t *testing.T) {
	svc := &fakeAttendance{roster: []client.AttendanceRosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "present"},
		{EnrollmentID: "enr-2", StudentID: "stu-2", Status: "absent"},
	}}
	h := handler.NewAttendanceHandler(svc)

	rec := doReq(t, h.GetRoster, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.rosterGroup != "grp-1" || svc.rosterDate != "2026-09-01" {
		t.Errorf("forwarded input = %q %q", svc.rosterGroup, svc.rosterDate)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Default-present: a row with no stored mark still reports "present".
	if len(out.Roster) != 2 || out.Roster[0]["status"] != "present" || out.Roster[1]["status"] != "absent" {
		t.Errorf("body = %v", out.Roster)
	}
	if out.Roster[0]["enrollmentId"] != "enr-1" || out.Roster[0]["studentId"] != "stu-1" {
		t.Errorf("row = %v", out.Roster[0])
	}
}

// An unknown group is an InvalidArgument at the domain ⇒ 400.
func TestAttendanceGetRosterUnknownGroupIs400(t *testing.T) {
	svc := &fakeAttendance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewAttendanceHandler(svc)
	rec := doReq(t, h.GetRoster, http.MethodGet, "/api/v1/attendance/roster?groupId=ghost&date=2026-09-01", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAttendanceGetRosterUpstreamFailureIs500(t *testing.T) {
	svc := &fakeAttendance{err: connect.NewError(connect.CodeInternal, nil)}
	h := handler.NewAttendanceHandler(svc)
	rec := doReq(t, h.GetRoster, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// --- save the day ----------------------------------------------------------

func TestAttendanceSaveDailyReturnsCounts(t *testing.T) {
	svc := &fakeAttendance{absent: 2, marked: 3}
	h := handler.NewAttendanceHandler(svc)

	rec := doReq(t, h.SaveDaily, http.MethodPost, "/api/v1/attendance",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[`+
			`{"enrollmentId":"enr-1","status":"absent","note":"cita médica"},`+
			`{"enrollmentId":"enr-2","status":"late","note":""}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.saveGroup != "grp-1" || svc.saveDate != "2026-09-01" || len(svc.saveMarks) != 2 {
		t.Fatalf("forwarded input = %q %q %v", svc.saveGroup, svc.saveDate, svc.saveMarks)
	}
	if svc.saveMarks[0] != (client.Mark{EnrollmentID: "enr-1", Status: "absent", Note: "cita médica"}) {
		t.Errorf("mark[0] = %+v", svc.saveMarks[0])
	}
	if svc.saveMarks[1] != (client.Mark{EnrollmentID: "enr-2", Status: "late"}) {
		t.Errorf("mark[1] = %+v", svc.saveMarks[1])
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["absentCount"] != float64(2) || out["markedCount"] != float64(3) {
		t.Errorf("body = %v", out)
	}
}

// An empty marks array is the legitimate "everybody present" reconcile: it must
// reach the service (as an empty set), not be rejected at the edge.
func TestAttendanceSaveDailyEmptyMarksIsAllPresent(t *testing.T) {
	svc := &fakeAttendance{}
	h := handler.NewAttendanceHandler(svc)

	rec := doReq(t, h.SaveDaily, http.MethodPost, "/api/v1/attendance",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.saveGroup != "grp-1" || len(svc.saveMarks) != 0 {
		t.Errorf("forwarded input = %q %v", svc.saveGroup, svc.saveMarks)
	}
}

func TestAttendanceSaveDailyInvalidBodyIs400(t *testing.T) {
	h := handler.NewAttendanceHandler(&fakeAttendance{})
	rec := doReq(t, h.SaveDaily, http.MethodPost, "/api/v1/attendance", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// A client error raised upstream (an unknown status, an enrollment outside the
// group, an over-long note) arrives as Connect InvalidArgument and must surface
// at the edge as 400, not 500. This covers the mapping only — what the attendance
// service itself rejects is pinned by its own service tests.
func TestAttendanceSaveDailyUpstreamInvalidArgumentIs400(t *testing.T) {
	svc := &fakeAttendance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewAttendanceHandler(svc)
	rec := doReq(t, h.SaveDaily, http.MethodPost, "/api/v1/attendance",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[{"enrollmentId":"enr-1","status":"tarde"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// --- list ------------------------------------------------------------------

func TestAttendanceListForwardsRangeAndReturnsRecords(t *testing.T) {
	svc := &fakeAttendance{records: []client.AttendanceRecord{
		{ID: "att-1", EnrollmentID: "enr-1", Date: "2026-09-01", Status: "absent", Note: "cita médica"},
	}}
	h := handler.NewAttendanceHandler(svc)

	rec := doReq(t, h.ListAttendance, http.MethodGet,
		"/api/v1/attendance?enrollmentId=enr-1&from=2026-09-01&to=2026-09-30", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.listEnrollment != "enr-1" || svc.listFrom != "2026-09-01" || svc.listTo != "2026-09-30" {
		t.Errorf("forwarded filters = %q %q %q", svc.listEnrollment, svc.listFrom, svc.listTo)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Only exceptions are stored, so present days never appear in the result.
	if len(out.Records) != 1 || out.Records[0]["id"] != "att-1" ||
		out.Records[0]["date"] != "2026-09-01" || out.Records[0]["status"] != "absent" {
		t.Errorf("body = %v", out.Records)
	}
}

func TestAttendanceListUnknownEnrollmentIs404(t *testing.T) {
	svc := &fakeAttendance{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewAttendanceHandler(svc)
	rec := doReq(t, h.ListAttendance, http.MethodGet, "/api/v1/attendance?enrollmentId=ghost", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAttendanceListInvalidRangeIs400(t *testing.T) {
	svc := &fakeAttendance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewAttendanceHandler(svc)
	rec := doReq(t, h.ListAttendance, http.MethodGet,
		"/api/v1/attendance?enrollmentId=enr-1&from=2026-09-30&to=2026-09-01", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
