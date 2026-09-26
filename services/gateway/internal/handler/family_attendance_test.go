package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// familyToday is the fixed school day of these tests: 2026-09-17 (UTC).
var familyToday = time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)

type familyAttendanceClass struct {
	SubjectName string `json:"subjectName"`
	Status      string `json:"status"`
	Note        string `json:"note"`
}

type familyAttendanceDay struct {
	Date    string                  `json:"date"`
	Status  string                  `json:"status"`
	Mode    string                  `json:"mode"`
	Note    string                  `json:"note"`
	Classes []familyAttendanceClass `json:"classes"`
}

type familyAttendanceResponse struct {
	Month   string                `json:"month"`
	Today   string                `json:"today"`
	Days    []familyAttendanceDay `json:"days"`
	Summary map[string]int        `json:"summary"`
}

const (
	grpDaily    = "grp-daily"
	grpPerClass = "grp-sec"
)

func familyAttendanceHandler(students *fakeFamilyStudents, att *fakeFamilyAttendance, catalog *fakeFamilyConfig) *handler.FamilyHandler {
	return familyAttendanceHandlerAt(students, att, catalog, familyToday)
}

func familyAttendanceHandlerAt(students *fakeFamilyStudents, att *fakeFamilyAttendance, catalog *fakeFamilyConfig, now time.Time) *handler.FamilyHandler {
	if catalog == nil {
		catalog = &fakeFamilyConfig{}
	}
	return handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, att, catalog, handler.WithFamilyClock(func() time.Time { return now }))
}

func getFamilyAttendance(t *testing.T, h *handler.FamilyHandler, query string) (*httptest.ResponseRecorder, familyAttendanceResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Attendance(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/attendance"+query, "usr-9", "studentId", "stu-1"))
	var out familyAttendanceResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	}
	return rec, out
}

func childIn(groupID string) *fakeFamilyStudents {
	s := oneChild()
	s.enrollments = []client.Enrollment{{ID: "enr-1", StudentID: "stu-1", GroupID: groupID, Status: "active"}}
	return s
}

func dayOf(t *testing.T, days []familyAttendanceDay, date string) familyAttendanceDay {
	t.Helper()
	for _, d := range days {
		if d.Date == date {
			return d
		}
	}
	t.Fatalf("no day %s in %+v", date, days)
	return familyAttendanceDay{}
}

// A daily group: a taken list with no exception is "present"; an exception is
// its status with its note; today with no list is "not_taken"; a day nobody took
// does not appear. Newest first, and the summary counts DAYS.
func TestFamilyAttendanceDailyDays(t *testing.T) {
	att := &fakeFamilyAttendance{
		sessions: []client.TakenList{
			{GroupID: grpDaily, Date: "2026-09-16"},
			{GroupID: grpDaily, Date: "2026-09-15"},
			{GroupID: grpDaily, Date: "2026-09-14"},
		},
		byEnrollment: map[string][]client.AttendanceRecord{"enr-1": {
			{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "absent", Note: "cita médica"},
			{EnrollmentID: "enr-1", Date: "2026-09-14", Status: "late"},
		}},
	}
	rec, out := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), att, nil), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	assert.Equal(t, "2026-09", out.Month, "defaults to the current UTC month")
	assert.Equal(t, "2026-09-17", out.Today)
	dates := make([]string, 0, len(out.Days))
	for _, d := range out.Days {
		dates = append(dates, d.Date)
	}
	assert.Equal(t, []string{"2026-09-17", "2026-09-16", "2026-09-15", "2026-09-14"}, dates, "newest first; untaken days absent")

	assert.Equal(t, familyAttendanceDay{Date: "2026-09-17", Status: "not_taken", Mode: "daily", Classes: []familyAttendanceClass{}}, out.Days[0])
	assert.Equal(t, familyAttendanceDay{Date: "2026-09-16", Status: "present", Mode: "daily", Classes: []familyAttendanceClass{}}, out.Days[1])
	assert.Equal(t, familyAttendanceDay{Date: "2026-09-15", Status: "absent", Mode: "daily", Note: "cita médica", Classes: []familyAttendanceClass{}}, out.Days[2])
	assert.Equal(t, "late", out.Days[3].Status)
	assert.Equal(t, map[string]int{"present": 1, "absent": 1, "late": 1, "excused": 0, "partial": 0}, out.Summary)

	assert.Equal(t, 1, att.sessionsCalls, "one batched read of the taken lists")
	assert.Equal(t, []string{grpDaily}, att.gotSessionGroups)
	assert.Equal(t, "2026-09-01", att.gotSessionsFrom)
	assert.Equal(t, "2026-09-30", att.gotSessionsTo)
	assert.Equal(t, "2026-09-01", att.gotFrom)
	assert.Equal(t, "2026-09-30", att.gotTo)
}

