package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T5: a per-class list's subject NAME travels on the events it publishes (it is
// never stored), so comms can say which class a student missed.
func TestSaveCarriesTheSubjectNameOnEvents(t *testing.T) {
	tests := []struct {
		name        string
		subjectID   string
		subjectName string
		wantName    string
	}{
		{name: "per-class list names its class", subjectID: subjectMath, subjectName: "  Matemáticas ", wantName: "Matemáticas"},
		{name: "daily list ignores a name", subjectID: "", subjectName: "Matemáticas", wantName: ""},
		{name: "per-class list without a name", subjectID: subjectMath, subjectName: "", wantName: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &spyPublisher{}
			svc := newService(t, newFakeRepo(), pub, "rec-1", "rec-2")
			_, err := svc.SaveDailyAttendance(context.Background(), service.SaveRequest{
				GroupID: groupID, Date: schoolDay, SubjectID: tt.subjectID, SubjectName: tt.subjectName,
				Marks: []service.Mark{
					{EnrollmentID: enrollmentAna, Status: "absent"},
					{EnrollmentID: enrollmentBeto, Status: "late"},
				},
				TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
			})
			require.NoError(t, err)
			require.Len(t, pub.marked, 2)
			require.Len(t, pub.absences, 1)
			for _, e := range pub.marked {
				assert.Equal(t, tt.wantName, e.SubjectName)
			}
			assert.Equal(t, tt.wantName, pub.absences[0].SubjectName)
		})
	}
}

// A correction back to present carries the class name too: comms renders
// "sí asistió … (Matemáticas)".
func TestClearedMarkCarriesTheSubjectName(t *testing.T) {
	pub := &spyPublisher{}
	svc := newService(t, newFakeRepo(), pub, "rec-1")
	req := service.SaveRequest{
		GroupID: groupID, Date: schoolDay, SubjectID: subjectMath, SubjectName: "Matemáticas",
		Marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}},
		TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
	}
	_, err := svc.SaveDailyAttendance(context.Background(), req)
	require.NoError(t, err)
	req.Marks = nil
	_, err = svc.SaveDailyAttendance(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, pub.marked, 2)
	assert.Equal(t, "present", pub.marked[1].Status)
	assert.Equal(t, "Matemáticas", pub.marked[1].SubjectName)
}

func TestSaveRejectsAnOverlongSubjectName(t *testing.T) {
	svc := newService(t, newFakeRepo(), nil)
	_, err := svc.SaveDailyAttendance(context.Background(), service.SaveRequest{
		GroupID: groupID, Date: schoolDay, SubjectID: subjectMath, SubjectName: strings.Repeat("á", 201),
		TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
	})
	wantInvalid(t, err)
}

const (
	sessionGroupA = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	sessionGroupB = "7c9e6679-7425-40de-944b-e07fc1f90ae8"
)

func TestListSessionsDelegatesAValidQuery(t *testing.T) {
	repo := newFakeRepo()
	repo.sessionsResult = []service.TakenList{{GroupID: sessionGroupA, Date: day(t, "2026-09-02"), SubjectID: subjectMath}}
	svc := newService(t, repo, nil)

	got, err := svc.ListSessions(context.Background(), []string{sessionGroupA, sessionGroupB, sessionGroupA}, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	assert.Equal(t, repo.sessionsResult, got)
	assert.Equal(t, []string{sessionGroupA, sessionGroupB}, repo.lastSessionGroups, "duplicate group ids are queried once")
	assert.Equal(t, day(t, "2026-09-01"), repo.lastSessionFrom)
	assert.Equal(t, day(t, "2026-09-30"), repo.lastSessionTo)
}

func TestListSessionsRejectsBadQueries(t *testing.T) {
	many := make([]string, 21)
	for i := range many {
		many[i] = "7c9e6679-7425-40de-944b-" + strings.Repeat("0", 10) + string(rune('a'+i%6)) + string(rune('a'+i/6))
	}
	tests := []struct {
		name     string
		groups   []string
		from, to string
	}{
		{name: "no groups", groups: nil, from: "2026-09-01", to: "2026-09-30"},
		{name: "malformed group", groups: []string{"grp-1"}, from: "2026-09-01", to: "2026-09-30"},
		{name: "too many groups", groups: many, from: "2026-09-01", to: "2026-09-30"},
		{name: "missing from", groups: []string{sessionGroupA}, from: "", to: "2026-09-30"},
		{name: "missing to", groups: []string{sessionGroupA}, from: "2026-09-01", to: ""},
		{name: "malformed date", groups: []string{sessionGroupA}, from: "01/09/2026", to: "2026-09-30"},
		{name: "reversed range", groups: []string{sessionGroupA}, from: "2026-09-30", to: "2026-09-01"},
		{name: "range too wide", groups: []string{sessionGroupA}, from: "2026-01-01", to: "2026-09-30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)
			_, err := svc.ListSessions(context.Background(), tt.groups, tt.from, tt.to)
			wantInvalid(t, err)
			assert.Zero(t, repo.sessionsCalls, "a rejected query never reaches the database")
		})
	}
}

// The widest accepted range is 62 days inclusive: two whole months.
func TestListSessionsAcceptsTwoMonths(t *testing.T) {
	svc := newService(t, newFakeRepo(), nil)
	_, err := svc.ListSessions(context.Background(), []string{sessionGroupA}, "2026-08-01", "2026-10-01")
	require.NoError(t, err)
	_, err = svc.ListSessions(context.Background(), []string{sessionGroupA}, "2026-08-01", "2026-10-02")
	wantInvalid(t, err)
}
