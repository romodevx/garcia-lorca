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
	"github.com/garcia-lorca/pkg/levelarchive"
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
	level    *configv1.Level
	grade    *configv1.GradeLevel
	notFound bool
}

func (s *stubConfigService) GetGradeLevel(_ context.Context, _ *connect.Request[configv1.GetGradeLevelRequest]) (*connect.Response[configv1.GetGradeLevelResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "grade level not found"))
	}
	return connect.NewResponse(&configv1.GetGradeLevelResponse{GradeLevel: s.grade}), nil
}

func (s *stubConfigService) GetLevel(_ context.Context, _ *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	if s.notFound {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "level not found"))
	}
	return connect.NewResponse(&configv1.GetLevelResponse{Level: s.level}), nil
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

func TestLevelByID_MapsFieldsAndNotFound(t *testing.T) {
	const levelID = "10000000-0000-0000-0000-000000000001"

	t.Run("found carries the id and the school's own name", func(t *testing.T) {
		c := newClient(t, &stubConfigService{level: &configv1.Level{Id: levelID, Name: "Preescolar"}})
		got, err := c.LevelByID(context.Background(), levelID)
		if err != nil {
			t.Fatalf("LevelByID: %v", err)
		}
		if got.ID() != levelID || got.Name() != "Preescolar" {
			t.Errorf("mapping mismatch: (%q, %q)", got.ID(), got.Name())
		}
	})

	t.Run("config NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubConfigService{notFound: true})
		_, err := c.LevelByID(context.Background(), levelID)
		if glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("want KindNotFound, got %v (kind %v)", err, glerrors.KindOf(err))
		}
	})
}

func TestLevelOfGrade_MapsFieldsAndNotFound(t *testing.T) {
	const gradeID, levelID = "90000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001"

	t.Run("found returns the grade's level", func(t *testing.T) {
		c := newClient(t, &stubConfigService{grade: &configv1.GradeLevel{Id: gradeID, LevelId: levelID}, level: &configv1.Level{Id: levelID}})
		got, err := c.LevelOfGrade(context.Background(), gradeID)
		if err != nil || got != levelID {
			t.Fatalf("LevelOfGrade = (%q, %v)", got, err)
		}
	})

	t.Run("config NotFound maps to ErrNotFound", func(t *testing.T) {
		c := newClient(t, &stubConfigService{notFound: true})
		if _, err := c.LevelOfGrade(context.Background(), gradeID); glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("want KindNotFound, got %v", err)
		}
	})
}

// Review finding 4: a level whose deletion is running refuses every placement
// that resolves it — through its id or through one of its grades.
func TestLevelLookupsRefuseALevelBeingDeleted(t *testing.T) {
	const gradeID, levelID = "90000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001"
	cases := []struct {
		name    string
		status  configv1.LevelStatus
		refused bool
	}{
		{"active", configv1.LevelStatus_LEVEL_STATUS_ACTIVE, false},
		{"unspecified (an older config) is active", configv1.LevelStatus_LEVEL_STATUS_UNSPECIFIED, false},
		{"deleting", configv1.LevelStatus_LEVEL_STATUS_DELETING, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, &stubConfigService{
				grade: &configv1.GradeLevel{Id: gradeID, LevelId: levelID},
				level: &configv1.Level{Id: levelID, Name: "Kinder", Status: tc.status},
			})
			_, byID := c.LevelByID(context.Background(), levelID)
			_, byGrade := c.LevelOfGrade(context.Background(), gradeID)
			for _, err := range []error{byID, byGrade} {
				if !tc.refused {
					if err != nil {
						t.Fatalf("unexpected refusal: %v", err)
					}
					continue
				}
				if levelarchive.CodeOf(err) != levelarchive.CodeLevelBeingDeleted || glerrors.KindOf(err) != glerrors.KindConflict {
					t.Errorf("want LEVEL_BEING_DELETED conflict, got %v", err)
				}
			}
		})
	}
}
