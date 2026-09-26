package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// listTeacher is the T3 fixture. usr-t → tch-1 teaches:
//   - grp-prim (1° PRIMARIA · A, a DAILY level) for two subjects (asg-1, asg-2);
//   - grp-sec  (1° SECUNDARIA · A, a PER_CLASS level) for Matemáticas I (asg-3).
//
// tch-2 teaches Historia in the same grp-sec (asg-9) and all of grp-other.
func listTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}, {ID: "tch-2", UserID: "usr-other"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-prim", SubjectID: "sub-esp", TeacherID: "tch-1"},
				{ID: "asg-2", CycleID: "cyc-1", GroupID: "grp-prim", SubjectID: "sub-mat", TeacherID: "tch-1"},
				{ID: "asg-3", CycleID: "cyc-1", GroupID: "grp-sec", SubjectID: "sub-mat1", TeacherID: "tch-1"},
				{ID: "asg-old", CycleID: "cyc-past", GroupID: "grp-old", SubjectID: "sub-esp", TeacherID: "tch-1"},
			},
			"tch-2": {
				{ID: "asg-9", CycleID: "cyc-1", GroupID: "grp-sec", SubjectID: "sub-hist", TeacherID: "tch-2"},
				{ID: "asg-8", CycleID: "cyc-1", GroupID: "grp-other", SubjectID: "sub-esp", TeacherID: "tch-2"},
			},
		},
		assignmentsByGroup: map[string][]client.TeachingAssignment{
			"grp-sec": {
				{ID: "asg-3", GroupID: "grp-sec", SubjectID: "sub-mat1"},
				{ID: "asg-9", GroupID: "grp-sec", SubjectID: "sub-hist"},
			},
		},
		cycles: []client.CycleResult{{ID: "cyc-1", Status: "active"}, {ID: "cyc-past", Status: "closed"}},
		groups: []client.GroupResult{
			{ID: "grp-prim", GradeLevelID: "gl-1p", Name: "A"},
			{ID: "grp-sec", GradeLevelID: "gl-1s", Name: "A"},
			{ID: "grp-other", GradeLevelID: "gl-1p", Name: "B"},
		},
		gradeLevels: []client.GradeLevelResult{
			{ID: "gl-1p", LevelID: "lvl-p", Name: "1° PRIMARIA"},
			{ID: "gl-1s", LevelID: "lvl-s", Name: "1° SECUNDARIA"},
		},
		levels: []client.LevelResult{
			{ID: "lvl-p", Name: "Primaria", AttendanceMode: "daily"},
			{ID: "lvl-s", Name: "Secundaria", AttendanceMode: "per_class"},
		},
		subjects: []client.SubjectResult{
			{ID: "sub-esp", Name: "Español"},
			{ID: "sub-mat", Name: "Matemáticas"},
			{ID: "sub-mat1", Name: "Matemáticas I"},
			{ID: "sub-hist", Name: "Historia"},
		},
	}
}

// listResponse is the roster JSON both attendance surfaces answer with.
type listResponse struct {
	Mode       string `json:"mode"`
	Date       string `json:"date"`
	Editable   bool   `json:"editable"`
	WindowDays int    `json:"windowDays"`
	Session    *struct {
		TakenAt     time.Time `json:"takenAt"`
		TakenByName string    `json:"takenByName"`
		UpdatedAt   time.Time `json:"updatedAt"`
	} `json:"session"`
	Rows []struct {
		EnrollmentID string `json:"enrollmentId"`
		StudentID    string `json:"studentId"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Note         string `json:"note"`
	} `json:"rows"`
}

func decodeAttendanceList(t *testing.T, rec *httptest.ResponseRecorder) listResponse {
	t.Helper()
	var out listResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return out
}

// --- classes -------------------------------------------------------------------

// One entry per DAILY group the teacher teaches (deduped across its subjects)
// and one per PER_CLASS assignment; never another teacher's class, never a past
// cycle's.
func TestTeachingAttendanceClasses(t *testing.T) {
	h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Classes(rec, teacherGET("/api/v1/teaching/attendance/classes", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		Classes []map[string]any `json:"classes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []map[string]any{
		{
			"groupId": "grp-prim", "groupLabel": "1° PRIMARIA · A", "groupName": "A", "gradeLevelName": "1° PRIMARIA",
			"levelName": "Primaria", "mode": "daily",
		},
		{
			"groupId": "grp-sec", "groupLabel": "1° SECUNDARIA · A", "groupName": "A", "gradeLevelName": "1° SECUNDARIA",
			"levelName": "Secundaria", "mode": "per_class",
			"subjectId": "sub-mat1", "subjectName": "Matemáticas I", "assignmentId": "asg-3",
		},
	}, out.Classes)
}

