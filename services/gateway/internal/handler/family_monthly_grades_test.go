package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeEnrollmentMonthly struct {
	got   string
	calls int
	res   client.EnrollmentMonthlyGrades
}

func (f *fakeEnrollmentMonthly) GetEnrollmentMonthlyGrades(_ context.Context, enrollmentID string) (client.EnrollmentMonthlyGrades, error) {
	f.calls++
	f.got = enrollmentID
	return f.res, nil
}

type fakeSubjectNames struct{ subjects []client.SubjectResult }

func (f fakeSubjectNames) ListSubjects(_ context.Context, _ string) ([]client.SubjectResult, error) {
	return f.subjects, nil
}

func familyMonthlyStudents() *fakeFamilyStudents {
	return &fakeFamilyStudents{
		links: []client.GuardianLink{{ID: "gl-1", GuardianUserID: "usr-9", StudentID: "stu-1"}},
		enrollments: []client.Enrollment{
			{ID: "enr-old", StudentID: "stu-1", Status: "withdrawn"},
			{ID: "enr-1", StudentID: "stu-1", Status: "active"},
		},
	}
}

var monthlyScale = client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}

func familyMonthlyReq(userID, studentID string) *http.Request {
	return guardianRequest(http.MethodGet, "/api/v1/family/students/"+studentID+"/grades/monthly", userID, "studentId", studentID)
}

func TestFamilyMonthlyGradesShape(t *testing.T) {
	grades := &fakeEnrollmentMonthly{res: client.EnrollmentMonthlyGrades{
		CycleID: "cyc-1", CycleName: "2026-2027", Months: []string{"2026-08", "2026-09"}, Scale: monthlyScale,
		GradingEnabled: true, SubjectIDs: []string{"sub-mat", "sub-esp", "sub-art"},
		Grades: []client.MonthlyGrade{
			{SubjectID: "sub-mat", Month: "2026-09", Value: "9.5"},
			{SubjectID: "sub-mat", Month: "2026-08", Value: "10"},
			{SubjectID: "sub-esp", Month: "2026-09", Value: "7"},
		},
	}}
	subjects := fakeSubjectNames{subjects: []client.SubjectResult{
		{ID: "sub-mat", Name: "Matemáticas"}, {ID: "sub-esp", Name: "Español"}, {ID: "sub-art", Name: "Artes"},
	}}
	h := handler.NewFamilyMonthlyGradesHandler(familyMonthlyStudents(), grades, subjects)

	rec := httptest.NewRecorder()
	h.MonthlyGrades(rec, familyMonthlyReq("usr-9", "stu-1"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "enr-1", grades.got, "the child's ACTIVE enrollment")
	assert.JSONEq(t, `{
		"gradingEnabled": true, "cycleName": "2026-2027", "months": ["2026-08","2026-09"],
		"scale": {"min":5,"max":10,"decimals":1,"passing":6},
		"subjects": [
			{"subjectId":"sub-art","subjectName":"Artes","grades":{}},
			{"subjectId":"sub-esp","subjectName":"Español","grades":{"2026-09":7}},
			{"subjectId":"sub-mat","subjectName":"Matemáticas","grades":{"2026-08":10,"2026-09":9.5}}
		]}`, rec.Body.String(), "subjects sorted by name (es); nothing captured ⇒ an empty object")
}

func TestFamilyMonthlyGradesPreescolar(t *testing.T) {
	grades := &fakeEnrollmentMonthly{res: client.EnrollmentMonthlyGrades{
		CycleName: "2026-2027", Months: []string{"2026-08"}, Scale: monthlyScale, GradingEnabled: false,
	}}
	h := handler.NewFamilyMonthlyGradesHandler(familyMonthlyStudents(), grades, fakeSubjectNames{})
	rec := httptest.NewRecorder()
	h.MonthlyGrades(rec, familyMonthlyReq("usr-9", "stu-1"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"gradingEnabled":false,"cycleName":"2026-2027","months":["2026-08"],
		"scale":{"min":5,"max":10,"decimals":1,"passing":6},"subjects":[]}`, rec.Body.String())
}

func TestFamilyMonthlyGradesOwnershipAndEnrollment(t *testing.T) {
	grades := &fakeEnrollmentMonthly{}
	h := handler.NewFamilyMonthlyGradesHandler(familyMonthlyStudents(), grades, fakeSubjectNames{})

	rec := httptest.NewRecorder()
	h.MonthlyGrades(rec, familyMonthlyReq("usr-9", "stu-someone-else"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another family's child is 404")

	noActive := familyMonthlyStudents()
	noActive.enrollments = []client.Enrollment{{ID: "enr-old", StudentID: "stu-1", Status: "withdrawn"}}
	rec = httptest.NewRecorder()
	handler.NewFamilyMonthlyGradesHandler(noActive, grades, fakeSubjectNames{}).MonthlyGrades(rec, familyMonthlyReq("usr-9", "stu-1"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "no active enrollment ⇒ nothing to show")
	assert.Zero(t, grades.calls, "grades is never asked about a child that is not the caller's or not enrolled")

	rec = httptest.NewRecorder()
	h.MonthlyGrades(rec, httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
