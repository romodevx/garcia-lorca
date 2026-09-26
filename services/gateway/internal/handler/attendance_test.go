package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeAttendance is the director's attendance surface: the pass-list double of
// the teaching tests plus the per-enrollment history read.
type fakeAttendance struct {
	fakeTeachingAttendance

	listEnrollment string
	listFrom       string
	listTo         string
	records        []client.AttendanceRecord
	listErr        error
}

func (f *fakeAttendance) ListAttendance(_ context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error) {
	f.listEnrollment, f.listFrom, f.listTo = enrollmentID, from, to
	return f.records, f.listErr
}

var directorUsers = &fakeUsers{users: map[string]client.User{"usr-dir": {ID: "usr-dir", DisplayName: "Dirección"}}}

func newDirectorAttendance(att handler.AttendanceService) *handler.AttendanceHandler {
	students := &fakeTeachingStudents{students: []client.StudentResult{{ID: "stu-1", FirstName: "Ana", LastName: "García"}}}
	return handler.NewAttendanceHandler(att, listTeacher(), students, directorUsers)
}

// directorReq is an authenticated director request (attendance:read/write).
func directorReq(method, target, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{
		UserID: "usr-dir", TenantID: "t1", Email: "director@garcialorca.edu.mx",
		Permissions: []string{"attendance:read", "attendance:write"},
	}))
}

// --- roster ----------------------------------------------------------------

// The director has no date window (every day is editable, future included) but
// the level's mode still applies, and a per-class subject must be one taught in
// that group.
func TestAttendanceGetRoster(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantCode    int
		wantBody    string
		wantMode    string
		wantSubject string
	}{
		{name: "daily list long ago is editable", query: "groupId=grp-prim&date=2026-01-10", wantCode: 200, wantMode: "daily"},
		{name: "daily list in the future is editable", query: "groupId=grp-prim&date=2026-12-01", wantCode: 200, wantMode: "daily"},
		{name: "per-class list of any subject taught in the group", query: "groupId=grp-sec&date=2026-09-15&subjectId=sub-hist", wantCode: 200, wantMode: "per_class", wantSubject: "sub-hist"},
		{name: "per-class list without a subject", query: "groupId=grp-sec&date=2026-09-15", wantCode: 400, wantBody: "Esta lista es por clase: elige la asignatura"},
		{name: "daily list with a subject", query: "groupId=grp-prim&date=2026-09-15&subjectId=sub-esp", wantCode: 400, wantBody: "Esta lista es por día"},
		{name: "a subject not taught in the group", query: "groupId=grp-sec&date=2026-09-15&subjectId=sub-esp", wantCode: 400, wantBody: "La asignatura no se imparte en este grupo"},
		{name: "unknown group", query: "groupId=ghost&date=2026-09-15", wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{roster: []client.AttendanceRosterRow{
				{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "late", Note: "8:15"},
			}}}
			h := newDirectorAttendance(att)
			rec := httptest.NewRecorder()
			h.GetRoster(rec, directorReq(http.MethodGet, "/api/v1/attendance/roster?"+tt.query, ""))

			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				assert.Zero(t, att.rosterCalls)
				assert.Contains(t, rec.Body.String(), tt.wantBody)
				return
			}
			out := decodeAttendanceList(t, rec)
			assert.Equal(t, tt.wantMode, out.Mode)
			assert.True(t, out.Editable, "the director has no date window")
			assert.Equal(t, 7, out.WindowDays)
			assert.Equal(t, tt.wantSubject, att.gotRosterSubjectID)
			require.Len(t, out.Rows, 1)
			assert.Equal(t, "Ana García", out.Rows[0].Name)
			assert.Equal(t, "8:15", out.Rows[0].Note)
		})
	}
}

func TestAttendanceGetRosterUpstreamFailureIs500(t *testing.T) {
	att := &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{rosterErr: connect.NewError(connect.CodeInternal, nil)}}
	rec := httptest.NewRecorder()
	newDirectorAttendance(att).GetRoster(rec, directorReq(http.MethodGet, "/api/v1/attendance/roster?groupId=grp-prim&date=2026-09-01", ""))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- save the day ----------------------------------------------------------

