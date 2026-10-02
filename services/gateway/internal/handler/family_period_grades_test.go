package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeEnrollmentPeriods struct {
	got   string
	calls int
	res   client.EnrollmentPeriodGrades
}

func (f *fakeEnrollmentPeriods) GetEnrollmentPeriodGrades(_ context.Context, enrollmentID string) (client.EnrollmentPeriodGrades, error) {
	f.calls++
	f.got = enrollmentID
	return f.res, nil
}

type fakeSubjectNames struct{ subjects []client.SubjectResult }

func (f fakeSubjectNames) ListSubjects(_ context.Context, _ string) ([]client.SubjectResult, error) {
	return f.subjects, nil
}

// fakeCycles lists the school's cycles.
type fakeCycles struct{ cycles []client.CycleResult }

func (f fakeCycles) ListCycles(context.Context) ([]client.CycleResult, error) { return f.cycles, nil }

var (
	cycleNow     = client.CycleResult{ID: "cyc-1", Name: "2026-2027", Status: "active", StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC)}
	cycleNext    = client.CycleResult{ID: "cyc-2", Name: "2027-2028", Status: "draft", StartsOn: time.Date(2027, 8, 23, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2028, 7, 7, 0, 0, 0, 0, time.UTC)}
	schoolCycles = fakeCycles{cycles: []client.CycleResult{cycleNext, cycleNow}}
)

func newFamilyPeriods(students handler.FamilyEnrollmentReader, grades handler.EnrollmentPeriodGradesReader, subjects handler.SubjectNamesReader) *handler.FamilyPeriodGradesHandler {
	return handler.NewFamilyPeriodGradesHandler(students, grades, subjects, schoolCycles)
}

func familyPeriodStudents() *fakeFamilyStudents {
	return &fakeFamilyStudents{
		links: []client.GuardianLink{{ID: "gl-1", GuardianUserID: "usr-9", StudentID: "stu-1"}},
		enrollments: []client.Enrollment{
			{ID: "enr-old", StudentID: "stu-1", Status: "withdrawn", CycleID: "cyc-0"},
			{ID: "enr-1", StudentID: "stu-1", Status: "active", CycleID: "cyc-1"},
		},
	}
}

var periodScale = client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}

