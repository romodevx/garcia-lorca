package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

const tenant = "t1"

var (
	someDate  = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	laterDate = someDate.AddDate(1, 0, 0)
)

// --- fake CatalogRepository ------------------------------------------------
//
// The fake models real repository behavior: it enforces the same unique-key
// scopes as the DB (duplicate → KindConflict) and answers Get* so the use
// case's parent-existence checks work against persisted state. It lets the
// handler tests run through a real CatalogService without a database.

type fakeCatalog struct {
	cycles      map[string]domain.Cycle
	levels      map[string]domain.Level
	gradeLevels map[string]domain.GradeLevel
	subjects    map[string]domain.Subject
	terms       map[string]domain.Term
	groups      map[string]domain.Group
	unique      map[string]bool
}

func newFake() *fakeCatalog {
	return &fakeCatalog{
		cycles:      map[string]domain.Cycle{},
		levels:      map[string]domain.Level{},
		gradeLevels: map[string]domain.GradeLevel{},
		subjects:    map[string]domain.Subject{},
		terms:       map[string]domain.Term{},
		groups:      map[string]domain.Group{},
		unique:      map[string]bool{},
	}
}

func (f *fakeCatalog) claim(key string) error {
	if f.unique[key] {
		return glerrors.ErrConflict
	}
	f.unique[key] = true
	return nil
}

func (f *fakeCatalog) CreateCycle(_ context.Context, c domain.Cycle) error {
	if err := f.claim("cycle|" + c.Name()); err != nil {
		return err
	}
	f.cycles[c.ID()] = c
	return nil
}

