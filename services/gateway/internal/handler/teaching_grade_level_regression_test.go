package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// Regression: ISSUE-001 — a teacher's picker showed "ESPAÑOL, ESPAÑOL" and
// "A, A, A, B, B": subjects are per grade and groups are named by letter only,
// so nothing on the screen told 1° Primaria A from 1° Secundaria A.
// Found by /qa on 2026-09-25
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-25-t2.md

// sameLetterTeacher teaches "A" in two grades: grp-p (1° PRIMARIA) and grp-s
// (1° SECUNDARIA), one subject each.
func sameLetterTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-p", CycleID: "cyc-1", GroupID: "grp-p", SubjectID: "sub-p", TeacherID: "tch-1"},
				{ID: "asg-s", CycleID: "cyc-1", GroupID: "grp-s", SubjectID: "sub-s", TeacherID: "tch-1"},
			},
		},
		groups: []client.GroupResult{
			{ID: "grp-p", GradeLevelID: "gl-p", Name: "A"},
			{ID: "grp-s", GradeLevelID: "gl-s", Name: "A"},
		},
		subjects: []client.SubjectResult{
			{ID: "sub-p", GradeLevelID: "gl-p", Name: "ESPAÑOL"},
			{ID: "sub-s", GradeLevelID: "gl-s", Name: "ESPAÑOL"},
		},
		gradeLevels: []client.GradeLevelResult{
			{ID: "gl-p", Name: "1° PRIMARIA"},
			{ID: "gl-s", Name: "1° SECUNDARIA"},
		},
	}
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder, key string) map[string]map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[string]map[string]any{}
	for _, row := range out[key] {
		byID[row["id"].(string)] = row
	}
	return byID
}

func TestTeachingAssignmentsCarryTheGroupGradeLevelName(t *testing.T) {
	h := handler.NewTeachingHandler(sameLetterTeacher(), &fakeTeachingGrades{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	byID := decodeList(t, rec, "assignments")
	if got := byID["asg-p"]["gradeLevelName"]; got != "1° PRIMARIA" {
		t.Errorf("asg-p gradeLevelName = %v, want 1° PRIMARIA", got)
	}
	if got := byID["asg-s"]["gradeLevelName"]; got != "1° SECUNDARIA" {
		t.Errorf("asg-s gradeLevelName = %v, want 1° SECUNDARIA", got)
	}
}

// The grade name is display enrichment like the others: a failed grade-level
// read degrades to "", never a failed list.
func TestTeachingGradeLevelNameDegradesToEmpty(t *testing.T) {
	config := sameLetterTeacher()
	config.gradeLevelsErr = errors.New("config down")
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	byID := decodeList(t, rec, "assignments")
	if got := byID["asg-p"]["gradeLevelName"]; got != "" {
		t.Errorf("gradeLevelName = %v, want \"\" when the grade levels cannot be read", got)
	}
	if got := byID["asg-p"]["groupName"]; got != "A" {
		t.Errorf("groupName = %v, want A kept", got)
	}
}
