package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// Regression: ISSUE-001 — the config screen asks for the grade "as the school
// names it", so a school writes "1°" under PRIMARIA and "1°" under SECUNDARIA.
// Every teacher surface labelled a group with its grade's name alone, and a
// teacher of both first grades saw "1° · A" twice.
// Found by /qa on 2026-09-28
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-28-full.md

// bareGradeTeacher teaches "A" in two grades both named "1°": grp-p under
// PRIMARIA and grp-s under SECUNDARIA; grp-k's grade already names its level.
func bareGradeTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-p", CycleID: "cyc-1", GroupID: "grp-p", SubjectID: "sub-p", TeacherID: "tch-1"},
				{ID: "asg-s", CycleID: "cyc-1", GroupID: "grp-s", SubjectID: "sub-s", TeacherID: "tch-1"},
				{ID: "asg-k", CycleID: "cyc-1", GroupID: "grp-k", SubjectID: "sub-k", TeacherID: "tch-1"},
			},
		},
		cycles: []client.CycleResult{{ID: "cyc-1", Status: "active"}},
		groups: []client.GroupResult{
			{ID: "grp-p", GradeLevelID: "gl-p", Name: "A"},
			{ID: "grp-s", GradeLevelID: "gl-s", Name: "A"},
			{ID: "grp-k", GradeLevelID: "gl-k", Name: "A"},
		},
		subjects: []client.SubjectResult{
			{ID: "sub-p", GradeLevelID: "gl-p", Name: "ESPAÑOL"},
			{ID: "sub-s", GradeLevelID: "gl-s", Name: "ESPAÑOL"},
			{ID: "sub-k", GradeLevelID: "gl-k", Name: "LENGUAJE"},
		},
		gradeLevels: []client.GradeLevelResult{
			{ID: "gl-p", LevelID: "lvl-p", Name: "1°"},
			{ID: "gl-s", LevelID: "lvl-s", Name: "1°"},
			{ID: "gl-k", LevelID: "lvl-k", Name: "1° KÍNDER"},
		},
		levels: []client.LevelResult{
			{ID: "lvl-p", Name: "PRIMARIA", AttendanceMode: "daily"},
			{ID: "lvl-s", Name: "SECUNDARIA", AttendanceMode: "per_class"},
			{ID: "lvl-k", Name: "KÍNDER", AttendanceMode: "daily"},
		},
	}
}

func TestTeachingAssignmentsNameTheLevelOfABareGrade(t *testing.T) {
	h := handler.NewTeachingHandler(bareGradeTeacher(), &fakeTeachingGrades{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	byID := decodeList(t, rec, "assignments")
	assert.Equal(t, "1° PRIMARIA", byID["asg-p"]["gradeLevelName"])
	assert.Equal(t, "1° SECUNDARIA", byID["asg-s"]["gradeLevelName"])
	assert.Equal(t, "1° KÍNDER", byID["asg-k"]["gradeLevelName"], "a grade that already names its level is kept")
}

func TestTeachingAssignmentsKeepTheBareGradeWhenLevelsCannotBeRead(t *testing.T) {
	cfg := bareGradeTeacher()
	cfg.levels = nil
	h := handler.NewTeachingHandler(cfg, &fakeTeachingGrades{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	byID := decodeList(t, rec, "assignments")
	assert.Equal(t, "1°", byID["asg-p"]["gradeLevelName"])
}

func TestTeachingAttendanceClassesNameTheLevelOfABareGrade(t *testing.T) {
	h := newTeachingAttendance(bareGradeTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Classes(rec, teacherGET("/api/v1/teaching/attendance/classes", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		Classes []struct {
			GroupID        string `json:"groupId"`
			GroupLabel     string `json:"groupLabel"`
			GradeLevelName string `json:"gradeLevelName"`
		} `json:"classes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	labels := map[string][2]string{}
	for _, c := range out.Classes {
		labels[c.GroupID] = [2]string{c.GradeLevelName, c.GroupLabel}
	}
	assert.Equal(t, [2]string{"1° PRIMARIA", "1° PRIMARIA · A"}, labels["grp-p"])
	assert.Equal(t, [2]string{"1° SECUNDARIA", "1° SECUNDARIA · A"}, labels["grp-s"])
	assert.Equal(t, [2]string{"1° KÍNDER", "1° KÍNDER · A"}, labels["grp-k"])
}
