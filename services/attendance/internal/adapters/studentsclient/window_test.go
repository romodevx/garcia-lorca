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

type stubTransfers struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	gotGroup string
}

func (s *stubTransfers) ListTransfers(_ context.Context, req *connect.Request[studentsv1.ListTransfersRequest]) (*connect.Response[studentsv1.ListTransfersResponse], error) {
	s.gotGroup = req.Msg.GetToGroupId()
	return connect.NewResponse(&studentsv1.ListTransfersResponse{Transfers: []*studentsv1.EnrollmentTransfer{
		{EnrollmentId: "enr-1", ToGroupId: "grp-1", MovedAt: timestamppb.New(time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC))},
	}}), nil
}

func TestTransfersIntoReadsEveryMoveIntoTheGroup(t *testing.T) {
	stub := &stubTransfers{}
	mux := http.NewServeMux()
	path, h := studentsv1connect.NewStudentsServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := New(srv.URL, srv.Client()).TransfersInto(context.Background(), "grp-1")
	if err != nil {
		t.Fatal(err)
	}
	if stub.gotGroup != "grp-1" {
		t.Errorf("asked for group %q", stub.gotGroup)
	}
	want := []service.TransferIn{{EnrollmentID: "enr-1", MovedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("TransfersInto = %+v, want %+v", got, want)
	}
}
