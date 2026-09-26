package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Regression: ISSUE-001 (T3) — the teacher's attendance picker showed
// "MATEMÁTICAS · 2 grupos" beside "MATEMÁTICAS · 1 grupo": subjects exist per
// grade, and the classes answer carried the grade only baked into groupLabel,
// so the subject step had nothing to tell 1° from 2° Secundaria.
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t3.md
func TestTeachingAttendanceClassesCarryGradeAndGroupName(t *testing.T) {
	cfg := &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1s", SubjectID: "sub-mat-1", TeacherID: "tch-1"},
				{ID: "asg-2", CycleID: "cyc-1", GroupID: "grp-2s", SubjectID: "sub-mat-2", TeacherID: "tch-1"},
				{ID: "asg-3", CycleID: "cyc-1", GroupID: "grp-1p", SubjectID: "sub-esp-p", TeacherID: "tch-1"},
			},
		},
		cycles: []client.CycleResult{{ID: "cyc-1", Status: "active"}},
		groups: []client.GroupResult{
			{ID: "grp-1s", GradeLevelID: "gl-1s", Name: "A"},
			{ID: "grp-2s", GradeLevelID: "gl-2s", Name: "A"},
			{ID: "grp-1p", GradeLevelID: "gl-1p", Name: "B"},
		},
		gradeLevels: []client.GradeLevelResult{
			{ID: "gl-1s", LevelID: "lvl-s", Name: "1° SECUNDARIA"},
			{ID: "gl-2s", LevelID: "lvl-s", Name: "2° SECUNDARIA"},
			{ID: "gl-1p", LevelID: "lvl-p", Name: "1° PRIMARIA"},
		},
		levels: []client.LevelResult{
			{ID: "lvl-s", Name: "SECUNDARIA", AttendanceMode: "per_class"},
			{ID: "lvl-p", Name: "PRIMARIA", AttendanceMode: "daily"},
		},
		subjects: []client.SubjectResult{
			{ID: "sub-mat-1", Name: "MATEMÁTICAS"},
			{ID: "sub-mat-2", Name: "MATEMÁTICAS"},
			{ID: "sub-esp-p", Name: "ESPAÑOL"},
		},
	}
	h := newTeachingAttendance(cfg, &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Classes(rec, teacherGET("/api/v1/teaching/attendance/classes", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		Classes []struct {
			GroupID        string `json:"groupId"`
			GroupLabel     string `json:"groupLabel"`
			GroupName      string `json:"groupName"`
			GradeLevelName string `json:"gradeLevelName"`
		} `json:"classes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Classes, 3)
	byGroup := map[string][2]string{}
	for _, c := range out.Classes {
		byGroup[c.GroupID] = [2]string{c.GradeLevelName, c.GroupName}
	}
	assert.Equal(t, [2]string{"1° SECUNDARIA", "A"}, byGroup["grp-1s"])
	assert.Equal(t, [2]string{"2° SECUNDARIA", "A"}, byGroup["grp-2s"])
	assert.Equal(t, [2]string{"1° PRIMARIA", "B"}, byGroup["grp-1p"], "daily groups carry them too")
}
