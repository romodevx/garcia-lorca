package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// rosterMeta is the part of the roster JSON these tests are about: why a list
// is read-only and the server's school day.
type rosterMeta struct {
	Mode           string  `json:"mode"`
	Editable       bool    `json:"editable"`
	ReadOnlyReason *string `json:"readOnlyReason"`
	Today          string  `json:"today"`
}

func decodeRosterMeta(t *testing.T, rec *httptest.ResponseRecorder) rosterMeta {
	t.Helper()
	var out rosterMeta
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return out
}

// --- readOnlyReason + today (teacher) ---------------------------------------------

// A read-only list says WHY, as a code the web maps to its sentence: a future
// day is "future" (never the 7-day message), a day past the window "window". An
// editable list carries no reason at all. Every roster carries the server's
// school day, so the web's date picker starts from the same day the server
// measures the window against.
func TestTeachingAttendanceRosterSaysWhyItIsReadOnly(t *testing.T) {
	tests := []struct {
		name       string
		date       string
		wantReason string // "" = the field is absent
	}{
		{name: "today is editable", date: "2026-09-15"},
		{name: "7 days back is editable", date: "2026-09-08"},
		{name: "tomorrow is in the future", date: "2026-09-16", wantReason: "future"},
		{name: "8 days back is outside the window", date: "2026-09-07", wantReason: "window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
			rec := httptest.NewRecorder()
			h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date="+tt.date, "usr-t"))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			out := decodeRosterMeta(t, rec)
			assert.Equal(t, "2026-09-15", out.Today, "today is the server's UTC school day")
			if tt.wantReason == "" {
				assert.True(t, out.Editable)
				assert.Nil(t, out.ReadOnlyReason, "an editable list carries no readOnlyReason")
				assert.NotContains(t, rec.Body.String(), "readOnlyReason")
				return
			}
			assert.False(t, out.Editable)
			require.NotNil(t, out.ReadOnlyReason)
			assert.Equal(t, tt.wantReason, *out.ReadOnlyReason)
		})
	}
}

// today follows the UTC school day: 01:00 UTC on the 16th is still the 15th in
// the school, but the system (like finance) is already on the 16th.
func TestTeachingAttendanceRosterTodayIsTheUTCDay(t *testing.T) {
	scope := handler.NewTeachingHandler(listTeacher(), &fakeTeachingGrades{}, &fakeTeachingStudents{})
	lateNight := func() time.Time { return time.Date(2026, 9, 15, 19, 30, 0, 0, time.FixedZone("CST", -6*3600)) }
	h := handler.NewTeachingAttendanceHandler(scope, &fakeTeachingAttendance{}, teacherUsers, handler.WithAttendanceClock(lateNight))
	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-16", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	out := decodeRosterMeta(t, rec)
	assert.Equal(t, "2026-09-16", out.Today)
	assert.True(t, out.Editable)
}

// The director has no window: every list is editable, no reason, and the same
// server school day.
func TestAttendanceGetRosterCarriesTodayAndNoReason(t *testing.T) {
	students := &fakeTeachingStudents{}
	h := handler.NewAttendanceHandler(&fakeAttendance{}, listTeacher(), students, directorUsers, handler.WithDirectorAttendanceClock(fixedClock))
	rec := httptest.NewRecorder()
	h.GetRoster(rec, directorReq(http.MethodGet, "/api/v1/attendance/roster?groupId=grp-prim&date=2026-12-01", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	out := decodeRosterMeta(t, rec)
	assert.True(t, out.Editable)
	assert.Nil(t, out.ReadOnlyReason)
	assert.Equal(t, "2026-09-15", out.Today)
}

// --- the level's mode governs NEW lists only --------------------------------------

// takenDaily is a list somebody already saved: it has a session.
func takenDaily() *fakeTeachingAttendance {
	at := time.Date(2026, 9, 10, 14, 0, 0, 0, time.UTC)
	return &fakeTeachingAttendance{session: &client.AttendanceSession{TakenAt: at, TakenByName: "Gabriela Rojas", UpdatedAt: at}}
}

// markedOnly is a list with a stored mark but no session (written before
// sessions existed).
func markedOnly() *fakeTeachingAttendance {
	return &fakeTeachingAttendance{roster: []client.AttendanceRosterRow{{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "absent"}}}
}

// grp-sec's level is per_class now. A DAILY list saved for it before the switch
// is still the school's record of that day: the teacher can read and correct
// it. A daily list nobody took is still refused — the mode governs new lists.
func TestTeachingAttendanceExistingListSurvivesAModeSwitch(t *testing.T) {
	t.Run("roster of an existing daily list under a per_class level", func(t *testing.T) {
		for name, att := range map[string]*fakeTeachingAttendance{"session": takenDaily(), "stored mark": markedOnly()} {
			h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
			rec := httptest.NewRecorder()
			h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-sec&date=2026-09-10", "usr-t"))
			require.Equal(t, http.StatusOK, rec.Code, "%s: %s", name, rec.Body.String())
			out := decodeRosterMeta(t, rec)
			assert.Equal(t, "daily", out.Mode, "%s: the list keeps the shape it was taken in", name)
			assert.True(t, out.Editable, name)
			assert.Equal(t, "", att.gotRosterSubjectID, name)
		}
	})
	t.Run("save corrects an existing daily list under a per_class level", func(t *testing.T) {
		att := takenDaily()
		h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-sec","date":"2026-09-10","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, 1, att.saveCalls)
		assert.Equal(t, "", att.gotSave.SubjectID)
	})
	t.Run("a new daily list under a per_class level is still refused", func(t *testing.T) {
		att := &fakeTeachingAttendance{}
		h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-sec","date":"2026-09-10","marks":[]}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Esta lista es por clase")
		assert.Zero(t, att.saveCalls)
	})
	t.Run("an existing per-class list under a daily level, caller's subject", func(t *testing.T) {
		att := takenDaily()
		h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-10&subjectId=sub-esp", "usr-t"))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "per_class", decodeRosterMeta(t, rec).Mode)
	})
	t.Run("ownership is unchanged: another teacher's existing per-class list is 404", func(t *testing.T) {
		att := takenDaily()
		h := newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-10&subjectId=sub-hist", "usr-t"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Zero(t, att.rosterCalls, "an unowned list is never probed")
	})
}

