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
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
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
	allGroups   []*configv1.Group
	gradeLevels []*configv1.GradeLevel
	subjects    []*configv1.Subject
	teachers    []*configv1.Teacher
	terms       []*configv1.Term
	termsReq    *configv1.ListTermsRequest
}

func (s *stubCatalog) ListSubjects(context.Context, *connect.Request[configv1.ListSubjectsRequest]) (*connect.Response[configv1.ListSubjectsResponse], error) {
	return connect.NewResponse(&configv1.ListSubjectsResponse{Subjects: s.subjects}), nil
}

func (s *stubCatalog) ListTeachers(context.Context, *connect.Request[configv1.ListTeachersRequest]) (*connect.Response[configv1.ListTeachersResponse], error) {
	return connect.NewResponse(&configv1.ListTeachersResponse{Teachers: s.teachers}), nil
}

func (s *stubCatalog) ListTerms(_ context.Context, r *connect.Request[configv1.ListTermsRequest]) (*connect.Response[configv1.ListTermsResponse], error) {
	s.termsReq = r.Msg
	return connect.NewResponse(&configv1.ListTermsResponse{Terms: s.terms}), nil
}

func (s *stubCatalog) ListCycles(context.Context, *connect.Request[configv1.ListCyclesRequest]) (*connect.Response[configv1.ListCyclesResponse], error) {
	out := make([]*configv1.Cycle, 0, len(s.cycles))
	for _, c := range s.cycles {
		out = append(out, c)
	}
	return connect.NewResponse(&configv1.ListCyclesResponse{Cycles: out}), nil
}

func (s *stubCatalog) ListGroups(_ context.Context, r *connect.Request[configv1.ListGroupsRequest]) (*connect.Response[configv1.ListGroupsResponse], error) {
	var out []*configv1.Group
	for _, g := range s.allGroups {
		if g.GetCycleId() == r.Msg.GetCycleId() {
			out = append(out, g)
		}
	}
	return connect.NewResponse(&configv1.ListGroupsResponse{Groups: out}), nil
}

