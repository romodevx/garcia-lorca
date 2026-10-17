package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// The report's group and pupils: Ana all month, Beto moved in on the 20th,
// Cruz enrolled on the 10th, Dora withdrew on the 5th, Eva withdrew in August,
// Fede joined after the range.
const (
	reportGroup = "grp-r"
	enrAna      = "enr-1"
	enrBeto     = "enr-2"
	enrCruz     = "enr-3"
	enrDora     = "enr-4"
	enrEva      = "enr-5"
	enrFede     = "enr-6"
)

func at(t *testing.T, s string) time.Time { return day(t, s).Add(13 * time.Hour) }

func reportService(t *testing.T, repo *fakeRepo) *service.AttendanceService {
	t.Helper()
	enrollments := &fakeEnrollments{
		byGroup: map[string][]service.EnrollmentRef{reportGroup: {
			{ID: enrFede, StudentID: "stu-6", Status: "active", EnrolledAt: at(t, "2026-10-02")},
			{ID: enrAna, StudentID: "stu-1", Status: "active", EnrolledAt: at(t, "2026-08-01")},
			{ID: enrBeto, StudentID: "stu-2", Status: "active", EnrolledAt: at(t, "2026-08-01")},
			{ID: enrCruz, StudentID: "stu-3", Status: "active", EnrolledAt: at(t, "2026-09-10")},
			{ID: enrDora, StudentID: "stu-4", Status: "withdrawn", EnrolledAt: at(t, "2026-08-01"), EndedAt: at(t, "2026-09-05")},
			{ID: enrEva, StudentID: "stu-5", Status: "withdrawn", EnrolledAt: at(t, "2026-08-01"), EndedAt: at(t, "2026-08-20")},
		}},
		moves: map[string][]service.Move{reportGroup: {
			{EnrollmentID: enrBeto, FromGroupID: "grp-old", ToGroupID: reportGroup, MovedAt: at(t, "2026-09-01")},
			{EnrollmentID: enrBeto, ToGroupID: reportGroup, MovedAt: at(t, "2026-09-20")}, // the latest move in counts
		}},
	}
	groups := &fakeGroups{existing: map[string]bool{reportGroup: true}}
	return service.NewAttendanceService(repo, enrollments, groups, nil, testTenant, fixedIDs(), nil)
}

// T12: a pupil is counted only on the group's lists while they were in it —
// from enrolling (or the latest move into the group) to leaving — and the
// marks outside that window (an old group's legacy marks) are not theirs.
func TestSummarizeAttendanceCountsEachPupilInsideTheirWindow(t *testing.T) {
	repo := newFakeRepo()
	repo.history = service.GroupHistory{
		Lists: []time.Time{day(t, "2026-09-01"), day(t, "2026-09-04"), day(t, "2026-09-10"), day(t, "2026-09-10"), day(t, "2026-09-21"), day(t, "2026-09-30")},
		Marks: []service.DayMark{
			{EnrollmentID: enrAna, Date: day(t, "2026-09-04"), Status: "late"},
			{EnrollmentID: enrAna, Date: day(t, "2026-09-10"), Status: "absent"},
			{EnrollmentID: enrBeto, Date: day(t, "2026-09-04"), Status: "absent"}, // legacy, before he moved in
			{EnrollmentID: enrBeto, Date: day(t, "2026-09-21"), Status: "late"},
			{EnrollmentID: enrDora, Date: day(t, "2026-09-04"), Status: "excused"},
		},
	}
	svc := reportService(t, repo)

	got, err := svc.SummarizeAttendance(context.Background(), reportGroup, "2026-09-01", "2026-09-30")
	require.NoError(t, err)

	assert.Equal(t, 6, got.ListsTaken)
	assert.Equal(t, []service.StudentCounts{
		{EnrollmentID: enrAna, StudentID: "stu-1", Active: true, Lists: 6, ExceptionCounts: service.ExceptionCounts{Absent: 1, Late: 1}},
		{EnrollmentID: enrBeto, StudentID: "stu-2", Active: true, Lists: 2, ExceptionCounts: service.ExceptionCounts{Late: 1}},
		{EnrollmentID: enrCruz, StudentID: "stu-3", Active: true, Lists: 4},
		{EnrollmentID: enrDora, StudentID: "stu-4", Active: false, Lists: 2, ExceptionCounts: service.ExceptionCounts{Excused: 1}},
	}, got.Students, "Eva left before the range and Fede joined after it")
	assert.Equal(t, reportGroup, repo.lastHistoryGroup)
	assert.ElementsMatch(t, []string{enrAna, enrBeto, enrCruz, enrDora}, repo.lastHistoryIDs)
	assert.Equal(t, day(t, "2026-09-01"), repo.lastHistoryFrom)
	assert.Equal(t, day(t, "2026-09-30"), repo.lastHistoryTo)
}