func TestAttendanceExistingListSurvivesAModeSwitch(t *testing.T) {
	t.Run("director reads an existing daily list under a per_class level", func(t *testing.T) {
		att := &fakeAttendance{fakeTeachingAttendance: *takenDaily()}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).GetRoster(rec, directorReq(http.MethodGet, "/api/v1/attendance/roster?groupId=grp-sec&date=2026-09-10", ""))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "daily", decodeRosterMeta(t, rec).Mode)
	})
	t.Run("director corrects it", func(t *testing.T) {
		att := &fakeAttendance{fakeTeachingAttendance: *markedOnly()}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance", `{"groupId":"grp-sec","date":"2026-09-10","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, 1, att.saveCalls)
	})
	t.Run("a new list under the old key is still refused", func(t *testing.T) {
		att := &fakeAttendance{}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance", `{"groupId":"grp-prim","date":"2026-09-10","subjectId":"sub-esp","marks":[]}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Esta lista es por día")
		assert.Zero(t, att.saveCalls)
	})
}

// --- one list resolves one group chain, not the catalog ----------------------------

// Resolving a list reads the group, its grade and its level — never the whole
// catalog (groups, grades, levels AND subjects) for a single group.
func TestAttendanceListsResolveOnlyTheGroupChain(t *testing.T) {
	t.Run("teacher", func(t *testing.T) {
		config := listTeacher()
		h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-sec&date=2026-09-15&subjectId=sub-mat1", "usr-t"))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Zero(t, config.gotListGroups, "no whole-catalog group list")
		assert.Zero(t, config.gotListSubjects, "subjects are not needed to resolve a list")
		assert.Equal(t, 3, config.gotPointReads, "group → grade → level")
	})
	t.Run("director", func(t *testing.T) {
		config := listTeacher()
		h := handler.NewAttendanceHandler(&fakeAttendance{}, config, &fakeTeachingStudents{}, directorUsers)
		rec := httptest.NewRecorder()
		h.SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance", `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Zero(t, config.gotListGroups)
		assert.Zero(t, config.gotListSubjects)
		assert.Equal(t, 3, config.gotPointReads)
	})
}

// --- the taker's name fits its column ------------------------------------------------

// taken_by_name is CHECKed to 200 characters; a longer iam display name is cut
// to fit instead of making the attendance service refuse the whole save.
func TestTeachingAttendanceSaveClampsALongTakerName(t *testing.T) {
	long := strings.Repeat("Ñ", 250)
	att := &fakeTeachingAttendance{}
	users := &fakeUsers{users: map[string]client.User{"usr-t": {ID: "usr-t", DisplayName: long}}}
	h := newTeachingAttendanceWithUsers(listTeacher(), att, users)
	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-prim","date":"2026-09-15","marks":[]}`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 200, utf8.RuneCountInString(att.gotSave.TakenByName))
	assert.True(t, strings.HasPrefix(long, att.gotSave.TakenByName))
}
