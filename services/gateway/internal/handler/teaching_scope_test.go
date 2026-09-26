package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// The teacher surfaces answer for the ACTIVE cycle only (T2). A teacher keeps
// last year's assignments in the catalogue — they are history — but their grades
// and pass lists are closed: a request aimed at one is the same 404 an unknown
// id gets, so nothing about a past cycle leaks.

// twoCycleTeacher: usr-t is tch-1, who taught asg-old / grp-old in the closed
// cyc-old and teaches asg-now / grp-now in the active cyc-now. usr-b is tch-2,
// who teaches asg-b / grp-b in the active cycle.
func twoCycleTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-1", UserID: "usr-t"},
			{ID: "tch-2", UserID: "usr-b"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-old", CycleID: "cyc-old", GroupID: "grp-old", SubjectID: "sub-1", TeacherID: "tch-1"},
				{ID: "asg-now", CycleID: "cyc-now", GroupID: "grp-now", SubjectID: "sub-1", TeacherID: "tch-1"},
			},
			"tch-2": {
				{ID: "asg-b", CycleID: "cyc-now", GroupID: "grp-b", SubjectID: "sub-2", TeacherID: "tch-2"},
			},
		},
		cycles: []client.CycleResult{
			{ID: "cyc-old", Status: "closed"},
			{ID: "cyc-now", Status: "active"},
			{ID: "cyc-next", Status: "draft"},
		},
		termsByCycle: map[string][]client.TermResult{
			"cyc-old": {{ID: "trm-old", CycleID: "cyc-old"}},
			"cyc-now": {{ID: "trm-now", CycleID: "cyc-now"}},
		},
	}
}

func decodeIDs(t *testing.T, body []byte, key string) []string {
	t.Helper()
	var out map[string][]map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	ids := make([]string, 0, len(out[key]))
	for _, item := range out[key] {
		ids = append(ids, item["id"].(string))
	}
	return ids
}

func TestTeachingListsAnswerForTheActiveCycleOnly(t *testing.T) {
	tests := []struct {
		name  string
		serve func(config *fakeTeachingConfig, rec *httptest.ResponseRecorder)
		key   string
		want  []string
	}{
		{
			name: "assignments",
			serve: func(c *fakeTeachingConfig, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(c, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
					Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))
			},
			key: "assignments", want: []string{"asg-now"},
		},
		{
			name: "terms",
			serve: func(c *fakeTeachingConfig, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(c, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
					Terms(rec, teacherGET("/api/v1/teaching/terms", "usr-t"))
			},
			key: "terms", want: []string{"trm-now"},
		},
		{
			name: "attendance groups",
			serve: func(c *fakeTeachingConfig, rec *httptest.ResponseRecorder) {
				newTeachingAttendance(c, &fakeTeachingAttendance{}, &fakeTeachingStudents{}).
					Groups(rec, teacherGET("/api/v1/teaching/groups", "usr-t"))
			},
			key: "groups", want: []string{"grp-now"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.serve(twoCycleTeacher(), rec)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.want, decodeIDs(t, rec.Body.Bytes(), tc.key))
		})
	}
}

func TestTeachingListsIncludeEveryActiveCycle(t *testing.T) {
	config := twoCycleTeacher()
	config.cycles = []client.CycleResult{{ID: "cyc-old", Status: "active"}, {ID: "cyc-now", Status: "active"}}
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
		Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.ElementsMatch(t, []string{"asg-old", "asg-now"}, decodeIDs(t, rec.Body.Bytes(), "assignments"))
}

func TestTeachingListsWithNoActiveCycleAreEmpty(t *testing.T) {
	config := twoCycleTeacher()
	config.cycles = []client.CycleResult{{ID: "cyc-old", Status: "closed"}, {ID: "cyc-now", Status: "draft"}}
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
		Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, decodeIDs(t, rec.Body.Bytes(), "assignments"))
}

// Not knowing which cycle is live must not widen the scope to every cycle: the
// read fails closed.
func TestTeachingCycleLookupFailureFailsClosed(t *testing.T) {
	config := twoCycleTeacher()
	config.cyclesErr = connect.NewError(connect.CodeUnavailable, errors.New("config down"))
	grades := &fakeTeachingGrades{}
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(config, grades, &fakeTeachingStudents{}).
		Roster(rec, teacherGET("/api/v1/teaching/roster?assignmentId=asg-now&termId=trm-now", "usr-t"))
	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError, "an upstream failure, never a widened read")
	assert.Zero(t, grades.rosterCalls)
}

// A teacher with no linked record never costs a cycle read.
func TestTeachingUnlinkedPrincipalSkipsTheCycleRead(t *testing.T) {
	config := twoCycleTeacher()
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
		Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-nobody"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, config.gotListCycles)
}

// --- IDOR sweep: every caller-supplied id outside the caller's active scope ---

