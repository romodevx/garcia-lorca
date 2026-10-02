package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T12: the attendance report counts a group's lists and, per ACTIVE pupil,
// the exceptions on those lists — zeros included, in roster order.
func TestSummarizeAttendanceCountsEveryActivePupil(t *testing.T) {
	repo := newFakeRepo()
	repo.summary = service.GroupSummary{
		ListsTaken: 12,
		Counts: map[string]service.ExceptionCounts{
			enrollmentAna:  {Absent: 2, Late: 3},
			enrollmentCruz: {Excused: 1},
		},
	}
	svc := newService(t, repo, nil)

	got, err := svc.SummarizeAttendance(context.Background(), groupID, "2026-08-01", "2026-08-31")
	require.NoError(t, err)

	assert.Equal(t, 12, got.ListsTaken)
	assert.Equal(t, []service.StudentCounts{
		{EnrollmentID: enrollmentAna, StudentID: studentAna, ExceptionCounts: service.ExceptionCounts{Absent: 2, Late: 3}},
		{EnrollmentID: enrollmentBeto, StudentID: studentBeto},
		{EnrollmentID: enrollmentCruz, StudentID: studentCruz, ExceptionCounts: service.ExceptionCounts{Excused: 1}},
	}, got.Students)
	assert.Equal(t, groupID, repo.lastSummaryGroup)
	assert.Equal(t, []string{enrollmentAna, enrollmentBeto, enrollmentCruz}, repo.lastSummaryIDs, "the withdrawn pupil is not asked for")
	assert.Equal(t, day(t, "2026-08-01"), repo.lastSummaryFrom)
	assert.Equal(t, day(t, "2026-08-31"), repo.lastSummaryTo)
}

func TestSummarizeAttendanceRejectsBadQueries(t *testing.T) {
	tests := []struct{ name, group, from, to string }{
		{"unknown group", "grp-nope", "2026-08-01", "2026-08-31"},
		{"missing from", groupID, "", "2026-08-31"},
		{"malformed to", groupID, "2026-08-01", "31/08/2026"},
		{"reversed range", groupID, "2026-08-31", "2026-08-01"},
		{"more than a year", groupID, "2025-08-01", "2026-08-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)
			_, err := svc.SummarizeAttendance(context.Background(), tt.group, tt.from, tt.to)
			wantInvalid(t, err)
			assert.Zero(t, repo.summaryCalls, "a refused query never reaches the database")
		})
	}
}

func TestSummarizeAttendanceAcceptsAWholeYear(t *testing.T) {
	svc := newService(t, newFakeRepo(), nil)
	_, err := svc.SummarizeAttendance(context.Background(), groupID, "2025-08-01", "2026-07-31")
	require.NoError(t, err)
}

func TestSummarizeAttendancePropagatesRepoFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("db down")
	svc := newService(t, repo, nil)
	_, err := svc.SummarizeAttendance(context.Background(), groupID, "2026-08-01", "2026-08-31")
	require.Error(t, err)
}