func TestAttendanceSaveDaily(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantBody string
	}{
		{name: "an old day", body: `{"groupId":"grp-prim","date":"2026-02-02","marks":[]}`, wantCode: 200},
		{name: "per-class list", body: `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`, wantCode: 200},
		{name: "per-class list without a subject", body: `{"groupId":"grp-sec","date":"2026-09-15","marks":[]}`, wantCode: 400, wantBody: "Esta lista es por clase"},
		{name: "a subject not taught in the group", body: `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-esp","marks":[]}`, wantCode: 400, wantBody: "La asignatura no se imparte en este grupo"},
		{name: "unknown group", body: `{"groupId":"ghost","date":"2026-09-15","marks":[]}`, wantCode: 404},
		{name: "malformed body", body: `{`, wantCode: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{saveAbsent: 2, saveMarked: 3}}
			rec := httptest.NewRecorder()
			newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance", tt.body))
			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				assert.Zero(t, att.saveCalls)
				assert.Contains(t, rec.Body.String(), tt.wantBody)
				return
			}
			assert.Equal(t, "usr-dir", att.gotSave.TakenBy)
			assert.Equal(t, "Dirección", att.gotSave.TakenByName)
			assert.JSONEq(t, `{"absentCount":2,"markedCount":3}`, rec.Body.String())
		})
	}
}

func TestAttendanceSaveDailyForwardsMarks(t *testing.T) {
	att := &fakeAttendance{}
	rec := httptest.NewRecorder()
	newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance",
		`{"groupId":"grp-prim","date":"2026-09-01","marks":[{"enrollmentId":"enr-1","status":"absent","note":"cita médica"},{"enrollmentId":"enr-2","status":"late"}]}`))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []client.Mark{
		{EnrollmentID: "enr-1", Status: "absent", Note: "cita médica"},
		{EnrollmentID: "enr-2", Status: "late"},
	}, att.gotSave.Marks)
}

// A client error raised upstream (an unknown status, an enrollment outside the
// group, an over-long note) arrives as Connect InvalidArgument ⇒ 400.
func TestAttendanceSaveDailyUpstreamInvalidArgumentIs400(t *testing.T) {
	att := &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{saveErr: connect.NewError(connect.CodeInvalidArgument, nil)}}
	rec := httptest.NewRecorder()
	newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance",
		`{"groupId":"grp-prim","date":"2026-09-01","marks":[{"enrollmentId":"enr-1","status":"tarde"}]}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- list ------------------------------------------------------------------

func TestAttendanceListForwardsRangeAndReturnsRecords(t *testing.T) {
	att := &fakeAttendance{records: []client.AttendanceRecord{
		{ID: "att-1", EnrollmentID: "enr-1", Date: "2026-09-01", Status: "absent", Note: "cita médica", SubjectID: "sub-mat1"},
	}}
	rec := httptest.NewRecorder()
	newDirectorAttendance(att).ListAttendance(rec, directorReq(http.MethodGet, "/api/v1/attendance?enrollmentId=enr-1&from=2026-09-01&to=2026-09-30", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "enr-1", att.listEnrollment)
	assert.Equal(t, "2026-09-01", att.listFrom)
	assert.Equal(t, "2026-09-30", att.listTo)
	var out struct {
		Records []map[string]any `json:"records"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []map[string]any{{
		"id": "att-1", "enrollmentId": "enr-1", "date": "2026-09-01", "status": "absent", "note": "cita médica", "subjectId": "sub-mat1",
	}}, out.Records)
}

func TestAttendanceListUpstreamErrors(t *testing.T) {
	for code, want := range map[connect.Code]int{
		connect.CodeNotFound:        http.StatusNotFound,
		connect.CodeInvalidArgument: http.StatusBadRequest,
	} {
		att := &fakeAttendance{listErr: connect.NewError(code, nil)}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).ListAttendance(rec, directorReq(http.MethodGet, "/api/v1/attendance?enrollmentId=ghost", ""))
		assert.Equal(t, want, rec.Code, code.String())
	}
}
