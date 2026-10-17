package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// CC-301: a list is who sat in the group THAT day, not who sits in it today.
// Lalo enrolled in A on the 17th; Mía was in A and moved to B on the 17th;
// Rita withdrew from A on the 15th; Toño has been in A all along.
const (
	histGroupA = "grp-hist-a"
	histGroupB = "grp-hist-b"

	enrLalo = "enr-h-lalo"
	enrMia  = "enr-h-mia"
	enrRita = "enr-h-rita"
	enrTono = "enr-h-tono"
)

// morelia is the school's calendar (America/Mexico_City): every instant below
// is read on it, never on UTC.
func morelia(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	return loc
}

func historyService(t *testing.T, repo *fakeRepo) *service.AttendanceService {
	t.Helper()
	mx := morelia(t)
	enrollments := &fakeEnrollments{
		byGroup: map[string][]service.EnrollmentRef{
			histGroupA: {
				{ID: enrTono, StudentID: "stu-tono", GroupID: histGroupA, Status: "active", EnrolledAt: time.Date(2026, 8, 24, 0, 0, 0, 0, mx)},
				// 00:00 Morelia on the 17th is 06:00 UTC: still the 17th at school.
				{ID: enrLalo, StudentID: "stu-lalo", GroupID: histGroupA, Status: "active", EnrolledAt: time.Date(2026, 10, 17, 0, 0, 0, 0, mx)},
				// Moved out since: the enrollment now carries group B.
				{ID: enrMia, StudentID: "stu-mia", GroupID: histGroupB, Status: "active", EnrolledAt: time.Date(2026, 8, 24, 0, 0, 0, 0, mx)},
				// Withdrawn on the 15th, in the shape students really sends:
				// withdrawn_on "2026-10-15" and ended_at its midnight UTC
				// (2026-10-15T00:00Z), which on Morelia's clock is still the
				// 14th. The withdrawal day counts as her last day of class.
				{ID: enrRita, StudentID: "stu-rita", GroupID: histGroupA, Status: "withdrawn", EnrolledAt: time.Date(2026, 8, 24, 0, 0, 0, 0, mx),
					EndedAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), EndedOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)},
			},
			histGroupB: {
				{ID: enrMia, StudentID: "stu-mia", GroupID: histGroupB, Status: "active", EnrolledAt: time.Date(2026, 8, 24, 0, 0, 0, 0, mx)},
			},
		},
		moves: map[string][]service.Move{
			histGroupA: {{EnrollmentID: enrMia, FromGroupID: histGroupA, ToGroupID: histGroupB, MovedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, mx)}},
			histGroupB: {{EnrollmentID: enrMia, FromGroupID: histGroupA, ToGroupID: histGroupB, MovedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, mx)}},
		},
	}
	groups := &fakeGroups{existing: map[string]bool{histGroupA: true, histGroupB: true}}
	return service.NewAttendanceService(repo, enrollments, groups, nil, testTenant, fixedIDs("rec-h1", "rec-h2"), nil,
		service.WithSchoolLocation(mx))
}

func rosterIDs(t *testing.T, svc *service.AttendanceService, group, date string) []string {
	t.Helper()
	rows, err := roster(svc, context.Background(), group, date)
	require.NoError(t, err)
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.EnrollmentID)
	}
	return ids
}

func TestRosterIsWhoWasInTheGroupThatDay(t *testing.T) {
	svc := historyService(t, newFakeRepo())

	tests := []struct {
		name  string
		group string
		date  string
		want  []string
	}{
		{"before Lalo enrolled, with Mía still in A and Rita not yet gone", histGroupA, "2026-10-14", []string{enrMia, enrRita, enrTono}},
		{"Rita's last day is hers (she left that evening)", histGroupA, "2026-10-15", []string{enrMia, enrRita, enrTono}},
		{"the day after Rita left", histGroupA, "2026-10-16", []string{enrMia, enrTono}},
		{"the move day: Mía is on both lists, Lalo's first day", histGroupA, "2026-10-17", []string{enrLalo, enrMia, enrTono}},
		{"after the move, Mía is no longer in A", histGroupA, "2026-10-19", []string{enrLalo, enrTono}},
		{"B before Mía joined does not show her", histGroupB, "2026-10-16", []string{}},
		{"B from the move day on", histGroupB, "2026-10-17", []string{enrMia}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, rosterIDs(t, svc, tt.group, tt.date))
		})
	}
}

// The server refuses a mark for a pupil who was not in the group that day: no
// absence before enrolling (the family was notified of one), none after leaving,
// and none on the new group's list before the move.
func TestSaveRefusesAPupilWhoWasNotInTheGroupThatDay(t *testing.T) {
	tests := []struct {
		name       string
		group      string
		date       string
		enrollment string
	}{
		{"before enrolling", histGroupA, "2026-10-16", enrLalo},
		{"after withdrawing", histGroupA, "2026-10-16", enrRita},
		{"after moving out", histGroupA, "2026-10-19", enrMia},
		{"on the new group's list before the move", histGroupB, "2026-10-16", enrMia},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := historyService(t, repo)
			_, _, err := save(svc, context.Background(), tt.group, tt.date, []service.Mark{{EnrollmentID: tt.enrollment, Status: "absent"}})
			wantInvalid(t, err)
			assert.Zero(t, repo.reconcileCalls, "a refused save writes nothing")
		})
	}
}

// A past list keeps its pupils after a later move: the old group's list of a
// day before the move still holds the pupil, and its reconcile scope is that
// day's members — so a correction on it never drops the moved pupil's mark.
func TestAPastListKeepsAPupilWhoMovedLater(t *testing.T) {
	repo := newFakeRepo()
	svc := historyService(t, repo)

	_, marked, err := save(svc, context.Background(), histGroupA, "2026-10-16", []service.Mark{{EnrollmentID: enrMia, Status: "absent"}})
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	assert.ElementsMatch(t, []string{enrMia, enrTono}, repo.lastReconcileIDs)

	rows, err := roster(svc, context.Background(), histGroupA, "2026-10-16")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, enrMia, rows[0].EnrollmentID)
	assert.Equal(t, "absent", rows[0].Status)
	assert.Equal(t, "stu-mia", rows[0].StudentID)
}

// The report reads the same membership: a pupil moved out of the group is
// still on its report for the days she sat in it, and no longer active there.
func TestSummaryCountsAMovedOutPupilUntilTheMove(t *testing.T) {
	repo := newFakeRepo()
	repo.history = service.GroupHistory{
		Lists: []time.Time{day(t, "2026-10-14"), day(t, "2026-10-16"), day(t, "2026-10-17"), day(t, "2026-10-19")},
	}
	svc := historyService(t, repo)

	got, err := svc.SummarizeAttendance(context.Background(), histGroupA, "2026-10-01", "2026-10-31")
	require.NoError(t, err)

	byID := map[string]service.StudentCounts{}
	for _, line := range got.Students {
		byID[line.EnrollmentID] = line
	}
	assert.Equal(t, 3, byID[enrMia].Lists, "the 14th, 16th and the move day")
	assert.False(t, byID[enrMia].Active, "she left the group")
	assert.Equal(t, 2, byID[enrLalo].Lists, "from enrolling on the 17th")
	assert.Equal(t, 1, byID[enrRita].Lists, "until withdrawing on the 15th")
	assert.Equal(t, 4, byID[enrTono].Lists)
}
