package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeGroupLabeler struct {
	label     string
	found     bool
	err       error
	studentID string
	day       string
}

func (f *fakeGroupLabeler) StudentGroupLabel(_ context.Context, studentID, day string) (string, bool, error) {
	f.studentID, f.day = studentID, day
	return f.label, f.found, f.err
}

// 03:00 UTC on 17 October is still the 16th in Morelia: the label is the
// group of the SCHOOL's day.
var lateEveningInMorelia = time.Date(2026, time.October, 17, 3, 0, 0, 0, time.UTC)

func groupLabelFixture(t *testing.T, labeler *fakeGroupLabeler) *handler.FamilyHandler {
	t.Helper()
	zone, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("zone: %v", err)
	}
	students := oneChild()
	students.student = client.StudentResult{ID: "stu-1", FirstName: "Alumna", LastName: "Dos", Level: "Primaria", GradeGroup: "B"}
	opts := []handler.FamilyOption{handler.WithFamilyClock(func() time.Time { return lateEveningInMorelia })}
	if labeler != nil {
		opts = append(opts, handler.WithFamilyGroupLabeler(labeler, zone))
	}
	return handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, nil, nil, opts...)
}

type labeledStudent struct {
	GradeGroup string `json:"gradeGroup"`
	GroupLabel string `json:"groupLabel"`
}

func listedStudent(t *testing.T, h *handler.FamilyHandler) labeledStudent {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ListStudents(rec, guardianGET("/api/v1/family/students", "usr-9"))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d", rec.Code)
	}
	var body struct {
		Students []labeledStudent `json:"students"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Students) != 1 {
		t.Fatalf("list body = %s (%v)", rec.Body.String(), err)
	}
	return body.Students[0]
}

func profiledStudent(t *testing.T, h *handler.FamilyHandler) labeledStudent {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Profile(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/profile", "usr-9", "studentId", "stu-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: status %d", rec.Code)
	}
	var body labeledStudent
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("profile body = %s (%v)", rec.Body.String(), err)
	}
	return body
}

// ISSUE-T14-108: the child's group reads as every screen and receipt names it,
// for the school's today, while gradeGroup keeps the bare letter older apps read.
func TestTheFamilyReadsTheGroupInFull(t *testing.T) {
	labeler := &fakeGroupLabeler{label: "3° PRIMARIA · B", found: true}
	h := groupLabelFixture(t, labeler)

	for name, got := range map[string]labeledStudent{"list": listedStudent(t, h), "profile": profiledStudent(t, h)} {
		if got.GroupLabel != "3° PRIMARIA · B" || got.GradeGroup != "B" {
			t.Errorf("%s: groupLabel %q, gradeGroup %q; want the full label and the letter", name, got.GroupLabel, got.GradeGroup)
		}
	}
	if labeler.studentID != "stu-1" || labeler.day != "2026-10-16" {
		t.Errorf("labeled %q on %q, want stu-1 on the school's day 2026-10-16", labeler.studentID, labeler.day)
	}
}

// With no label to give — no enrollment the catalog resolves, a lookup that
// failed, or no labeler wired — groupLabel is the stored letter and the read
// still answers: the label is a name, not a reason to fail the screen.
func TestTheGroupLabelFallsBackToTheLetter(t *testing.T) {
	cases := map[string]*fakeGroupLabeler{
		"not found":     {found: false},
		"lookup failed": {err: errors.New("config down")},
		"not wired":     nil,
	}
	for name, labeler := range cases {
		h := groupLabelFixture(t, labeler)
		for view, got := range map[string]labeledStudent{"list": listedStudent(t, h), "profile": profiledStudent(t, h)} {
			if got.GroupLabel != "B" || got.GradeGroup != "B" {
				t.Errorf("%s (%s): groupLabel %q, gradeGroup %q; want both B", name, view, got.GroupLabel, got.GradeGroup)
			}
		}
	}
}
