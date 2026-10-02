package handler_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T12: the report's counts cross the wire one entry per active pupil, zeros
// included.
func TestSummarizeAttendanceMapsTheCounts(t *testing.T) {
	repo := newFakeRepo()
	repo.summary = service.GroupSummary{ListsTaken: 9, Counts: map[string]service.ExceptionCounts{"enr-1": {Absent: 1, Late: 2, Excused: 3}}}
	h := newHandler(repo, []service.EnrollmentRef{activeEnrollment("enr-1", "stu-1"), activeEnrollment("enr-2", "stu-2")})

	res, err := h.SummarizeAttendance(context.Background(), connect.NewRequest(&attendancev1.SummarizeAttendanceRequest{
		GroupId: groupID, FromDate: "2026-09-01", ToDate: "2026-09-30",
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(9), res.Msg.GetListsTaken())
	require.Len(t, res.Msg.GetCounts(), 2)
	first := res.Msg.GetCounts()[0]
	assert.Equal(t, "enr-1", first.GetEnrollmentId())
	assert.Equal(t, "stu-1", first.GetStudentId())
	assert.Equal(t, [3]int32{1, 2, 3}, [3]int32{first.GetAbsent(), first.GetLate(), first.GetExcused()})
	assert.Zero(t, res.Msg.GetCounts()[1].GetLate())
}

func TestSummarizeAttendanceBadRangeIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	_, err := h.SummarizeAttendance(context.Background(), connect.NewRequest(&attendancev1.SummarizeAttendanceRequest{
		GroupId: groupID, FromDate: "2026-09-30", ToDate: "2026-09-01",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