func (s *stubCatalog) ListGradeLevels(_ context.Context, r *connect.Request[configv1.ListGradeLevelsRequest]) (*connect.Response[configv1.ListGradeLevelsResponse], error) {
	var out []*configv1.GradeLevel
	for _, g := range s.gradeLevels {
		if g.GetLevelId() == r.Msg.GetLevelId() {
			out = append(out, g)
		}
	}
	return connect.NewResponse(&configv1.ListGradeLevelsResponse{GradeLevels: out}), nil
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

// A grading level whose scale config never sent (an older config deployed
// after grades) or sent inconsistent is refused with a sentence the sheet can
// show — never a silent 0–0 scale that refuses every grade, never a bare 500.
func TestLevelOfGroupRejectsAMissingScale(t *testing.T) {
	for name, grading := range map[string]*configv1.GradingScale{
		"absent":       nil,
		"inconsistent": {Enabled: true, MinHundredths: 900, MaxHundredths: 500},
	} {
		t.Run(name, func(t *testing.T) {
			stub := catalogStub()
			stub.levels["lvl-1"].Grading = grading
			_, err := newCatalogClient(t, stub).LevelOfGroup(context.Background(), "grp-1")
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Equal(t, "La escala de calificación del nivel no está configurada", glerrors.SafeMessage(err))
			assert.Equal(t, domain.CodeLevelScaleUnavailable, domain.RefusalCodeOf(err))
		})
	}
}

// A level that takes no numeric grades needs no usable scale to be read.
func TestLevelOfGroupToleratesAnUnusedScale(t *testing.T) {
	stub := catalogStub()
	stub.levels["lvl-1"].Grading = &configv1.GradingScale{Enabled: false}
	got, err := newCatalogClient(t, stub).LevelOfGroup(context.Background(), "grp-1")
	require.NoError(t, err)
	assert.False(t, got.GradingEnabled)
}

// LevelIDOfGroup needs no scale at all.
func TestLevelIDOfGroupIgnoresTheScale(t *testing.T) {
	stub := catalogStub()
	stub.levels["lvl-1"].Grading = nil
	key, err := newCatalogClient(t, stub).LevelIDOfGroup(context.Background(), "grp-1")
	require.NoError(t, err)
	assert.Equal(t, "lvl-1", key.ID)
	_, err = newCatalogClient(t, stub).LevelIDOfGroup(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestActiveCycleAndGroupsOfLevel(t *testing.T) {
	stub := catalogStub()
	c := newCatalogClient(t, stub)
	_, ok, err := c.ActiveCycle(context.Background())
	require.NoError(t, err)
	assert.False(t, ok, "no cycle is active")

	stub.cycles["cyc-1"].Status = configv1.CycleStatus_CYCLE_STATUS_ACTIVE
	stub.cycles["cyc-0"] = &configv1.Cycle{Id: "cyc-0", Status: configv1.CycleStatus_CYCLE_STATUS_CLOSED}
	got, ok, err := c.ActiveCycle(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "cyc-1", got.ID)

	stub.gradeLevels = []*configv1.GradeLevel{{Id: "gl-1", LevelId: "lvl-1"}, {Id: "gl-9", LevelId: "lvl-9"}}
	stub.allGroups = []*configv1.Group{
		{Id: "grp-1", CycleId: "cyc-1", GradeLevelId: "gl-1"},
		{Id: "grp-2", CycleId: "cyc-1", GradeLevelId: "gl-9"},
		{Id: "grp-3", CycleId: "cyc-0", GradeLevelId: "gl-1"},
	}
	groups, err := c.GroupsOfLevel(context.Background(), "cyc-1", "lvl-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"grp-1"}, groups)
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

// T7: a level catalog is read in a fixed number of calls — the level, its
// grades, the cycle groups, the cycle assignments, every subject and every
// teacher — and keeps only the level groups and their assignments.
func TestLevelCatalogReadsOnlyTheLevel(t *testing.T) {
	stub := catalogStub()
	stub.levels["lvl-1"].Name = "Primaria"
	stub.gradeLevels = []*configv1.GradeLevel{{Id: "gl-1", LevelId: "lvl-1", Name: "1° PRIMARIA"}, {Id: "gl-9", LevelId: "lvl-9", Name: "1° SECUNDARIA"}}
	stub.allGroups = []*configv1.Group{
		{Id: "grp-1", CycleId: "cyc-1", GradeLevelId: "gl-1", Name: "A"},
		{Id: "grp-2", CycleId: "cyc-1", GradeLevelId: "gl-9", Name: "A"},
	}
	stub.assignments = []*configv1.TeachingAssignment{
		{Id: "a1", GroupId: "grp-1", SubjectId: "s1", TeacherId: "t1", CycleId: "cyc-1"},
		{Id: "a2", GroupId: "grp-2", SubjectId: "s2", TeacherId: "t2", CycleId: "cyc-1"},
	}
	stub.subjects = []*configv1.Subject{{Id: "s1", Name: "Matemáticas"}, {Id: "s2", Name: "Física"}}
	stub.teachers = []*configv1.Teacher{{Id: "t1", FirstName: "Gabriela", LastName: "Rojas"}, {Id: "t2", FirstName: "Otro", LastName: "Docente"}}

	got, err := newCatalogClient(t, stub).LevelCatalog(context.Background(), "cyc-1", "lvl-1")
	require.NoError(t, err)
	assert.Equal(t, "Primaria", got.LevelName)
	assert.True(t, got.GradingEnabled)
	assert.Equal(t, 600, got.Scale.PassingHundredths())
	assert.Equal(t, []ports.GroupRef{{ID: "grp-1", Label: "1° PRIMARIA · A"}}, got.Groups)
	require.Len(t, got.Assignments, 1)
	assert.Equal(t, "a1", got.Assignments[0].ID)
	assert.Equal(t, "cyc-1", stub.listReq.GetCycleId())
	assert.Empty(t, stub.listReq.GetGroupId(), "ONE read for the whole cycle, never one per group")
	assert.Equal(t, "Matemáticas", got.SubjectNames["s1"])
	assert.Equal(t, "Gabriela Rojas", got.TeacherNames["t1"])

	_, err = newCatalogClient(t, stub).LevelCatalog(context.Background(), "cyc-1", "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// A term carries its name, ordinal and months; a cycle lists its terms, of one
// level when asked.
func TestListTermsProjectsMonths(t *testing.T) {
	stub := catalogStub()
	stub.terms = []*configv1.Term{
		{Id: "term-2", CycleId: "cyc-1", LevelId: "lvl-1", Name: "2.º trimestre", Ordinal: 2, Months: []string{"2026-12", "2027-01"}},
		{Id: "term-1", CycleId: "cyc-1", LevelId: "lvl-1", Name: "1.er trimestre", Ordinal: 1, Months: []string{"2026-09", "2026-10", "2026-11"}},
	}
	got, err := newCatalogClient(t, stub).ListTerms(context.Background(), "cyc-1", "lvl-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "term-1", got[0].ID, "ordinal order")
	assert.Equal(t, "1.er trimestre", got[0].Name)
	assert.Equal(t, 1, got[0].Ordinal)
	require.Len(t, got[0].Months, 3)
	assert.Equal(t, "2026-09", got[0].Months[0].String())
	assert.Equal(t, "lvl-1", stub.termsReq.GetLevelId())

	stub.terms = []*configv1.Term{{Id: "bad", Months: []string{"sept"}}}
	_, err = newCatalogClient(t, stub).ListTerms(context.Background(), "cyc-1", "")
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err), "a malformed month from config is not silently dropped")
}

// Review finding 4: config's level status travels with the lookup every write
// path already makes, through both projections.
func TestLevelLookupsCarryTheDeletingStatus(t *testing.T) {
	for name, status := range map[string]configv1.LevelStatus{
		"deleting": configv1.LevelStatus_LEVEL_STATUS_DELETING,
		"active":   configv1.LevelStatus_LEVEL_STATUS_ACTIVE,
		"unset":    configv1.LevelStatus_LEVEL_STATUS_UNSPECIFIED,
	} {
		t.Run(name, func(t *testing.T) {
			stub := catalogStub()
			stub.levels["lvl-1"].Status = status
			c := newCatalogClient(t, stub)
			want := status == configv1.LevelStatus_LEVEL_STATUS_DELETING

			ref, err := c.LevelOfGroup(context.Background(), "grp-1")
			require.NoError(t, err)
			assert.Equal(t, want, ref.Deleting)

			key, err := c.LevelIDOfGroup(context.Background(), "grp-1")
			require.NoError(t, err)
			assert.Equal(t, ports.LevelKey{ID: "lvl-1", Deleting: want}, key)

			catalog, err := c.LevelCatalog(context.Background(), "cyc-1", "lvl-1")
			require.NoError(t, err)
			assert.Equal(t, want, catalog.Deleting, "a period's close/reopen reads it here")
		})
	}
}
