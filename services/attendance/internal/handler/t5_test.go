package handler_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

type spyPublisher struct{ marked []service.MarkedEvent }

func (s *spyPublisher) PublishAbsenceRecorded(context.Context, service.AbsenceEvent) error {
	return nil
}

func (s *spyPublisher) PublishAttendanceMarked(_ context.Context, e service.MarkedEvent) error {
	s.marked = append(s.marked, e)
	return nil
}

// The wire's subject_name reaches the published events.
func TestSaveDailyAttendanceForwardsTheSubjectName(t *testing.T) {
	pub := &spyPublisher{}
	enr := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{groupID: {activeEnrollment("enr-1", "stu-1")}}}
	svc := service.NewAttendanceService(newFakeRepo(), enr, &fakeGroups{existing: map[string]bool{groupID: true}}, pub,
		testTenant, func() string { return "rec-1" }, slog.New(slog.DiscardHandler))
	h := handler.NewAttendanceHandler(svc)

	_, err := h.SaveDailyAttendance(context.Background(), connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: testDate, SubjectId: subjectUUID, SubjectName: "Matemáticas",
		TakenBy: takerID, TakenByName: "Gabriela Rojas",
		Marks: []*attendancev1.Mark{{EnrollmentId: "enr-1", Status: "late"}},
	}))
	require.NoError(t, err)
	require.Len(t, pub.marked, 1)
	assert.Equal(t, "Matemáticas", pub.marked[0].SubjectName)
}

func TestListSessionsMapsTheTakenLists(t *testing.T) {
	repo := newFakeRepo()
	repo.taken = []service.TakenList{
		{GroupID: sessionGroup, Date: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), SubjectID: subjectUUID},
		{GroupID: sessionGroup, Date: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)},
	}
	h := newHandler(repo, nil)

	res, err := h.ListSessions(context.Background(), connect.NewRequest(&attendancev1.ListSessionsRequest{
		GroupIds: []string{sessionGroup}, FromDate: "2026-09-01", ToDate: "2026-09-30",
	}))
	require.NoError(t, err)
	require.Len(t, res.Msg.GetLists(), 2)
	assert.Equal(t, "2026-09-16", res.Msg.GetLists()[0].GetDate())
	assert.Equal(t, subjectUUID, res.Msg.GetLists()[0].GetSubjectId())
	assert.Equal(t, sessionGroup, res.Msg.GetLists()[1].GetGroupId())
	assert.Empty(t, res.Msg.GetLists()[1].GetSubjectId())
}

func TestListSessionsBadRangeIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	_, err := h.ListSessions(context.Background(), connect.NewRequest(&attendancev1.ListSessionsRequest{
		GroupIds: []string{sessionGroup}, FromDate: "2026-09-30", ToDate: "2026-09-01",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

const sessionGroup = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
