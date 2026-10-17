package studentsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T12: the report needs when a pupil joined the group and when they left it.
func TestToRefCarriesTheEnrollmentWindow(t *testing.T) {
	enrolled := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC)
	ended := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	got := toRef(&studentsv1.Enrollment{Id: "enr-1", EnrolledAt: timestamppb.New(enrolled), EndedAt: timestamppb.New(ended)})
	if !got.EnrolledAt.Equal(enrolled) || !got.EndedAt.Equal(ended) {
		t.Errorf("toRef window = %v..%v, want %v..%v", got.EnrolledAt, got.EndedAt, enrolled, ended)
	}
	if open := toRef(&studentsv1.Enrollment{Id: "enr-2"}); !open.EndedAt.IsZero() || !open.EnrolledAt.IsZero() {
		t.Errorf("unset timestamps must stay zero, got %+v", open)
	}
}

type stubHistory struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	gotMovesGroup, gotEverGroup string
}

func (s *stubHistory) ListTransfers(_ context.Context, req *connect.Request[studentsv1.ListTransfersRequest]) (*connect.Response[studentsv1.ListTransfersResponse], error) {
	s.gotMovesGroup = req.Msg.GetGroupId()
	return connect.NewResponse(&studentsv1.ListTransfersResponse{Transfers: []*studentsv1.EnrollmentTransfer{
		{EnrollmentId: "enr-1", FromGroupId: "grp-0", ToGroupId: "grp-1", MovedAt: timestamppb.New(time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC))},
	}}), nil
}

func (s *stubHistory) ListEnrollments(_ context.Context, req *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	s.gotEverGroup = req.Msg.GetEverInGroupId()
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: []*studentsv1.Enrollment{
		{Id: "enr-1", StudentId: "stu-1", GroupId: "grp-2", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
	}}), nil
}

func historyClient(t *testing.T) (*Client, *stubHistory) {
	t.Helper()
	stub := &stubHistory{}
	mux := http.NewServeMux()
	path, h := studentsv1connect.NewStudentsServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client()), stub
}

// CC-301: the group's moves are read in BOTH directions, in one read.
func TestMovesOfReadsEveryMoveIntoOrOutOfTheGroup(t *testing.T) {
	c, stub := historyClient(t)
	got, err := c.MovesOf(context.Background(), "grp-1")
	if err != nil {
		t.Fatal(err)
	}
	if stub.gotMovesGroup != "grp-1" {
		t.Errorf("asked for group %q", stub.gotMovesGroup)
	}
	want := service.Move{EnrollmentID: "enr-1", FromGroupID: "grp-0", ToGroupID: "grp-1", MovedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)}
	if len(got) != 1 || got[0] != want {
		t.Errorf("MovesOf = %+v, want %+v", got, want)
	}
}

// CC-301: the group's past includes the enrollments moved out of it since.
func TestEverInGroupAsksForTheGroupsWholePast(t *testing.T) {
	c, stub := historyClient(t)
	got, err := c.EverInGroup(context.Background(), "grp-1")
	if err != nil {
		t.Fatal(err)
	}
	if stub.gotEverGroup != "grp-1" {
		t.Errorf("asked for ever_in_group_id %q", stub.gotEverGroup)
	}
	if len(got) != 1 || got[0].GroupID != "grp-2" || got[0].Status != "active" {
		t.Errorf("EverInGroup = %+v", got)
	}
}
