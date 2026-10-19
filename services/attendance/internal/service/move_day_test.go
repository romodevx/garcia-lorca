package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// C2-011: on any school day a pupil counts in exactly ONE list — the group
// whose list already holds a mark for them that day, if one does; otherwise
// the group they are in at the end of that day. Mía moved from A to B at noon
// on the 17th (historyService).
func TestAMovedPupilIsOnExactlyOneListOnTheMoveDay(t *testing.T) {
	const moveDay = "2026-10-17"
	tests := []struct {
		name       string
		markIn     string // group whose list marks Mía on the move day ("" = nobody)
		status     string
		wantA      []string
		wantB      []string
		wantStatus string // Mía's status on the list that shows her
	}{
		{
			name:  "nobody marked her: only the new group lists her, as present",
			wantA: []string{enrLalo, enrTono}, wantB: []string{enrMia}, wantStatus: "present",
		},
		{
			name:   "the old group marked her excused: it keeps her, the new group does not list her",
			markIn: histGroupA, status: "excused",
			wantA: []string{enrLalo, enrMia, enrTono}, wantB: []string{}, wantStatus: "excused",
		},
		{
			name:   "the new group marked her late: only the new group lists her",
			markIn: histGroupB, status: "late",
			wantA: []string{enrLalo, enrTono}, wantB: []string{enrMia}, wantStatus: "late",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := historyService(t, newFakeRepo())
			if tt.markIn != "" {
				_, _, err := save(svc, context.Background(), tt.markIn, moveDay, []service.Mark{{EnrollmentID: enrMia, Status: tt.status}})
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantA, rosterIDs(t, svc, histGroupA, moveDay))
			assert.Equal(t, tt.wantB, rosterIDs(t, svc, histGroupB, moveDay))
			assert.Equal(t, tt.wantStatus, statusOf(t, svc, moveDay, enrMia))
		})
	}
}

// From the day after the move only the new group lists her, whatever the old
// group marked on the move day.
func TestAMovedPupilIsOnlyOnTheNewListAfterTheMoveDay(t *testing.T) {
	svc := historyService(t, newFakeRepo())
	_, _, err := save(svc, context.Background(), histGroupA, "2026-10-17", []service.Mark{{EnrollmentID: enrMia, Status: "excused"}})
	require.NoError(t, err)

	assert.Equal(t, []string{enrLalo, enrTono}, rosterIDs(t, svc, histGroupA, "2026-10-19"))
	assert.Equal(t, []string{enrMia}, rosterIDs(t, svc, histGroupB, "2026-10-19"))
}

// statusOf is the pupil's status on whichever of the two lists shows them;
// it fails when both do (the bug) or neither does.
func statusOf(t *testing.T, svc *service.AttendanceService, date, enrollmentID string) string {
	t.Helper()
	var found []string
	for _, group := range []string{histGroupA, histGroupB} {
		rows, err := roster(svc, context.Background(), group, date)
		require.NoError(t, err)
		for _, r := range rows {
			if r.EnrollmentID == enrollmentID {
				found = append(found, r.Status)
			}
		}
	}
	require.Len(t, found, 1, "the pupil is on exactly one list")
	return found[0]
}

// The group report counts the move day once: in the group holding the mark,
// or, with none, in the new group — never as a present day in both.
func TestSummaryCountsTheMoveDayInExactlyOneGroup(t *testing.T) {
	lists := []time.Time{day(t, "2026-10-16"), day(t, "2026-10-17"), day(t, "2026-10-19")}
	excusedInA := service.DayMark{EnrollmentID: enrMia, Date: day(t, "2026-10-17"), Status: "excused"}
	tests := []struct {
		name      string
		group     string
		history   service.GroupHistory
		wantLists int
		wantCount service.ExceptionCounts
		wantFound bool
	}{
		{
			name: "no mark: the old group counts the day before the move only", group: histGroupA,
			history: service.GroupHistory{Lists: lists}, wantLists: 1, wantFound: true,
		},
		{
			name: "no mark: the new group counts the move day and after", group: histGroupB,
			history: service.GroupHistory{Lists: lists}, wantLists: 2, wantFound: true,
		},
		{
			name: "excused in the old group: it counts the move day and the absence", group: histGroupA,
			history:   service.GroupHistory{Lists: lists, Marks: []service.DayMark{excusedInA}},
			wantLists: 2, wantCount: service.ExceptionCounts{Excused: 1}, wantFound: true,
		},
		{
			name: "excused in the old group: the new group counts only the day after", group: histGroupB,
			history:   service.GroupHistory{Lists: lists, Elsewhere: []service.DayMark{excusedInA}},
			wantLists: 1, wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.history = tt.history
			svc := historyService(t, repo)

			got, err := svc.SummarizeAttendance(context.Background(), tt.group, "2026-10-01", "2026-10-31")
			require.NoError(t, err)

			line, ok := countsOf(got, enrMia)
			require.Equal(t, tt.wantFound, ok)
			assert.Equal(t, tt.wantLists, line.Lists)
			assert.Equal(t, tt.wantCount, line.ExceptionCounts)
		})
	}
}

func countsOf(summary service.AttendanceSummary, enrollmentID string) (service.StudentCounts, bool) {
	for _, s := range summary.Students {
		if s.EnrollmentID == enrollmentID {
			return s, true
		}
	}
	return service.StudentCounts{}, false
}
