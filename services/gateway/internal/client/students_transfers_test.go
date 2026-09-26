package client

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
)

// fakeHistoryRPC implements only ListTransfers and ListEnrollments.
type fakeHistoryRPC struct {
	studentsv1connect.StudentsServiceClient
	gotStudentID string
}

var historyMoved = time.Date(2026, 9, 10, 17, 0, 0, 0, time.UTC)

func (f *fakeHistoryRPC) ListTransfers(_ context.Context, req *connect.Request[studentsv1.ListTransfersRequest]) (*connect.Response[studentsv1.ListTransfersResponse], error) {
	f.gotStudentID = req.Msg.GetStudentId()
	return connect.NewResponse(&studentsv1.ListTransfersResponse{Transfers: []*studentsv1.EnrollmentTransfer{
		{Id: "tr-1", EnrollmentId: "enr-1", FromGroupId: "grp-a", ToGroupId: "grp-b", MovedAt: timestamppb.New(historyMoved)},
	}}), nil
}

func (f *fakeHistoryRPC) ListEnrollments(_ context.Context, _ *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: []*studentsv1.Enrollment{
		{Id: "enr-1", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN, EndedAt: timestamppb.New(historyMoved)},
		{Id: "enr-2", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
	}}), nil
}

func TestListTransfersAndEndedAt(t *testing.T) {
	rpc := &fakeHistoryRPC{}
	c := &StudentsClient{rpc: rpc}
	got, err := c.ListTransfers(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	want := Transfer{ID: "tr-1", EnrollmentID: "enr-1", FromGroupID: "grp-a", ToGroupID: "grp-b", MovedAt: historyMoved}
	if rpc.gotStudentID != "stu-1" || len(got) != 1 || got[0] != want {
		t.Fatalf("ListTransfers = %+v (asked %q), want [%+v]", got, rpc.gotStudentID, want)
	}

	enrollments, err := c.ListEnrollments(context.Background(), "stu-1", "", "")
	if err != nil {
		t.Fatalf("ListEnrollments: %v", err)
	}
	if !enrollments[0].EndedAt.Equal(historyMoved) || !enrollments[1].EndedAt.IsZero() {
		t.Errorf("EndedAt = %v / %v", enrollments[0].EndedAt, enrollments[1].EndedAt)
	}
}