func TestSummarizeAttendanceRejectsBadQueries(t *testing.T) {
	tests := []struct{ name, group, from, to string }{
		{"unknown group", "grp-nope", "2026-08-01", "2026-08-31"},
		{"missing from", reportGroup, "", "2026-08-31"},
		{"malformed to", reportGroup, "2026-08-01", "31/08/2026"},
		{"reversed range", reportGroup, "2026-08-31", "2026-08-01"},
		{"more than a year", reportGroup, "2025-08-01", "2026-08-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, err := reportService(t, repo).SummarizeAttendance(context.Background(), tt.group, tt.from, tt.to)
			wantInvalid(t, err)
			assert.Zero(t, repo.historyCalls, "a refused query never reaches the database")
		})
	}
}

func TestSummarizeAttendanceAcceptsAWholeYear(t *testing.T) {
	_, err := reportService(t, newFakeRepo()).SummarizeAttendance(context.Background(), reportGroup, "2025-08-01", "2026-07-31")
	require.NoError(t, err)
}

func TestSummarizeAttendancePropagatesFailures(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("db down")
	_, err := reportService(t, repo).SummarizeAttendance(context.Background(), reportGroup, "2026-09-01", "2026-09-30")
	require.Error(t, err)
}

// A pupil enrolled at 18:12 in Mexico City (00:12 UTC the next day) is in the
// group for that day's list: the window is read on the school's own calendar.
func TestSummarizeAttendanceReadsTheWindowInTheSchoolsZone(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	repo := newFakeRepo()
	repo.history = service.GroupHistory{Lists: []time.Time{day(t, "2026-09-27")}}
	enrollments := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{reportGroup: {
		{ID: enrAna, StudentID: "stu-1", Status: "active", EnrolledAt: time.Date(2026, 9, 28, 0, 12, 0, 0, time.UTC)},
	}}}
	groups := &fakeGroups{existing: map[string]bool{reportGroup: true}}
	svc := service.NewAttendanceService(repo, enrollments, groups, nil, testTenant, fixedIDs(), nil, service.WithSchoolLocation(mexico))

	got, err := svc.SummarizeAttendance(context.Background(), reportGroup, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	require.Len(t, got.Students, 1)
	assert.Equal(t, 1, got.Students[0].Lists)
}

// T14: an imported pupil is enrolled from the cycle's first day at 00:00 in
// Morelia (06:00 UTC), so every list from that day on is theirs — none taken
// before the import day reads as "lists counted, zero present" any more.
func TestSummarizeAttendanceCountsAnImportedPupilFromTheCycleStart(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	repo := newFakeRepo()
	repo.history = service.GroupHistory{
		Lists: []time.Time{day(t, "2026-08-24"), day(t, "2026-09-15"), day(t, "2026-10-16")},
		Marks: []service.DayMark{{EnrollmentID: enrAna, Date: day(t, "2026-09-15"), Status: "absent"}},
	}
	enrollments := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{reportGroup: {
		{ID: enrAna, StudentID: "stu-1", Status: "active", EnrolledAt: time.Date(2026, 8, 24, 0, 0, 0, 0, mexico)},
	}}}
	groups := &fakeGroups{existing: map[string]bool{reportGroup: true}}
	svc := service.NewAttendanceService(repo, enrollments, groups, nil, testTenant, fixedIDs(), nil, service.WithSchoolLocation(mexico))

	got, err := svc.SummarizeAttendance(context.Background(), reportGroup, "2026-08-24", "2026-10-17")
	require.NoError(t, err)
	require.Len(t, got.Students, 1)
	assert.Equal(t, 3, got.Students[0].Lists, "every list from the cycle's first day")
	assert.Equal(t, 1, got.Students[0].Absent)
}
