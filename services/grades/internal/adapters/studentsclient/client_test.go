package studentsclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/adapters/studentsclient"
)

// stubStudentsService embeds the generated Unimplemented handler and overrides
// only GetEnrollment/ListEnrollments, so the adapter runs against the REAL
// Connect client over an in-process httptest server.
type stubStudentsService struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	enrollment  *studentsv1.Enrollment
	enrollments []*studentsv1.Enrollment
	notFound    bool
}

func (s *stubStudentsService) GetEnrollment(_ context.Context, _ *connect.Request[studentsv1.GetEnrollmentRequest]) (*connect.Response[studentsv1.GetEnrollmentResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "enrollment not found"))
	}
	return connect.NewResponse(&studentsv1.GetEnrollmentResponse{Enrollment: s.enrollment}), nil
}

func (s *stubStudentsService) ListEnrollments(_ context.Context, _ *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: s.enrollments}), nil
}

func newClient(t *testing.T, svc *stubStudentsService) *studentsclient.Client {
	t.Helper()
	path, handler := studentsv1connect.NewStudentsServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return studentsclient.New(srv.URL, srv.Client())
}

func TestByID_MapsFieldsStatusAndNotFound(t *testing.T) {
	const (
		enrollmentID = "e0000000-0000-0000-0000-000000000001"
		studentID    = "d0000000-0000-0000-0000-000000000001"
		groupID      = "40000000-0000-0000-0000-000000000001"
		cycleID      = "c0000000-0000-0000-0000-000000000001"
	)

	t.Run("found projects fields and lowercases the status enum", func(t *testing.T) {
		c := newClient(t, &stubStudentsService{enrollment: &studentsv1.Enrollment{
			Id: enrollmentID, StudentId: studentID, GroupId: groupID, CycleId: cycleID,
			Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE,
		}})
		got, err := c.ByID(context.Background(), enrollmentID)
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if got.ID != enrollmentID || got.StudentID != studentID || got.GroupID != groupID ||
			got.CycleID != cycleID || got.Status != "active" {
			t.Errorf("mapping mismatch: %+v", got)
		}
	})

	t.Run("students NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubStudentsService{notFound: true})
		_, err := c.ByID(context.Background(), enrollmentID)
		if glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("want KindNotFound, got %v (kind %v)", err, glerrors.KindOf(err))
		}
	})
}

func TestByGroup_ProjectsEachEnrollment(t *testing.T) {
	const groupID = "40000000-0000-0000-0000-000000000001"
	c := newClient(t, &stubStudentsService{enrollments: []*studentsv1.Enrollment{
		{Id: "e1", StudentId: "s1", GroupId: groupID, Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
		{Id: "e2", StudentId: "s2", GroupId: groupID, Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN},
	}})
	got, err := c.ByGroup(context.Background(), groupID)
	if err != nil {
		t.Fatalf("ByGroup: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 enrollments, got %d", len(got))
	}
	if got[0].ID != "e1" || got[0].Status != "active" || got[1].ID != "e2" || got[1].Status != "withdrawn" {
		t.Errorf("projection mismatch: %+v", got)
	}
}