func TestTeachingAttendanceClassesEdgeCases(t *testing.T) {
	t.Run("no linked teacher is an empty list", func(t *testing.T) {
		h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Classes(rec, teacherGET("/api/v1/teaching/attendance/classes", "usr-nobody"))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"classes":[]}`, rec.Body.String())
	})
	t.Run("a catalog failure is an error, never a guessed mode", func(t *testing.T) {
		config := listTeacher()
		config.levelsErr = connect.NewError(connect.CodeUnavailable, errString("config down"))
		h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Classes(rec, teacherGET("/api/v1/teaching/attendance/classes", "usr-t"))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
	t.Run("without a principal is 401", func(t *testing.T) {
		h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Classes(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/attendance/classes", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- roster --------------------------------------------------------------------

func TestTeachingAttendanceRoster(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		user         string
		wantCode     int
		wantBody     string // substring of a text/plain error
		wantMode     string
		wantEditable bool
		wantSubject  string
	}{
		{name: "daily list today", query: "groupId=grp-prim&date=2026-09-15", wantCode: 200, wantMode: "daily", wantEditable: true},
		{name: "daily list 7 days back is editable", query: "groupId=grp-prim&date=2026-09-08", wantCode: 200, wantMode: "daily", wantEditable: true},
		{name: "daily list 8 days back is read-only", query: "groupId=grp-prim&date=2026-09-07", wantCode: 200, wantMode: "daily"},
		{name: "a future day is read-only", query: "groupId=grp-prim&date=2026-09-16", wantCode: 200, wantMode: "daily"},
		{name: "per-class list of the caller's subject", query: "groupId=grp-sec&date=2026-09-15&subjectId=sub-mat1", wantCode: 200, wantMode: "per_class", wantEditable: true, wantSubject: "sub-mat1"},
		{name: "daily list with a subject", query: "groupId=grp-prim&date=2026-09-15&subjectId=sub-esp", wantCode: 400, wantBody: "Esta lista es por día"},
		{name: "per-class list without a subject", query: "groupId=grp-sec&date=2026-09-15", wantCode: 400, wantBody: "Esta lista es por clase: elige la asignatura"},
		{name: "another teacher's subject in the caller's group", query: "groupId=grp-sec&date=2026-09-15&subjectId=sub-hist", wantCode: 404},
		{name: "a group the caller does not teach", query: "groupId=grp-other&date=2026-09-15", wantCode: 404},
		{name: "a past cycle's group", query: "groupId=grp-old&date=2026-09-15", wantCode: 404},
		{name: "empty principal owns nothing", query: "groupId=grp-prim&date=2026-09-15", user: "-", wantCode: 404},
		{name: "malformed date", query: "groupId=grp-prim&date=15/09/2026", wantCode: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A refused list gets an empty attendance double: the mode refusal
			// probes whether the list already exists (a stored mark or a
			// session would let it through), and here it must not.
			att := &fakeTeachingAttendance{}
			if tt.wantCode == http.StatusOK {
				att.roster = []client.AttendanceRosterRow{
					{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "excused", Note: "cita médica"},
					{EnrollmentID: "enr-2", StudentID: "stu-2", Status: "present"},
				}
			}
			students := &fakeTeachingStudents{students: []client.StudentResult{
				{ID: "stu-1", FirstName: "Ana", LastName: "García"},
				{ID: "stu-2", FirstName: "Luis", LastName: "Pérez"},
			}}
			h := newTeachingAttendance(listTeacher(), att, students)
			user := "usr-t"
			if tt.user == "-" {
				user = ""
			}
			rec := httptest.NewRecorder()
			h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?"+tt.query, user))

			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				if tt.wantCode == http.StatusNotFound {
					assert.Zero(t, att.rosterCalls, "a list the caller does not own must never reach the attendance service")
				}
				assert.Contains(t, rec.Body.String(), tt.wantBody)
				return
			}
			out := decodeAttendanceList(t, rec)
			assert.Equal(t, tt.wantMode, out.Mode)
			assert.Equal(t, tt.wantEditable, out.Editable)
			assert.Equal(t, 7, out.WindowDays)
			assert.Equal(t, att.gotRosterDate, out.Date)
			assert.Equal(t, tt.wantSubject, att.gotRosterSubjectID)
			assert.Nil(t, out.Session)
			require.Len(t, out.Rows, 2)
			assert.Equal(t, "Ana García", out.Rows[0].Name)
			assert.Equal(t, "excused", out.Rows[0].Status)
			assert.Equal(t, "cita médica", out.Rows[0].Note)
			assert.Equal(t, "Luis Pérez", out.Rows[1].Name)
		})
	}
}

func TestTeachingAttendanceRosterCarriesTheSession(t *testing.T) {
	takenAt := time.Date(2026, 9, 15, 14, 5, 0, 0, time.UTC)
	att := &fakeTeachingAttendance{session: &client.AttendanceSession{TakenAt: takenAt, TakenByName: "Gabriela Rojas", UpdatedAt: takenAt}}
	h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-15", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code)
	out := decodeAttendanceList(t, rec)
	require.NotNil(t, out.Session)
	assert.Equal(t, "Gabriela Rojas", out.Session.TakenByName)
	assert.True(t, out.Session.TakenAt.Equal(takenAt))
}

func TestTeachingAttendanceRosterEmptyGroupIsEmptyArray(t *testing.T) {
	h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-15", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"rows":[]`)
	assert.Contains(t, rec.Body.String(), `"session":null`)
}