var familyPeriods = []client.EvaluationPeriodRef{
	{ID: "per-1", Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"},
	{ID: "per-2", Ordinal: 2, Name: "2.º trimestre", StartsOn: "2026-12-07", EndsOn: "2027-03-26"},
}

func familyPeriodsReq(userID, studentID string) *http.Request {
	return guardianRequest(http.MethodGet, "/api/v1/family/students/"+studentID+"/grades/periods", userID, "studentId", studentID)
}

func TestFamilyPeriodGradesShape(t *testing.T) {
	grades := &fakeEnrollmentPeriods{res: client.EnrollmentPeriodGrades{
		CycleID: "cyc-1", CycleName: "2026-2027", Periods: familyPeriods, Scale: periodScale,
		GradingEnabled: true, SubjectIDs: []string{"sub-mat", "sub-esp", "sub-art"},
		Grades: []client.PeriodGrade{
			{SubjectID: "sub-mat", PeriodID: "per-2", Value: "9"},
			{SubjectID: "sub-mat", PeriodID: "per-1", Value: "10"},
			{SubjectID: "sub-esp", PeriodID: "per-2", Value: "7"},
		},
	}}
	subjects := fakeSubjectNames{subjects: []client.SubjectResult{
		{ID: "sub-mat", Name: "Matemáticas"}, {ID: "sub-esp", Name: "Español"}, {ID: "sub-art", Name: "Artes"},
	}}
	h := newFamilyPeriods(familyPeriodStudents(), grades, subjects)

	rec := httptest.NewRecorder()
	h.PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-1"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "enr-1", grades.got, "the child's ACTIVE enrollment")
	assert.JSONEq(t, `{
		"gradingEnabled": true, "cycleName": "2026-2027",
		"periods": [
			{"id":"per-1","ordinal":1,"name":"1.er trimestre","startsOn":"2026-08-24","endsOn":"2026-11-30"},
			{"id":"per-2","ordinal":2,"name":"2.º trimestre","startsOn":"2026-12-07","endsOn":"2027-03-26"}
		],
		"scale": {"min":5,"max":10,"decimals":1,"passing":6},
		"subjects": [
			{"subjectId":"sub-art","subjectName":"Artes","grades":{}},
			{"subjectId":"sub-esp","subjectName":"Español","grades":{"per-2":7}},
			{"subjectId":"sub-mat","subjectName":"Matemáticas","grades":{"per-1":10,"per-2":9}}
		]}`, rec.Body.String(), "subjects sorted by name (es); nothing captured ⇒ an empty object")
}

func TestFamilyPeriodGradesPreescolar(t *testing.T) {
	grades := &fakeEnrollmentPeriods{res: client.EnrollmentPeriodGrades{
		CycleName: "2026-2027", Periods: familyPeriods[:1], Scale: periodScale, GradingEnabled: false,
	}}
	h := newFamilyPeriods(familyPeriodStudents(), grades, fakeSubjectNames{})
	rec := httptest.NewRecorder()
	h.PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-1"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"gradingEnabled":false,"cycleName":"2026-2027",
		"periods":[{"id":"per-1","ordinal":1,"name":"1.er trimestre","startsOn":"2026-08-24","endsOn":"2026-11-30"}],"subjects":[]}`, rec.Body.String(),
		"a level without numeric grading sends no scale (its current one is not the one to judge by)")
}

func TestFamilyPeriodGradesOwnershipAndEnrollment(t *testing.T) {
	grades := &fakeEnrollmentPeriods{}
	h := newFamilyPeriods(familyPeriodStudents(), grades, fakeSubjectNames{})

	rec := httptest.NewRecorder()
	h.PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-someone-else"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another family's child is 404")

	noActive := familyPeriodStudents()
	noActive.enrollments = []client.Enrollment{{ID: "enr-old", StudentID: "stu-1", Status: "withdrawn"}}
	rec = httptest.NewRecorder()
	newFamilyPeriods(noActive, grades, fakeSubjectNames{}).PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-1"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "no active enrollment ⇒ nothing to show")
	assert.Zero(t, grades.calls, "grades is never asked about a child that is not the caller's or not enrolled")

	rec = httptest.NewRecorder()
	h.PeriodGrades(rec, httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades/periods", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// A child re-enrolled for next year has TWO active enrollments: the parent sees
// the one whose cycle is ACTIVE, whatever order students lists them in; with no
// active cycle, the one whose dates contain today (UTC).
func TestFamilyPeriodGradesPicksTheCurrentCycle(t *testing.T) {
	twoActive := func(order ...client.Enrollment) *fakeFamilyStudents {
		s := familyPeriodStudents()
		s.enrollments = order
		return s
	}
	current := client.Enrollment{ID: "enr-now", StudentID: "stu-1", Status: "active", CycleID: "cyc-1"}
	next := client.Enrollment{ID: "enr-next", StudentID: "stu-1", Status: "active", CycleID: "cyc-2"}
	inactiveNow := cycleNow
	inactiveNow.Status = "closed"
	tests := []struct {
		name     string
		students *fakeFamilyStudents
		cycles   fakeCycles
		today    time.Time
		want     string
	}{
		{"active cycle, listed first", twoActive(current, next), schoolCycles, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "enr-now"},
		{"active cycle, listed last", twoActive(next, current), schoolCycles, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "enr-now"},
		{"no active cycle: the one containing today", twoActive(next, current), fakeCycles{cycles: []client.CycleResult{cycleNext, inactiveNow}}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "enr-now"},
		{"no active cycle, today in next year", twoActive(current, next), fakeCycles{cycles: []client.CycleResult{cycleNext, inactiveNow}}, time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC), "enr-next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakeEnrollmentPeriods{res: client.EnrollmentPeriodGrades{GradingEnabled: true}}
			h := handler.NewFamilyPeriodGradesHandler(tt.students, grades, fakeSubjectNames{}, tt.cycles).WithClock(func() time.Time { return tt.today })
			rec := httptest.NewRecorder()
			h.PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-1"))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tt.want, grades.got)
		})
	}
}

// A level that stopped grading numerically still shows the grades already
// captured — exactly as stored — and omits the scale: grades do not record the
// scale they were captured under, and the level's current one (often 0–0)
// would mark every grade as failing.
func TestFamilyPeriodGradesKeepsCapturedGradesWhenGradingIsOff(t *testing.T) {
	grades := &fakeEnrollmentPeriods{res: client.EnrollmentPeriodGrades{
		CycleName: "2026-2027", Scale: periodScale, GradingEnabled: false,
		SubjectIDs: []string{"sub-mat"}, Grades: []client.PeriodGrade{{SubjectID: "sub-mat", PeriodID: "per-1", Value: "9"}},
	}}
	h := newFamilyPeriods(familyPeriodStudents(), grades, fakeSubjectNames{subjects: []client.SubjectResult{{ID: "sub-mat", Name: "Matemáticas"}}})
	rec := httptest.NewRecorder()
	h.PeriodGrades(rec, familyPeriodsReq("usr-9", "stu-1"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"gradingEnabled":false,"cycleName":"2026-2027","periods":[],
		"subjects":[{"subjectId":"sub-mat","subjectName":"Matemáticas","grades":{"per-1":9}}]}`, rec.Body.String())
}