// Each case is teacher A (usr-t) aiming at something that is not theirs in the
// active cycle. Every one must be 404 and must never reach the downstream
// service.
func TestTeachingIDORSweep(t *testing.T) {
	type downstream struct {
		grades *fakeTeachingGrades
		att    *fakeTeachingAttendance
	}
	tests := []struct {
		name  string
		serve func(d downstream, rec *httptest.ResponseRecorder)
		calls func(d downstream) int
	}{
		{
			name: "grades roster of B's assignment",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(twoCycleTeacher(), d.grades, &fakeTeachingStudents{}).
					Roster(rec, teacherGET("/api/v1/teaching/roster?assignmentId=asg-b&termId=trm-now", "usr-t"))
			},
			calls: func(d downstream) int { return d.grades.rosterCalls },
		},
		{
			name: "grade capture on B's assignment",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(twoCycleTeacher(), d.grades, &fakeTeachingStudents{}).
					CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
						`{"assignmentId":"asg-b","enrollmentId":"enr-b1","termId":"trm-now","value":10}`))
			},
			calls: func(d downstream) int { return d.grades.captureCalls },
		},
		{
			name: "grades roster of A's own past-cycle assignment",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(twoCycleTeacher(), d.grades, &fakeTeachingStudents{}).
					Roster(rec, teacherGET("/api/v1/teaching/roster?assignmentId=asg-old&termId=trm-old", "usr-t"))
			},
			calls: func(d downstream) int { return d.grades.rosterCalls },
		},
		{
			name: "grade capture on A's own past-cycle assignment",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				handler.NewTeachingHandler(twoCycleTeacher(), d.grades, &fakeTeachingStudents{}).
					CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
						`{"assignmentId":"asg-old","enrollmentId":"enr-1","termId":"trm-old","value":10}`))
			},
			calls: func(d downstream) int { return d.grades.captureCalls },
		},
		{
			name: "attendance roster of B's group",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				newTeachingAttendance(twoCycleTeacher(), d.att, &fakeTeachingStudents{}).
					Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-b&date=2026-09-01", "usr-t"))
			},
			calls: func(d downstream) int { return d.att.rosterCalls },
		},
		{
			name: "attendance save on B's group",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				newTeachingAttendance(twoCycleTeacher(), d.att, &fakeTeachingStudents{}).
					SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
						`{"groupId":"grp-b","date":"2026-09-01","marks":[]}`))
			},
			calls: func(d downstream) int { return d.att.saveCalls },
		},
		{
			name: "attendance roster of A's own past-cycle group",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				newTeachingAttendance(twoCycleTeacher(), d.att, &fakeTeachingStudents{}).
					Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-old&date=2026-09-01", "usr-t"))
			},
			calls: func(d downstream) int { return d.att.rosterCalls },
		},
		{
			name: "attendance save on A's own past-cycle group",
			serve: func(d downstream, rec *httptest.ResponseRecorder) {
				newTeachingAttendance(twoCycleTeacher(), d.att, &fakeTeachingStudents{}).
					SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
						`{"groupId":"grp-old","date":"2026-09-01","marks":[]}`))
			},
			calls: func(d downstream) int { return d.att.saveCalls },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := downstream{grades: &fakeTeachingGrades{}, att: &fakeTeachingAttendance{}}
			rec := httptest.NewRecorder()
			tc.serve(d, rec)
			assert.Equal(t, http.StatusNotFound, rec.Code, "never 403: no existence leak")
			assert.Zero(t, tc.calls(d), "an out-of-scope id must never reach downstream")
		})
	}
}

// A's own group, but a mark for an enrollment of ANOTHER group smuggled in: the
// gateway only owns the group check, so it passes the save through and the
// attendance service's per-mark re-binding refuses it (InvalidArgument → 400).
// The same for a grade captured on A's assignment for a pupil outside it: the
// grades service binds enrollment→group and refuses.
func TestTeachingForeignEnrollmentInsideOwnScopeIsRefusedDownstream(t *testing.T) {
	refuse := connect.NewError(connect.CodeInvalidArgument, errors.New("enrollment is not in this group"))

	t.Run("attendance", func(t *testing.T) {
		att := &fakeTeachingAttendance{saveErr: refuse}
		rec := httptest.NewRecorder()
		newTeachingAttendance(twoCycleTeacher(), att, &fakeTeachingStudents{}).
			SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
				`{"groupId":"grp-now","date":"2026-09-01","marks":[{"enrollmentId":"enr-of-grp-b","status":"absent"}]}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, 1, att.saveCalls)
		assert.Equal(t, "grp-now", att.gotSaveGroupID, "the save is keyed on the OWNED group")
		assert.Equal(t, "enr-of-grp-b", att.gotSaveMarks[0].EnrollmentID)
	})

	t.Run("grades", func(t *testing.T) {
		grades := &fakeTeachingGrades{captureErr: refuse}
		rec := httptest.NewRecorder()
		handler.NewTeachingHandler(twoCycleTeacher(), grades, &fakeTeachingStudents{}).
			CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
				`{"assignmentId":"asg-now","enrollmentId":"enr-of-grp-b","termId":"trm-now","value":10}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, 1, grades.captureCalls)
		assert.Equal(t, "asg-now", grades.gotCaptureAssignmentID)
	})
}
