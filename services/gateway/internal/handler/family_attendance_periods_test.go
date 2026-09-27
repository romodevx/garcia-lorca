package handler_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

func utcDay(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, time.UTC)
}

// The school years of these tests.
var (
	cycle2526 = client.CycleResult{ID: "cyc-2526", StartsOn: utcDay(2025, 8, 25, 0), EndsOn: utcDay(2026, 7, 10, 0)}
	cycle2627 = client.CycleResult{ID: "cyc-2627", StartsOn: utcDay(2026, 8, 24, 0), EndsOn: utcDay(2027, 7, 9, 0)}
)

// windowCase is one child's history and the lists taken around it.
type windowCase struct {
	name        string
	now         time.Time
	month       string
	enrollments []client.Enrollment
	transfers   []client.Transfer
	sessions    []client.TakenList
	wantDays    []string // every row, newest first
	wantPresent int
	wantGroups  []string // the groups ListSessions was asked for
	wantEnrs    []string // the enrollments ListAttendance was asked for
}

// Review #1/#2/#7: a group's lists are the child's only inside that group's
// window: the cycle ∩ the enrollment's active period, split by transfers
// (moved day belongs to the new group), ended the day before a withdrawal.
func TestFamilyAttendanceGroupWindows(t *testing.T) {
	tests := []windowCase{
		{
			name:  "transfer mid-month: the same enrollment row, two groups",
			now:   familyToday,
			month: "2026-09",
			enrollments: []client.Enrollment{
				{ID: "enr-1", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-b", Status: "active", EnrolledAt: utcDay(2026, 8, 1, 12)},
			},
			transfers: []client.Transfer{{EnrollmentID: "enr-1", FromGroupID: "grp-a", ToGroupID: "grp-b", MovedAt: utcDay(2026, 9, 10, 17)}},
			sessions: []client.TakenList{
				{GroupID: "grp-a", Date: "2026-09-11"}, // after the move: not the child's
				{GroupID: "grp-b", Date: "2026-09-11"},
				{GroupID: "grp-a", Date: "2026-09-10"}, // the move day belongs to B
				{GroupID: "grp-b", Date: "2026-09-10"},
				{GroupID: "grp-a", Date: "2026-09-09"},
				{GroupID: "grp-b", Date: "2026-09-09"}, // before the move: not the child's
			},
			wantDays:    []string{"2026-09-17", "2026-09-11", "2026-09-10", "2026-09-09"},
			wantPresent: 3,
			wantGroups:  []string{"grp-a", "grp-b"},
			wantEnrs:    []string{"enr-1"},
		},
		{
			name:  "early re-enrollment for next cycle does not cut this cycle",
			now:   utcDay(2026, 6, 20, 15),
			month: "2026-06",
			enrollments: []client.Enrollment{
				{ID: "enr-next", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-2a", Status: "active", EnrolledAt: utcDay(2026, 6, 1, 12)},
				{ID: "enr-cur", StudentID: "stu-1", CycleID: cycle2526.ID, GroupID: "grp-1a", Status: "active", EnrolledAt: utcDay(2025, 8, 20, 12)},
			},
			sessions: []client.TakenList{
				{GroupID: "grp-1a", Date: "2026-06-15"},
				{GroupID: "grp-1a", Date: "2026-06-02"},
			},
			wantDays:    []string{"2026-06-20", "2026-06-15", "2026-06-02"},
			wantPresent: 2,
			wantGroups:  []string{"grp-1a"}, // next cycle's group has no window in June
			wantEnrs:    []string{"enr-cur"},
		},
		{
			name:  "withdrawal ends the window the day before, keeps the history",
			now:   familyToday,
			month: "2026-09",
			enrollments: []client.Enrollment{
				{ID: "enr-1", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-a", Status: "withdrawn", EnrolledAt: utcDay(2026, 8, 1, 12), EndedAt: utcDay(2026, 9, 10, 15)},
			},
			sessions: []client.TakenList{
				{GroupID: "grp-a", Date: "2026-09-11"},
				{GroupID: "grp-a", Date: "2026-09-10"},
				{GroupID: "grp-a", Date: "2026-09-09"},
				{GroupID: "grp-a", Date: "2026-09-08"},
			},
			wantDays:    []string{"2026-09-17", "2026-09-09", "2026-09-08"},
			wantPresent: 2,
			wantGroups:  []string{"grp-a"},
			wantEnrs:    []string{"enr-1"},
		},
		{
			name:  "withdrawal with no recorded end keeps the lists up to today",
			now:   familyToday,
			month: "2026-09",
			enrollments: []client.Enrollment{
				{ID: "enr-1", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-a", Status: "withdrawn", EnrolledAt: utcDay(2026, 8, 1, 12)},
			},
			sessions: []client.TakenList{
				{GroupID: "grp-a", Date: "2026-09-11"},
				{GroupID: "grp-a", Date: "2026-09-09"},
			},
			wantDays:    []string{"2026-09-17", "2026-09-11", "2026-09-09"},
			wantPresent: 2,
			wantGroups:  []string{"grp-a"},
			wantEnrs:    []string{"enr-1"},
		},
		{
			name:  "a mid-year newcomer's group lists count from the day they enrolled",
			now:   familyToday,
			month: "2026-09",
			enrollments: []client.Enrollment{
				{ID: "enr-1", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-a", Status: "active", EnrolledAt: utcDay(2026, 9, 9, 12)},
			},
			sessions: []client.TakenList{
				{GroupID: "grp-a", Date: "2026-09-09"},
				{GroupID: "grp-a", Date: "2026-09-08"},
			},
			wantDays:    []string{"2026-09-17", "2026-09-09"},
			wantPresent: 1,
			wantGroups:  []string{"grp-a"},
			wantEnrs:    []string{"enr-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			students := oneChild()
			students.enrollments, students.transfers = tt.enrollments, tt.transfers
			att := &fakeFamilyAttendance{sessions: tt.sessions}
			catalog := &fakeFamilyConfig{cycles: []client.CycleResult{cycle2526, cycle2627}}
			h := familyAttendanceHandlerAt(students, att, catalog, tt.now)
			rec, out := getFamilyAttendance(t, h, "?month="+tt.month)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			dates := make([]string, 0, len(out.Days))
			for _, d := range out.Days {
				dates = append(dates, d.Date)
			}
			assert.Equal(t, tt.wantDays, dates)
			assert.Equal(t, tt.wantPresent, out.Summary["present"])
			assert.ElementsMatch(t, tt.wantGroups, att.gotSessionGroups)
			assert.ElementsMatch(t, tt.wantEnrs, att.gotEnrollmentIDs)
		})
	}
}

// Review #3: a month no window overlaps costs no attendance read, and today
// still appears when the month is the current one.
func TestFamilyAttendanceSkipsEnrollmentsOutsideTheMonth(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", CycleID: cycle2627.ID, GroupID: "grp-a", Status: "active", EnrolledAt: utcDay(2026, 8, 1, 12)},
	}
	att := &fakeFamilyAttendance{}
	catalog := &fakeFamilyConfig{cycles: []client.CycleResult{cycle2627}}
	rec, out := getFamilyAttendance(t, familyAttendanceHandler(students, att, catalog), "?month=2026-07")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, out.Days)
	assert.Zero(t, att.sessionsCalls)
	assert.Empty(t, att.gotEnrollmentIDs)
}

// Review #3: more groups than one ListSessions call accepts are chunked, never
// refused.
func TestFamilyAttendanceChunksManyGroups(t *testing.T) {
	const moves = 24
	students := oneChild()
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", GroupID: fmt.Sprintf("grp-%02d", moves), Status: "active"},
	}
	for i := 0; i < moves; i++ { // one move a day: 24 groups share August
		students.transfers = append(students.transfers, client.Transfer{
			EnrollmentID: "enr-1", FromGroupID: fmt.Sprintf("grp-%02d", i), ToGroupID: fmt.Sprintf("grp-%02d", i+1),
			MovedAt: utcDay(2026, 8, 1+i, 9),
		})
	}
	att := &fakeFamilyAttendance{}
	rec, _ := getFamilyAttendance(t, familyAttendanceHandler(students, att, nil), "?month=2026-08")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, att.sessionCalls, 2)
	for _, call := range att.sessionCalls {
		assert.LessOrEqual(t, len(call), 20)
	}
	assert.Len(t, att.gotSessionGroups, moves, "every group of the month, each once")
}

// Review #8: a day with a daily list AND per-class lists derives its status
// from all of them.
func TestFamilyAttendanceMixedDayCombinesEveryList(t *testing.T) {
	tests := []struct {
		name    string
		records []client.AttendanceRecord
		want    string
	}{
		{"all present", nil, "present"},
		{"absent in one class", []client.AttendanceRecord{{EnrollmentID: "enr-1", Date: "2026-09-15", SubjectID: "sub-mat", Status: "absent"}}, "partial"},
		{"absent on the daily list", []client.AttendanceRecord{{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "absent"}}, "absent"},
		{"late everywhere", []client.AttendanceRecord{
			{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "late"},
			{EnrollmentID: "enr-1", Date: "2026-09-15", SubjectID: "sub-mat", Status: "late"},
		}, "late"},
		{"late in one class of a present day", []client.AttendanceRecord{{EnrollmentID: "enr-1", Date: "2026-09-15", SubjectID: "sub-mat", Status: "late"}}, "late"},
		{"excused daily, late class", []client.AttendanceRecord{
			{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "excused"},
			{EnrollmentID: "enr-1", Date: "2026-09-15", SubjectID: "sub-mat", Status: "late"},
		}, "late"},
		{"excused on the daily list only", []client.AttendanceRecord{{EnrollmentID: "enr-1", Date: "2026-09-15", Status: "excused"}}, "excused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			att := &fakeFamilyAttendance{
				sessions: []client.TakenList{
					{GroupID: grpDaily, Date: "2026-09-15"},
					{GroupID: grpDaily, Date: "2026-09-15", SubjectID: "sub-mat"},
				},
				byEnrollment: map[string][]client.AttendanceRecord{"enr-1": tt.records},
			}
			rec, out := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), att, nil), "?month=2026-09")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			day := dayOf(t, out.Days, "2026-09-15")
			assert.Equal(t, tt.want, day.Status)
			assert.Equal(t, 1, out.Summary[tt.want], "the summary counts the combined status")
		})
	}
}