func (f *fakeCatalog) GetCycle(_ context.Context, id string) (domain.Cycle, error) {
	if c, ok := f.cycles[id]; ok {
		return c, nil
	}
	return domain.Cycle{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListCycles(_ context.Context) ([]domain.Cycle, error) {
	out := make([]domain.Cycle, 0, len(f.cycles))
	for _, c := range f.cycles {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCatalog) CreateLevel(_ context.Context, l domain.Level) error {
	if err := f.claim("level|" + l.Name()); err != nil {
		return err
	}
	f.levels[l.ID()] = l
	return nil
}

func (f *fakeCatalog) GetLevel(_ context.Context, id string) (domain.Level, error) {
	if l, ok := f.levels[id]; ok {
		return l, nil
	}
	return domain.Level{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListLevels(_ context.Context) ([]domain.Level, error) {
	out := make([]domain.Level, 0, len(f.levels))
	for _, l := range f.levels {
		out = append(out, l)
	}
	return out, nil
}

func (f *fakeCatalog) CreateGradeLevel(_ context.Context, g domain.GradeLevel) error {
	if err := f.claim("grade|" + g.LevelID() + "|" + itoa(g.Ordinal())); err != nil {
		return err
	}
	f.gradeLevels[g.ID()] = g
	return nil
}

func (f *fakeCatalog) GetGradeLevel(_ context.Context, id string) (domain.GradeLevel, error) {
	if g, ok := f.gradeLevels[id]; ok {
		return g, nil
	}
	return domain.GradeLevel{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListGradeLevels(_ context.Context, levelID string) ([]domain.GradeLevel, error) {
	out := make([]domain.GradeLevel, 0)
	for _, g := range f.gradeLevels {
		if g.LevelID() == levelID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeCatalog) CreateSubject(_ context.Context, s domain.Subject) error {
	if err := f.claim("subject|" + s.GradeLevelID() + "|" + s.Name()); err != nil {
		return err
	}
	f.subjects[s.ID()] = s
	return nil
}

func (f *fakeCatalog) GetSubject(_ context.Context, id string) (domain.Subject, error) {
	if s, ok := f.subjects[id]; ok {
		return s, nil
	}
	return domain.Subject{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListSubjects(_ context.Context, gradeLevelID string) ([]domain.Subject, error) {
	out := make([]domain.Subject, 0)
	for _, s := range f.subjects {
		if s.GradeLevelID() == gradeLevelID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeCatalog) CreateTerm(_ context.Context, t domain.Term) error {
	if err := f.claim("term|" + t.CycleID() + "|" + t.LevelID() + "|" + itoa(t.Ordinal())); err != nil {
		return err
	}
	f.terms[t.ID()] = t
	return nil
}

func (f *fakeCatalog) GetTerm(_ context.Context, id string) (domain.Term, error) {
	if t, ok := f.terms[id]; ok {
		return t, nil
	}
	return domain.Term{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListTerms(_ context.Context, cycleID, levelID string) ([]domain.Term, error) {
	out := make([]domain.Term, 0)
	for _, t := range f.terms {
		if t.CycleID() != cycleID {
			continue
		}
		if levelID != "" && t.LevelID() != levelID {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeCatalog) CreateGroup(_ context.Context, g domain.Group) error {
	if err := f.claim("group|" + g.CycleID() + "|" + g.GradeLevelID() + "|" + g.Name()); err != nil {
		return err
	}
	f.groups[g.ID()] = g
	return nil
}

func (f *fakeCatalog) GetGroup(_ context.Context, id string) (domain.Group, error) {
	if g, ok := f.groups[id]; ok {
		return g, nil
	}
	return domain.Group{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListGroups(_ context.Context, cycleID, gradeLevelID string) ([]domain.Group, error) {
	out := make([]domain.Group, 0)
	for _, g := range f.groups {
		if g.CycleID() != cycleID {
			continue
		}
		if gradeLevelID != "" && g.GradeLevelID() != gradeLevelID {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}

// itoa is a tiny local int→string for building unique keys.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// --- harness ---------------------------------------------------------------

func newHandler(t *testing.T) (*grpcadapter.ConfigHandler, *fakeCatalog) {
	t.Helper()
	repo := newFake()
	ids := 0
	svc := usecase.NewCatalogService(repo, tenant, func() string { ids++; return "id-" + itoa(ids) })
	return grpcadapter.NewConfigHandler(svc, usecase.NewHealthService("config")), repo
}

func ctx() context.Context { return context.Background() }

// codeOf extracts the Connect status code from an error (CodeUnknown if the
// error neither is nor wraps a *connect.Error).
func codeOf(err error) connect.Code { return connect.CodeOf(err) }

// seedLevel / seedCycle / seedGradeLevel create valid parents via the handler.

func seedLevel(t *testing.T, h *grpcadapter.ConfigHandler) string {
	t.Helper()
	res, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name:                  "Primaria",
		SepCode:               "SEP-PRIM",
		EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE,
	}))
	if err != nil {
		t.Fatalf("seed level: %v", err)
	}
	return res.Msg.GetLevel().GetId()
}

func seedCycle(t *testing.T, h *grpcadapter.ConfigHandler) string {
	t.Helper()
	res, err := h.CreateCycle(ctx(), connect.NewRequest(&configv1.CreateCycleRequest{
		Name:     "2026-2027",
		Status:   configv1.CycleStatus_CYCLE_STATUS_DRAFT,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if err != nil {
		t.Fatalf("seed cycle: %v", err)
	}
	return res.Msg.GetCycle().GetId()
}

func seedGradeLevel(t *testing.T, h *grpcadapter.ConfigHandler, levelID string) string {
	t.Helper()
	res, err := h.CreateGradeLevel(ctx(), connect.NewRequest(&configv1.CreateGradeLevelRequest{
		LevelId: levelID,
		Name:    "1° Primaria",
		Ordinal: 1,
	}))
	if err != nil {
		t.Fatalf("seed grade level: %v", err)
	}
	return res.Msg.GetGradeLevel().GetId()
}

// --- Health ----------------------------------------------------------------

func TestHealth(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.Health(ctx(), connect.NewRequest(&configv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

// --- Level -----------------------------------------------------------------

func TestCreateLevel(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name:                  "Primaria",
		SepCode:               "SEP-PRIM",
		EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_TERM,
	}))
	if err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	l := res.Msg.GetLevel()
	if l.GetId() == "" || l.GetTenantId() != tenant || l.GetName() != "Primaria" {
		t.Errorf("level wrong: %+v", l)
	}
	if l.GetEnrollmentGranularity() != configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_TERM {
		t.Errorf("granularity = %v, want TERM", l.GetEnrollmentGranularity())
	}
}

func TestCreateLevelUnspecifiedGranularityIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name:                  "Primaria",
		SepCode:               "SEP-PRIM",
		EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_UNSPECIFIED,
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("unspecified granularity: code = %v, want InvalidArgument", got)
	}
}

func TestCreateLevelDuplicateIsAlreadyExists(t *testing.T) {
	h, _ := newHandler(t)
	seedLevel(t, h)
	_, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name:                  "Primaria",
		SepCode:               "SEP-PRIM",
		EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE,
	}))
	if got := codeOf(err); got != connect.CodeAlreadyExists {
		t.Errorf("duplicate level: code = %v, want AlreadyExists", got)
	}
}

func TestGetLevel(t *testing.T) {
	h, _ := newHandler(t)
	id := seedLevel(t, h)
	res, err := h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: id}))
	if err != nil || res.Msg.GetLevel().GetId() != id {
		t.Fatalf("GetLevel: %+v %v", res, err)
	}
	_, err = h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: "nope"}))
	if got := codeOf(err); got != connect.CodeNotFound {
		t.Errorf("unknown level: code = %v, want NotFound", got)
	}
}

func TestListLevels(t *testing.T) {
	h, _ := newHandler(t)
	seedLevel(t, h)
	res, err := h.ListLevels(ctx(), connect.NewRequest(&configv1.ListLevelsRequest{}))
	if err != nil || len(res.Msg.GetLevels()) != 1 {
		t.Errorf("ListLevels: %d %v", len(res.Msg.GetLevels()), err)
	}
}

// --- Cycle -----------------------------------------------------------------

func TestCreateCycle(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.CreateCycle(ctx(), connect.NewRequest(&configv1.CreateCycleRequest{
		Name:     "2026-2027",
		Status:   configv1.CycleStatus_CYCLE_STATUS_ACTIVE,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	c := res.Msg.GetCycle()
	if c.GetId() == "" || c.GetName() != "2026-2027" {
		t.Errorf("cycle wrong: %+v", c)
	}
	if c.GetStatus() != configv1.CycleStatus_CYCLE_STATUS_ACTIVE {
		t.Errorf("status = %v, want ACTIVE", c.GetStatus())
	}
	if !c.GetStartsOn().AsTime().Equal(someDate) || !c.GetEndsOn().AsTime().Equal(laterDate) {
		t.Errorf("dates round-trip wrong: starts=%v ends=%v", c.GetStartsOn().AsTime(), c.GetEndsOn().AsTime())
	}
}

func TestCreateCycleUnspecifiedStatusIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateCycle(ctx(), connect.NewRequest(&configv1.CreateCycleRequest{
		Name:     "2026-2027",
		Status:   configv1.CycleStatus_CYCLE_STATUS_UNSPECIFIED,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("unspecified status: code = %v, want InvalidArgument", got)
	}
}

func TestCreateCycleDuplicateIsAlreadyExists(t *testing.T) {
	h, _ := newHandler(t)
	seedCycle(t, h)
	_, err := h.CreateCycle(ctx(), connect.NewRequest(&configv1.CreateCycleRequest{
		Name:     "2026-2027",
		Status:   configv1.CycleStatus_CYCLE_STATUS_DRAFT,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if got := codeOf(err); got != connect.CodeAlreadyExists {
		t.Errorf("duplicate cycle: code = %v, want AlreadyExists", got)
	}
}

func TestGetCycle(t *testing.T) {
	h, _ := newHandler(t)
	id := seedCycle(t, h)
	res, err := h.GetCycle(ctx(), connect.NewRequest(&configv1.GetCycleRequest{Id: id}))
	if err != nil || res.Msg.GetCycle().GetId() != id {
		t.Fatalf("GetCycle: %+v %v", res, err)
	}
	_, err = h.GetCycle(ctx(), connect.NewRequest(&configv1.GetCycleRequest{Id: "nope"}))
	if got := codeOf(err); got != connect.CodeNotFound {
		t.Errorf("unknown cycle: code = %v, want NotFound", got)
	}
}

func TestListCycles(t *testing.T) {
	h, _ := newHandler(t)
	seedCycle(t, h)
	res, err := h.ListCycles(ctx(), connect.NewRequest(&configv1.ListCyclesRequest{}))
	if err != nil || len(res.Msg.GetCycles()) != 1 {
		t.Errorf("ListCycles: %d %v", len(res.Msg.GetCycles()), err)
	}
}

// --- GradeLevel ------------------------------------------------------------

func TestCreateGradeLevel(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	res, err := h.CreateGradeLevel(ctx(), connect.NewRequest(&configv1.CreateGradeLevelRequest{
		LevelId: levelID,
		Name:    "1° Primaria",
		Ordinal: 1,
	}))
	if err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	g := res.Msg.GetGradeLevel()
	if g.GetLevelId() != levelID || g.GetOrdinal() != 1 {
		t.Errorf("grade level wrong: %+v", g)
	}
}

func TestCreateGradeLevelMissingParentIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateGradeLevel(ctx(), connect.NewRequest(&configv1.CreateGradeLevelRequest{
		LevelId: "no-such-level",
		Name:    "1° Primaria",
		Ordinal: 1,
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("missing parent level: code = %v, want InvalidArgument", got)
	}
}

func TestGetGradeLevel(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	id := seedGradeLevel(t, h, levelID)
	res, err := h.GetGradeLevel(ctx(), connect.NewRequest(&configv1.GetGradeLevelRequest{Id: id}))
	if err != nil || res.Msg.GetGradeLevel().GetId() != id {
		t.Fatalf("GetGradeLevel: %+v %v", res, err)
	}
}

func TestListGradeLevels(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	seedGradeLevel(t, h, levelID)
	res, err := h.ListGradeLevels(ctx(), connect.NewRequest(&configv1.ListGradeLevelsRequest{LevelId: levelID}))
	if err != nil || len(res.Msg.GetGradeLevels()) != 1 {
		t.Errorf("ListGradeLevels: %d %v", len(res.Msg.GetGradeLevels()), err)
	}
}

// --- Subject ---------------------------------------------------------------

func TestCreateSubject(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	gradeID := seedGradeLevel(t, h, levelID)
	res, err := h.CreateSubject(ctx(), connect.NewRequest(&configv1.CreateSubjectRequest{
		GradeLevelId: gradeID,
		Name:         "Matematicas",
	}))
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if res.Msg.GetSubject().GetGradeLevelId() != gradeID {
		t.Errorf("subject wrong: %+v", res.Msg.GetSubject())
	}
}

func TestCreateSubjectMissingParentIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateSubject(ctx(), connect.NewRequest(&configv1.CreateSubjectRequest{
		GradeLevelId: "no-such-grade",
		Name:         "Matematicas",
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("missing parent grade: code = %v, want InvalidArgument", got)
	}
}

func TestGetSubjectAndList(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	gradeID := seedGradeLevel(t, h, levelID)
	created, err := h.CreateSubject(ctx(), connect.NewRequest(&configv1.CreateSubjectRequest{
		GradeLevelId: gradeID,
		Name:         "Matematicas",
	}))
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	id := created.Msg.GetSubject().GetId()
	got, err := h.GetSubject(ctx(), connect.NewRequest(&configv1.GetSubjectRequest{Id: id}))
	if err != nil || got.Msg.GetSubject().GetId() != id {
		t.Fatalf("GetSubject: %+v %v", got, err)
	}
	list, err := h.ListSubjects(ctx(), connect.NewRequest(&configv1.ListSubjectsRequest{GradeLevelId: gradeID}))
	if err != nil || len(list.Msg.GetSubjects()) != 1 {
		t.Errorf("ListSubjects: %d %v", len(list.Msg.GetSubjects()), err)
	}
}

// --- Term ------------------------------------------------------------------

func TestCreateTerm(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	levelID := seedLevel(t, h)
	res, err := h.CreateTerm(ctx(), connect.NewRequest(&configv1.CreateTermRequest{
		CycleId:  cycleID,
		LevelId:  levelID,
		Name:     "T1",
		Ordinal:  1,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if err != nil {
		t.Fatalf("CreateTerm: %v", err)
	}
	term := res.Msg.GetTerm()
	if term.GetCycleId() != cycleID || term.GetLevelId() != levelID || term.GetOrdinal() != 1 {
		t.Errorf("term wrong: %+v", term)
	}
	if !term.GetStartsOn().AsTime().Equal(someDate) {
		t.Errorf("starts_on round-trip wrong: %v", term.GetStartsOn().AsTime())
	}
}

func TestCreateTermMissingParentIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	_, err := h.CreateTerm(ctx(), connect.NewRequest(&configv1.CreateTermRequest{
		CycleId:  "no-such-cycle",
		LevelId:  levelID,
		Name:     "T1",
		Ordinal:  1,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("missing cycle: code = %v, want InvalidArgument", got)
	}
}

func TestGetTermAndList(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	levelID := seedLevel(t, h)
	created, err := h.CreateTerm(ctx(), connect.NewRequest(&configv1.CreateTermRequest{
		CycleId:  cycleID,
		LevelId:  levelID,
		Name:     "T1",
		Ordinal:  1,
		StartsOn: timestamppb.New(someDate),
		EndsOn:   timestamppb.New(laterDate),
	}))
	if err != nil {
		t.Fatalf("CreateTerm: %v", err)
	}
	id := created.Msg.GetTerm().GetId()
	got, err := h.GetTerm(ctx(), connect.NewRequest(&configv1.GetTermRequest{Id: id}))
	if err != nil || got.Msg.GetTerm().GetId() != id {
		t.Fatalf("GetTerm: %+v %v", got, err)
	}
	list, err := h.ListTerms(ctx(), connect.NewRequest(&configv1.ListTermsRequest{CycleId: cycleID}))
	if err != nil || len(list.Msg.GetTerms()) != 1 {
		t.Errorf("ListTerms: %d %v", len(list.Msg.GetTerms()), err)
	}
}

// --- Group -----------------------------------------------------------------

func TestCreateGroup(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	levelID := seedLevel(t, h)
	gradeID := seedGradeLevel(t, h, levelID)
	res, err := h.CreateGroup(ctx(), connect.NewRequest(&configv1.CreateGroupRequest{
		CycleId:      cycleID,
		GradeLevelId: gradeID,
		Name:         "A",
		Capacity:     30,
	}))
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g := res.Msg.GetGroup()
	if g.GetCycleId() != cycleID || g.GetGradeLevelId() != gradeID || g.GetCapacity() != 30 {
		t.Errorf("group wrong: %+v", g)
	}
}

func TestCreateGroupMissingParentIsInvalid(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	_, err := h.CreateGroup(ctx(), connect.NewRequest(&configv1.CreateGroupRequest{
		CycleId:      cycleID,
		GradeLevelId: "no-such-grade",
		Name:         "A",
		Capacity:     30,
	}))
	if got := codeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("missing grade: code = %v, want InvalidArgument", got)
	}
}

func TestGetGroupAndList(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	levelID := seedLevel(t, h)
	gradeID := seedGradeLevel(t, h, levelID)
	created, err := h.CreateGroup(ctx(), connect.NewRequest(&configv1.CreateGroupRequest{
		CycleId:      cycleID,
		GradeLevelId: gradeID,
		Name:         "A",
		Capacity:     30,
	}))
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	id := created.Msg.GetGroup().GetId()
	got, err := h.GetGroup(ctx(), connect.NewRequest(&configv1.GetGroupRequest{Id: id}))
	if err != nil || got.Msg.GetGroup().GetId() != id {
		t.Fatalf("GetGroup: %+v %v", got, err)
	}
	list, err := h.ListGroups(ctx(), connect.NewRequest(&configv1.ListGroupsRequest{CycleId: cycleID}))
	if err != nil || len(list.Msg.GetGroups()) != 1 {
		t.Errorf("ListGroups: %d %v", len(list.Msg.GetGroups()), err)
	}
}
