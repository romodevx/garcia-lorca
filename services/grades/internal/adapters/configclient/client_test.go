package configclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/adapters/configclient"
)

// stubConfigService is a minimal config service. It embeds the generated
// Unimplemented handler (every RPC returns Unimplemented) and overrides only
// GetTeachingAssignment/GetTerm, so the adapter is exercised against the REAL
// Connect client over an in-process httptest server — no network, fully
// deterministic.
type stubConfigService struct {
	configv1connect.UnimplementedConfigServiceHandler
	assignment *configv1.TeachingAssignment
	term       *configv1.Term
	notFound   bool
}

func (s *stubConfigService) GetTeachingAssignment(_ context.Context, _ *connect.Request[configv1.GetTeachingAssignmentRequest]) (*connect.Response[configv1.GetTeachingAssignmentResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "teaching assignment not found"))
	}
	return connect.NewResponse(&configv1.GetTeachingAssignmentResponse{TeachingAssignment: s.assignment}), nil
}

func (s *stubConfigService) GetTerm(_ context.Context, _ *connect.Request[configv1.GetTermRequest]) (*connect.Response[configv1.GetTermResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "term not found"))
	}
	return connect.NewResponse(&configv1.GetTermResponse{Term: s.term}), nil
}

func newClient(t *testing.T, svc *stubConfigService) *configclient.Client {
	t.Helper()
	path, handler := configv1connect.NewConfigServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return configclient.New(srv.URL, srv.Client())
}

func TestByID_MapsFieldsAndNotFound(t *testing.T) {
	const (
		assignmentID = "a0000000-0000-0000-0000-000000000001"
		groupID      = "40000000-0000-0000-0000-000000000001"
		subjectID    = "50000000-0000-0000-0000-000000000001"
		teacherID    = "60000000-0000-0000-0000-000000000001"
		cycleID      = "c0000000-0000-0000-0000-000000000001"
	)

	t.Run("found projects id/group/subject/teacher/cycle", func(t *testing.T) {
		c := newClient(t, &stubConfigService{assignment: &configv1.TeachingAssignment{
			Id: assignmentID, GroupId: groupID, SubjectId: subjectID, TeacherId: teacherID, CycleId: cycleID,
		}})
		got, err := c.ByID(context.Background(), assignmentID)
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if got.ID != assignmentID || got.GroupID != groupID || got.SubjectID != subjectID ||
			got.TeacherID != teacherID || got.CycleID != cycleID {
			t.Errorf("mapping mismatch: %+v", got)
		}
	})

	t.Run("config NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubConfigService{notFound: true})
		_, err := c.ByID(context.Background(), assignmentID)
		if glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("want KindNotFound, got %v (kind %v)", err, glerrors.KindOf(err))
		}
	})
}

func TestTermByID_MapsFieldsAndNotFound(t *testing.T) {
	const (
		termID  = "70000000-0000-0000-0000-000000000001"
		cycleID = "c0000000-0000-0000-0000-000000000001"
	)

	t.Run("found projects id/cycle", func(t *testing.T) {
		c := newClient(t, &stubConfigService{term: &configv1.Term{Id: termID, CycleId: cycleID}})
		got, err := c.TermByID(context.Background(), termID)
		if err != nil {
			t.Fatalf("TermByID: %v", err)
		}
		if got.ID != termID || got.CycleID != cycleID {
			t.Errorf("mapping mismatch: %+v", got)
		}
	})

	t.Run("config NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubConfigService{notFound: true})
		_, err := c.TermByID(context.Background(), termID)
		if glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("want KindNotFound, got %v (kind %v)", err, glerrors.KindOf(err))
		}
	})
}