func TestTeachingAttendanceRosterWithoutPrincipalIs401(t *testing.T) {
	h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Roster(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-prim", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- save ----------------------------------------------------------------------

func TestTeachingAttendanceSave(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		user        string
		wantCode    int
		wantBody    string
		wantSubject string
	}{
		{name: "daily list today", body: `{"groupId":"grp-prim","date":"2026-09-15","marks":[{"enrollmentId":"enr-1","status":"absent","note":"gripa"}]}`, wantCode: 200},
		{name: "7 days back is the edge of the window", body: `{"groupId":"grp-prim","date":"2026-09-08","marks":[]}`, wantCode: 200},
		{name: "8 days back", body: `{"groupId":"grp-prim","date":"2026-09-07","marks":[]}`, wantCode: 400, wantBody: "Solo puedes corregir hasta 7 días atrás"},
		{name: "tomorrow", body: `{"groupId":"grp-prim","date":"2026-09-16","marks":[]}`, wantCode: 400, wantBody: "No puedes registrar una fecha futura"},
		{name: "per-class list of the caller's subject", body: `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`, wantCode: 200, wantSubject: "sub-mat1"},
		{name: "per-class list without a subject", body: `{"groupId":"grp-sec","date":"2026-09-15","marks":[]}`, wantCode: 400, wantBody: "Esta lista es por clase: elige la asignatura"},
		{name: "daily list with a subject", body: `{"groupId":"grp-prim","date":"2026-09-15","subjectId":"sub-esp","marks":[]}`, wantCode: 400, wantBody: "Esta lista es por día"},
		{name: "another teacher's subject in the caller's group", body: `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-hist","marks":[]}`, wantCode: 404},
		{name: "a group the caller does not teach", body: `{"groupId":"grp-other","date":"2026-09-15","marks":[]}`, wantCode: 404},
		{name: "empty principal owns nothing", body: `{"groupId":"grp-prim","date":"2026-09-15","marks":[]}`, user: "-", wantCode: 404},
		{name: "malformed date", body: `{"groupId":"grp-prim","date":"ayer","marks":[]}`, wantCode: 400},
		{name: "malformed body", body: `{not json`, wantCode: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &fakeTeachingAttendance{saveAbsent: 1, saveMarked: 1}
			h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
			user := "usr-t"
			if tt.user == "-" {
				user = ""
			}
			rec := httptest.NewRecorder()
			h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", user, tt.body))

			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				assert.Zero(t, att.saveCalls, "a refused save must never reach the attendance service")
				assert.Contains(t, rec.Body.String(), tt.wantBody)
				return
			}
			assert.Equal(t, 1, att.saveCalls)
			assert.Equal(t, tt.wantSubject, att.gotSave.SubjectID)
			assert.Equal(t, "usr-t", att.gotSave.TakenBy, "the taker is the principal, never a request field")
			assert.Equal(t, "Gabriela Rojas", att.gotSave.TakenByName)
			assert.JSONEq(t, `{"absentCount":1,"markedCount":1}`, rec.Body.String())
		})
	}
}

// The marks go through unchanged, notes included.
func TestTeachingAttendanceSavePassesMarksThrough(t *testing.T) {
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
		`{"groupId":"grp-prim","date":"2026-09-15","marks":[{"enrollmentId":"enr-1","status":"late","note":"8:15"}]}`))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, client.SaveAttendanceInput{
		GroupID: "grp-prim", Date: "2026-09-15", TakenBy: "usr-t", TakenByName: "Gabriela Rojas",
		Marks: []client.Mark{{EnrollmentID: "enr-1", Status: "late", Note: "8:15"}},
	}, att.gotSave)
}

// A foreign enrollment in an owned group is refused by the attendance service
// (it re-binds each enrollment to the group) and answered 400.
func TestTeachingAttendanceSaveForeignEnrollmentIs400(t *testing.T) {
	att := &fakeTeachingAttendance{saveErr: connect.NewError(connect.CodeInvalidArgument, errString("enrollment not in group"))}
	h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
		`{"groupId":"grp-prim","date":"2026-09-15","marks":[{"enrollmentId":"enr-of-grp-other","status":"absent"}]}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// The taker's name degrades to the principal's email when the directory fails.
func TestTeachingAttendanceSaveTakerNameFallsBackToEmail(t *testing.T) {
	att := &fakeTeachingAttendance{}
	scopeConfig := listTeacher()
	h := newTeachingAttendanceWithUsers(scopeConfig, att, &fakeUsers{err: connect.NewError(connect.CodeUnavailable, errString("iam down"))})
	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-prim","date":"2026-09-15","marks":[]}`))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "prof@garcialorca.mx", att.gotSave.TakenByName)
}

func TestTeachingAttendanceSaveWithoutPrincipalIs401(t *testing.T) {
	h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, httptest.NewRequest(http.MethodPost, "/api/v1/teaching/attendance",
		strings.NewReader(`{"groupId":"grp-prim","date":"2026-09-15","marks":[]}`)))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
