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
	"github.com/garcia-lorca/services/students/internal/adapters/configclient"
)

// stubConfigService is a minimal config service. It embeds the generated
// Unimplemented handler (every RPC returns Unimplemented) and overrides only
// GetGroup/GetTerm, so the adapter is exercised against the REAL Connect client
// over an in-process httptest server — no network, fully deterministic.
type stubConfigService struct {
	configv1connect.UnimplementedConfigServiceHandler
	group    *configv1.Group
	term     *configv1.Term
	notFound bool
}

func (s *stubConfigService) GetGroup(_ context.Context, _ *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "group not found"))
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: s.group}), nil
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

func TestGroupByID_MapsFieldsAndNotFound(t *testing.T) {
	const (
		groupID = "40000000-0000-0000-0000-000000000001"
		cycleID = "c0000000-0000-0000-0000-000000000001"
		gradeID = "90000000-0000-0000-0000-000000000001"
	)

	t.Run("found projects id/cycle/grade", func(t *testing.T) {
		c := newClient(t, &stubConfigService{group: &configv1.Group{
			Id: groupID, CycleId: cycleID, GradeLevelId: gradeID,
		}})
		got, err := c.GroupByID(context.Background(), groupID)
		if err != nil {
			t.Fatalf("GroupByID: %v", err)
		}
		if got.ID != groupID || got.CycleID != cycleID || got.GradeLevelID != gradeID {
			t.Errorf("mapping mismatch: %+v", got)
		}
	})

	t.Run("config NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubConfigService{notFound: true})
		_, err := c.GroupByID(context.Background(), groupID)
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