// Review #11: ?month is bounded to 2000-01 … next month.
func TestFamilyAttendanceBoundsTheMonth(t *testing.T) {
	tests := []struct {
		month string
		want  int
	}{
		{"1999-12", http.StatusBadRequest},
		{"2000-01", http.StatusOK},
		{"2026-10", http.StatusOK},
		{"2026-11", http.StatusBadRequest},
		{"9999-12", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.month, func(t *testing.T) {
			rec, _ := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), &fakeFamilyAttendance{}, nil), "?month="+tt.month)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

// Review #1: a cycle read that fails cannot place the windows, so the read fails.
func TestFamilyAttendanceCycleErrorFailsTheRead(t *testing.T) {
	catalog := &fakeFamilyConfig{cyclesErr: errString("config down")}
	rec, _ := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpDaily), &fakeFamilyAttendance{}, catalog), "")
	assert.GreaterOrEqual(t, rec.Code, 500)
}

// Review #10: a failed subject catalog read degrades to empty names, but says so.
func TestFamilyAttendanceLogsASubjectCatalogFailure(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	att := &fakeFamilyAttendance{sessions: []client.TakenList{{GroupID: grpPerClass, Date: "2026-09-15", SubjectID: "sub-mat"}}}
	catalog := &fakeFamilyConfig{subjectsErr: errString("config down")}
	rec, _ := getFamilyAttendance(t, familyAttendanceHandler(childIn(grpPerClass), att, catalog), "?month=2026-09")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "subject")
}