// A per-class group: classes = every taken list with the child's status; the
// day is present when all are, absent when absent in all, the shared status when
// all share one, partial otherwise. Class names come from the catalog.
func TestFamilyAttendancePerClassDays(t *testing.T) {
	sessions := func(date string, subjects ...string) []client.TakenList {
		out := make([]client.TakenList, 0, len(subjects))
		for _, s := range subjects {
			out = append(out, client.TakenList{GroupID: grpPerClass, Date: date, SubjectID: s})
		}
		return out
	}
	var all []client.TakenList
	all = append(all, sessions("2026-09-17", "sub-mat", "sub-esp")...)
	all = append(all, sessions("2026-09-16", "sub-mat", "sub-esp")...)
	all = append(all, sessions("2026-09-15", "sub-mat", "sub-esp")...)
	all = append(all, sessions("2026-09-14", "sub-mat", "sub-esp")...)
	att := &fakeFamilyAttendance{
		sessions: all,
		byEnrollment: map[string][]client.AttendanceRecord{"enr-1": {
			{EnrollmentID: "enr-1", Date: "2026-09-16", Status: "absent", SubjectID: "sub-mat"},
			{EnrollmentID: "enr-1", Date: "2026-09-16", Status: "absent", SubjectID: "sub-esp"},
			{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "late", SubjectID: "sub-mat", Note: "8:20"},
			{EnrollmentID: "enr-1", Date: "2026-09-14", Status: "late", SubjectID: "sub-mat"},
			{EnrollmentID: "enr-1", Date: "2026-09-14", Status: "late", SubjectID: "sub-esp"},
		}},
	}
	catalog := &fakeFamilyConfig{subjects: []client.SubjectResult{{ID: "sub-mat", Name: "Matemáticas"}, {ID: "sub-esp", Name: "Español"}}}
	rec, out := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpPerClass), att, catalog), "?month=2026-09")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	today := dayOf(t, out.Days, "2026-09-17")
	assert.Equal(t, "present", today.Status, "taken today, all present")
	assert.Equal(t, "per_class", today.Mode)
	assert.Equal(t, []familyAttendanceClass{{SubjectName: "Español", Status: "present"}, {SubjectName: "Matemáticas", Status: "present"}}, today.Classes, "sorted by name")

	assert.Equal(t, "absent", dayOf(t, out.Days, "2026-09-16").Status)
	partial := dayOf(t, out.Days, "2026-09-15")
	assert.Equal(t, "partial", partial.Status)
	assert.Contains(t, partial.Classes, familyAttendanceClass{SubjectName: "Matemáticas", Status: "late", Note: "8:20"})
	assert.Equal(t, "late", dayOf(t, out.Days, "2026-09-14").Status, "late in every class")
	assert.Equal(t, map[string]int{"present": 1, "absent": 1, "late": 1, "excused": 0, "partial": 1}, out.Summary)
	assert.Equal(t, 1, catalog.gotListSubjects, "names resolved once")
}

// A past month never shows today; an exception with no recorded list still
// appears (it was stored, so the day happened).
func TestFamilyAttendancePastMonth(t *testing.T) {
	att := &fakeFamilyAttendance{byEnrollment: map[string][]client.AttendanceRecord{"enr-1": {
		{EnrollmentID: "enr-1", Date: "2026-08-20", Status: "excused"},
	}}}
	rec, out := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), att, nil), "?month=2026-08")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "2026-08", out.Month)
	require.Len(t, out.Days, 1)
	assert.Equal(t, "excused", out.Days[0].Status)
	assert.Equal(t, "2026-08-31", att.gotSessionsTo)
	assert.Equal(t, 1, out.Summary["excused"])
}

func TestFamilyAttendanceRejectsAMalformedMonth(t *testing.T) {
	for _, month := range []string{"2026-13", "2026-9", "sept", "2026-09-01", "0000-00"} {
		t.Run(month, func(t *testing.T) {
			att := &fakeFamilyAttendance{}
			rec, _ := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), att, nil), "?month="+month)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, att.sessionsCalls)
		})
	}
}

// A child with no enrollment has no lists: only today, not taken.
func TestFamilyAttendanceWithoutEnrollmentsShowsOnlyToday(t *testing.T) {
	att := &fakeFamilyAttendance{}
	rec, out := getFamilyAttendance(t, familyAttendanceHandler(oneChild(), att, nil), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Zero(t, att.sessionsCalls)
	require.Len(t, out.Days, 1)
	assert.Equal(t, "not_taken", out.Days[0].Status)
}

func TestFamilyAttendanceSessionsErrorFailsTheRead(t *testing.T) {
	att := &fakeFamilyAttendance{sessionsErr: errString("attendance down")}
	rec, _ := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), att, nil), "")
	assert.GreaterOrEqual(t, rec.Code, 500)
}
