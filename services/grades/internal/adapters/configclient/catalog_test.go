package configclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/adapters/configclient"
)

// stubCatalog serves the group → grade → level chain, cycles and a group's
// assignments over a real Connect handler.
type stubCatalog struct {
	configv1connect.UnimplementedConfigServiceHandler
	groups      map[string]*configv1.Group
	grades      map[string]*configv1.GradeLevel
	levels      map[string]*configv1.Level
	cycles      map[string]*configv1.Cycle
	assignments []*configv1.TeachingAssignment
	listReq     *configv1.ListTeachingAssignmentsRequest
}

func notFound() error { return connect.NewError(connect.CodeNotFound, nil) }

func (s *stubCatalog) GetGroup(_ context.Context, r *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	g, ok := s.groups[r.Msg.GetId()]
	if !ok {
		return nil, notFound()
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: g}), nil
}

func (s *stubCatalog) GetGradeLevel(_ context.Context, r *connect.Request[configv1.GetGradeLevelRequest]) (*connect.Response[configv1.GetGradeLevelResponse], error) {
	g, ok := s.grades[r.Msg.GetId()]
	if !ok {
		return nil, notFound()
	}
	return connect.NewResponse(&configv1.GetGradeLevelResponse{GradeLevel: g}), nil
}

func (s *stubCatalog) GetLevel(_ context.Context, r *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	l, ok := s.levels[r.Msg.GetId()]
	if !ok {
		return nil, notFound()
	}
	return connect.NewResponse(&configv1.GetLevelResponse{Level: l}), nil
}

func (s *stubCatalog) GetCycle(_ context.Context, r *connect.Request[configv1.GetCycleRequest]) (*connect.Response[configv1.GetCycleResponse], error) {
	c, ok := s.cycles[r.Msg.GetId()]
	if !ok {
		return nil, notFound()
	}
	return connect.NewResponse(&configv1.GetCycleResponse{Cycle: c}), nil
}

func (s *stubCatalog) ListTeachingAssignments(_ context.Context, r *connect.Request[configv1.ListTeachingAssignmentsRequest]) (*connect.Response[configv1.ListTeachingAssignmentsResponse], error) {
	s.listReq = r.Msg
	return connect.NewResponse(&configv1.ListTeachingAssignmentsResponse{TeachingAssignments: s.assignments}), nil
}

func newCatalogClient(t *testing.T, svc *stubCatalog) *configclient.Client {
	t.Helper()
	path, handler := configv1connect.NewConfigServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return configclient.New(srv.URL, srv.Client())
}

func catalogStub() *stubCatalog {
	return &stubCatalog{
		groups: map[string]*configv1.Group{"grp-1": {Id: "grp-1", GradeLevelId: "gl-1"}},
		grades: map[string]*configv1.GradeLevel{"gl-1": {Id: "gl-1", LevelId: "lvl-1"}},
		levels: map[string]*configv1.Level{"lvl-1": {Id: "lvl-1", Grading: &configv1.GradingScale{
			Enabled: true, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600,
		}}},
		cycles: map[string]*configv1.Cycle{"cyc-1": {Id: "cyc-1", Name: "2026-2027",
			StartsOn: timestamppb.New(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)),
			EndsOn:   timestamppb.New(time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC))}},
	}
}

func TestLevelOfGroupWalksGroupGradeLevel(t *testing.T) {
	c := newCatalogClient(t, catalogStub())
	got, err := c.LevelOfGroup(context.Background(), "grp-1")
	require.NoError(t, err)
	assert.Equal(t, "lvl-1", got.ID)
	assert.True(t, got.GradingEnabled)
	assert.Equal(t, 500, got.Scale.MinHundredths())
	assert.Equal(t, 1000, got.Scale.MaxHundredths())
	assert.Equal(t, 600, got.Scale.PassingHundredths())
	assert.Equal(t, 1, got.Scale.Decimals())

	_, err = c.LevelOfGroup(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// A level whose scale config never sent (an old config) or sent inconsistent is
// an internal failure, never a silent 0–0 scale that refuses every grade.
func TestLevelOfGroupRejectsAMissingScale(t *testing.T) {
	stub := catalogStub()
	stub.levels["lvl-1"].Grading = nil
	_, err := newCatalogClient(t, stub).LevelOfGroup(context.Background(), "grp-1")
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err))
}

func TestCycleByID(t *testing.T) {
	c := newCatalogClient(t, catalogStub())
	got, err := c.CycleByID(context.Background(), "cyc-1")
	require.NoError(t, err)
	assert.Equal(t, "2026-2027", got.Name)
	assert.Equal(t, time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), got.StartsOn)
	assert.Equal(t, time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC), got.EndsOn)

	stub := catalogStub()
	stub.cycles["cyc-1"].StartsOn, stub.cycles["cyc-1"].EndsOn = nil, nil
	undated, err := newCatalogClient(t, stub).CycleByID(context.Background(), "cyc-1")
	require.NoError(t, err)
	assert.True(t, undated.StartsOn.IsZero() && undated.EndsOn.IsZero(), "unset dates stay zero, not 1970")

	_, err = c.CycleByID(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestAssignmentsByGroupFiltersByCycleAndGroup(t *testing.T) {
	stub := catalogStub()
	stub.assignments = []*configv1.TeachingAssignment{{Id: "a1", GroupId: "grp-1", SubjectId: "s1", TeacherId: "t1", CycleId: "cyc-1"}}
	got, err := newCatalogClient(t, stub).AssignmentsByGroup(context.Background(), "cyc-1", "grp-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "s1", got[0].SubjectID)
	assert.Equal(t, "cyc-1", stub.listReq.GetCycleId())
	assert.Equal(t, "grp-1", stub.listReq.GetGroupId())
}

func TestTermByIDCarriesItsLevel(t *testing.T) {
	c := newClient(t, &stubConfigService{term: &configv1.Term{Id: "term-1", CycleId: "cyc-1", LevelId: "lvl-1"}})
	got, err := c.TermByID(context.Background(), "term-1")
	require.NoError(t, err)
	assert.Equal(t, "lvl-1", got.LevelID)
}
