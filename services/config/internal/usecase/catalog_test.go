package usecase_test

import (
	"context"
	"sort"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
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
// The fake models real repository behavior: it enforces the same
// unique-key scopes as the DB (duplicate → KindConflict) and answers Get* so the
// use case's parent-existence checks work against persisted state.

type fakeCatalog struct {
	cycles      map[string]domain.Cycle
	levels      map[string]domain.Level
	gradeLevels map[string]domain.GradeLevel
	subjects    map[string]domain.Subject
	terms       map[string]domain.Term
	groups      map[string]domain.Group
	teachers    map[string]domain.Teacher
	assignments map[string]domain.TeachingAssignment
	periods     map[string]domain.NonWorkingPeriod
	events      map[string]domain.SchoolEvent
	profile     *domain.SchoolProfile
	unique      map[string]bool // scope-qualified unique keys across all entities
}

func newFake() *fakeCatalog {
	return &fakeCatalog{
		cycles:      map[string]domain.Cycle{},
		levels:      map[string]domain.Level{},
		gradeLevels: map[string]domain.GradeLevel{},
		subjects:    map[string]domain.Subject{},
		terms:       map[string]domain.Term{},
		groups:      map[string]domain.Group{},
		teachers:    map[string]domain.Teacher{},
		assignments: map[string]domain.TeachingAssignment{},
		periods:     map[string]domain.NonWorkingPeriod{},
		events:      map[string]domain.SchoolEvent{},
		unique:      map[string]bool{},
	}
}

// claim records a unique key, returning KindConflict if it is already taken.
func (f *fakeCatalog) claim(key string) error {
	if f.unique[key] {
		return glerrors.ErrConflict
	}
	f.unique[key] = true
	return nil
}

func (f *fakeCatalog) GetSchoolProfile(_ context.Context) (domain.SchoolProfile, bool, error) {
	if f.profile == nil {
		return domain.SchoolProfile{}, false, nil
	}
	return *f.profile, true, nil
}

func (f *fakeCatalog) UpsertSchoolProfile(_ context.Context, p domain.SchoolProfile) error {
	f.profile = &p
	return nil
}

func (f *fakeCatalog) CreateSchoolEvent(_ context.Context, e domain.SchoolEvent) error {
	f.events[e.ID()] = e
	return nil
}

func (f *fakeCatalog) ListSchoolEvents(_ context.Context, cycleID string) ([]domain.SchoolEvent, error) {
	out := make([]domain.SchoolEvent, 0, len(f.events))
	for _, e := range f.events {
		if e.CycleID() == cycleID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From().Before(out[j].From()) })
	return out, nil
}

func (f *fakeCatalog) DeleteSchoolEvent(_ context.Context, id string) error {
	if _, ok := f.events[id]; !ok {
		return glerrors.ErrNotFound
	}
	delete(f.events, id)
	return nil
}

func (f *fakeCatalog) CreateNonWorkingPeriod(_ context.Context, p domain.NonWorkingPeriod) error {
	if err := f.claim("period|" + p.CycleID() + "|" + p.Label()); err != nil {
		return err
	}
	f.periods[p.ID()] = p
	return nil
}

func (f *fakeCatalog) ListNonWorkingPeriods(_ context.Context, cycleID string) ([]domain.NonWorkingPeriod, error) {
	out := make([]domain.NonWorkingPeriod, 0, len(f.periods))
	for _, p := range f.periods {
		if p.CycleID() == cycleID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From().Before(out[j].From()) })
	return out, nil
}

func (f *fakeCatalog) DeleteNonWorkingPeriod(_ context.Context, id string) error {
	if _, ok := f.periods[id]; !ok {
		return glerrors.ErrNotFound
	}
	delete(f.periods, id)
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

func (f *fakeCatalog) ListTerms(_ context.Context, cycleID string, levelID string) ([]domain.Term, error) {
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

func (f *fakeCatalog) ListGroups(_ context.Context, cycleID string, gradeLevelID string) ([]domain.Group, error) {
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

func (f *fakeCatalog) CreateTeacher(_ context.Context, te domain.Teacher) error {
	if err := f.claim("teacher|" + te.Email()); err != nil {
		return err
	}
	f.teachers[te.ID()] = te
	return nil
}

func (f *fakeCatalog) GetTeacher(_ context.Context, id string) (domain.Teacher, error) {
	if te, ok := f.teachers[id]; ok {
		return te, nil
	}
	return domain.Teacher{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListTeachers(_ context.Context) ([]domain.Teacher, error) {
	out := make([]domain.Teacher, 0, len(f.teachers))
	for _, te := range f.teachers {
		out = append(out, te)
	}
	return out, nil
}

func (f *fakeCatalog) SetTeacherUserID(_ context.Context, teacherID string, userID *string) error {
	te, ok := f.teachers[teacherID]
	if !ok {
		return glerrors.ErrNotFound
	}
	link := ""
	if userID != nil {
		link = *userID
	}
	f.teachers[teacherID] = te.WithUserID(link)
	return nil
}

func (f *fakeCatalog) CreateTeachingAssignment(_ context.Context, a domain.TeachingAssignment) error {
	if err := f.claim("assignment|" + a.CycleID() + "|" + a.GroupID() + "|" + a.SubjectID()); err != nil {
		return err
	}
	f.assignments[a.ID()] = a
	return nil
}

func (f *fakeCatalog) GetTeachingAssignment(_ context.Context, id string) (domain.TeachingAssignment, error) {
	if a, ok := f.assignments[id]; ok {
		return a, nil
	}
	return domain.TeachingAssignment{}, glerrors.ErrNotFound
}

func (f *fakeCatalog) ListTeachingAssignments(_ context.Context, cycleID, groupID, teacherID string) ([]domain.TeachingAssignment, error) {
	out := make([]domain.TeachingAssignment, 0)
	for _, a := range f.assignments {
		if cycleID != "" && a.CycleID() != cycleID {
			continue
		}
		if groupID != "" && a.GroupID() != groupID {
			continue
		}
		if teacherID != "" && a.TeacherID() != teacherID {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// itoa is a tiny local int→string for building unique keys (avoids strconv noise).
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

func newService(t *testing.T) (*usecase.CatalogService, *fakeCatalog) {
	t.Helper()
	repo := newFake()
	ids := 0
	svc := usecase.NewCatalogService(
		repo,
		tenant,
		func() string { ids++; return "id-" + itoa(ids) },
	)
	return svc, repo
}

func ctx() context.Context { return context.Background() }

// seedLevel / seedCycle / seedGradeLevel create valid parents for nested tests.

func seedLevel(t *testing.T, svc *usecase.CatalogService) domain.Level {
	t.Helper()
	l, err := svc.CreateLevel(ctx(), "Primaria", "SEP-PRIM", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("seed level: %v", err)
	}
	return l
}

func seedCycle(t *testing.T, svc *usecase.CatalogService) domain.Cycle {
	t.Helper()
	c, err := svc.CreateCycle(ctx(), "2026-2027", domain.CycleStatusDraft, someDate, laterDate)
	if err != nil {
		t.Fatalf("seed cycle: %v", err)
	}
	return c
}

func seedGradeLevel(t *testing.T, svc *usecase.CatalogService, levelID string) domain.GradeLevel {
	t.Helper()
	g, err := svc.CreateGradeLevel(ctx(), levelID, "1° Primaria", 1)
	if err != nil {
		t.Fatalf("seed grade level: %v", err)
	}
	return g
}

// --- Cycle -----------------------------------------------------------------

func TestCreateCycle(t *testing.T) {
	svc, repo := newService(t)
	c, err := svc.CreateCycle(ctx(), "2026-2027", domain.CycleStatusDraft, someDate, laterDate)
	if err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	if c.ID() == "" || c.TenantID() != tenant || c.Name() != "2026-2027" || c.Status() != domain.CycleStatusDraft {
		t.Errorf("cycle wrong: %+v", c)
	}
	if len(repo.cycles) != 1 {
		t.Errorf("expected 1 persisted cycle, got %d", len(repo.cycles))
	}
}

func TestCreateCycleDefaultsUnspecifiedToDraft(t *testing.T) {
	svc, repo := newService(t)
	// Creating with an unspecified status defaults to draft (matches the proto
	// contract, the DDL DEFAULT 'draft', and the fewest-clicks UX).
	c, err := svc.CreateCycle(ctx(), "2026-2027", domain.CycleStatusUnspecified, someDate, laterDate)
	if err != nil {
		t.Fatalf("CreateCycle with unspecified status: %v", err)
	}
	if c.Status() != domain.CycleStatusDraft {
		t.Errorf("status = %v, want CycleStatusDraft", c.Status())
	}
	if len(repo.cycles) != 1 {
		t.Errorf("expected 1 persisted cycle, got %d", len(repo.cycles))
	}
	if got := repo.cycles[c.ID()]; got.Status() != domain.CycleStatusDraft {
		t.Errorf("persisted status = %v, want CycleStatusDraft", got.Status())
	}
}

func TestCreateCycleInvalidArg(t *testing.T) {
	svc, repo := newService(t)
	// empty name → constructor KindInvalid.
	if _, err := svc.CreateCycle(ctx(), "", domain.CycleStatusDraft, someDate, laterDate); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("empty name: want KindInvalid, got %v", err)
	}
	// endsOn before startsOn → constructor KindInvalid.
	if _, err := svc.CreateCycle(ctx(), "bad", domain.CycleStatusDraft, laterDate, someDate); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("reversed dates: want KindInvalid, got %v", err)
	}
	if len(repo.cycles) != 0 {
		t.Errorf("nothing should be persisted on invalid input, got %d", len(repo.cycles))
	}
}

func TestCreateCycleDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	seedCycle(t, svc)
	if _, err := svc.CreateCycle(ctx(), "2026-2027", domain.CycleStatusDraft, someDate, laterDate); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate cycle name: want KindConflict, got %v", err)
	}
}

func TestGetCycle(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	got, err := svc.GetCycle(ctx(), c.ID())
	if err != nil || got.ID() != c.ID() {
		t.Errorf("GetCycle: %+v %v", got, err)
	}
	if _, err := svc.GetCycle(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown cycle: want KindNotFound, got %v", err)
	}
}

func TestListCycles(t *testing.T) {
	svc, _ := newService(t)
	seedCycle(t, svc)
	got, err := svc.ListCycles(ctx())
	if err != nil || len(got) != 1 {
		t.Errorf("ListCycles: %d %v", len(got), err)
	}
}

// --- Level -----------------------------------------------------------------

func TestCreateLevel(t *testing.T) {
	svc, repo := newService(t)
	l, err := svc.CreateLevel(ctx(), "Primaria", "SEP-PRIM", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	if l.ID() == "" || l.TenantID() != tenant || l.Granularity() != domain.GranularityCycle {
		t.Errorf("level wrong: %+v", l)
	}
	if len(repo.levels) != 1 {
		t.Errorf("expected 1 persisted level, got %d", len(repo.levels))
	}
}

func TestCreateLevelInvalidArg(t *testing.T) {
	svc, _ := newService(t)
	// unknown granularity → constructor KindInvalid.
	if _, err := svc.CreateLevel(ctx(), "Primaria", "SEP-PRIM", domain.GranularityUnspecified); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("bad granularity: want KindInvalid, got %v", err)
	}
}

func TestCreateLevelDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	seedLevel(t, svc)
	if _, err := svc.CreateLevel(ctx(), "Primaria", "SEP-PRIM", domain.GranularityCycle); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate level name: want KindConflict, got %v", err)
	}
}

func TestListLevels(t *testing.T) {
	svc, _ := newService(t)
	seedLevel(t, svc)
	got, err := svc.ListLevels(ctx())
	if err != nil || len(got) != 1 {
		t.Errorf("ListLevels: %d %v", len(got), err)
	}
}

// --- GradeLevel ------------------------------------------------------------

func TestCreateGradeLevel(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g, err := svc.CreateGradeLevel(ctx(), l.ID(), "1° Primaria", 1)
	if err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	if g.LevelID() != l.ID() || g.Ordinal() != 1 {
		t.Errorf("grade level wrong: %+v", g)
	}
	if len(repo.gradeLevels) != 1 {
		t.Errorf("expected 1 persisted grade level, got %d", len(repo.gradeLevels))
	}
}

func TestCreateGradeLevelMissingParentIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	_, err := svc.CreateGradeLevel(ctx(), "no-such-level", "1° Primaria", 1)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing parent level: want KindInvalid, got %v", err)
	}
	if len(repo.gradeLevels) != 0 {
		t.Errorf("nothing should be persisted when parent is missing, got %d", len(repo.gradeLevels))
	}
}

func TestCreateGradeLevelInvalidArg(t *testing.T) {
	svc, _ := newService(t)
	l := seedLevel(t, svc)
	// ordinal 0 → constructor KindInvalid.
	if _, err := svc.CreateGradeLevel(ctx(), l.ID(), "1° Primaria", 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ordinal 0: want KindInvalid, got %v", err)
	}
}

func TestCreateGradeLevelDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	l := seedLevel(t, svc)
	seedGradeLevel(t, svc, l.ID())
	// same (level_id, ordinal) → conflict.
	if _, err := svc.CreateGradeLevel(ctx(), l.ID(), "otro nombre", 1); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate (level,ordinal): want KindConflict, got %v", err)
	}
}

func TestListGradeLevelsScoped(t *testing.T) {
	svc, _ := newService(t)
	l1 := seedLevel(t, svc)
	l2, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("second level: %v", err)
	}
	seedGradeLevel(t, svc, l1.ID())
	if _, err := svc.CreateGradeLevel(ctx(), l2.ID(), "1° Secundaria", 1); err != nil {
		t.Fatalf("grade under l2: %v", err)
	}
	got, err := svc.ListGradeLevels(ctx(), l1.ID())
	if err != nil || len(got) != 1 || got[0].LevelID() != l1.ID() {
		t.Errorf("ListGradeLevels scoped to l1: %d %v", len(got), err)
	}
}

// --- Subject ---------------------------------------------------------------

func TestCreateSubject(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	s, err := svc.CreateSubject(ctx(), g.ID(), "Matemáticas")
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if s.GradeLevelID() != g.ID() || s.Name() != "Matemáticas" {
		t.Errorf("subject wrong: %+v", s)
	}
	if len(repo.subjects) != 1 {
		t.Errorf("expected 1 persisted subject, got %d", len(repo.subjects))
	}
}

func TestCreateSubjectMissingParentIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	_, err := svc.CreateSubject(ctx(), "no-such-grade", "Matemáticas")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing parent grade level: want KindInvalid, got %v", err)
	}
	if len(repo.subjects) != 0 {
		t.Errorf("nothing should be persisted when parent is missing, got %d", len(repo.subjects))
	}
}

func TestCreateSubjectInvalidArg(t *testing.T) {
	svc, _ := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	if _, err := svc.CreateSubject(ctx(), g.ID(), ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("empty name: want KindInvalid, got %v", err)
	}
}

func TestCreateSubjectDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	if _, err := svc.CreateSubject(ctx(), g.ID(), "Matemáticas"); err != nil {
		t.Fatalf("first subject: %v", err)
	}
	if _, err := svc.CreateSubject(ctx(), g.ID(), "Matemáticas"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate subject: want KindConflict, got %v", err)
	}
}

func TestListSubjectsScoped(t *testing.T) {
	svc, _ := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	if _, err := svc.CreateSubject(ctx(), g.ID(), "Matemáticas"); err != nil {
		t.Fatalf("subject: %v", err)
	}
	got, err := svc.ListSubjects(ctx(), g.ID())
	if err != nil || len(got) != 1 {
		t.Errorf("ListSubjects: %d %v", len(got), err)
	}
	if other, _ := svc.ListSubjects(ctx(), "other-grade"); len(other) != 0 {
		t.Errorf("expected no subjects for other grade, got %d", len(other))
	}
}

// --- Term ------------------------------------------------------------------

func TestCreateTerm(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	term, err := svc.CreateTerm(ctx(), c.ID(), l.ID(), "T1", 1, someDate, laterDate)
	if err != nil {
		t.Fatalf("CreateTerm: %v", err)
	}
	if term.CycleID() != c.ID() || term.LevelID() != l.ID() || term.Ordinal() != 1 {
		t.Errorf("term wrong: %+v", term)
	}
	if len(repo.terms) != 1 {
		t.Errorf("expected 1 persisted term, got %d", len(repo.terms))
	}
}

func TestCreateTermMissingCycleIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	_, err := svc.CreateTerm(ctx(), "no-such-cycle", l.ID(), "T1", 1, someDate, laterDate)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing cycle: want KindInvalid, got %v", err)
	}
	if len(repo.terms) != 0 {
		t.Errorf("nothing persisted when cycle missing, got %d", len(repo.terms))
	}
}

func TestCreateTermMissingLevelIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	_, err := svc.CreateTerm(ctx(), c.ID(), "no-such-level", "T1", 1, someDate, laterDate)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing level: want KindInvalid, got %v", err)
	}
	if len(repo.terms) != 0 {
		t.Errorf("nothing persisted when level missing, got %d", len(repo.terms))
	}
}

func TestCreateTermInvalidArg(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	// reversed dates → constructor KindInvalid.
	if _, err := svc.CreateTerm(ctx(), c.ID(), l.ID(), "T1", 1, laterDate, someDate); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("reversed dates: want KindInvalid, got %v", err)
	}
}

func TestCreateTermDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	if _, err := svc.CreateTerm(ctx(), c.ID(), l.ID(), "T1", 1, someDate, laterDate); err != nil {
		t.Fatalf("first term: %v", err)
	}
	if _, err := svc.CreateTerm(ctx(), c.ID(), l.ID(), "T1-bis", 1, someDate, laterDate); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate (cycle,level,ordinal): want KindConflict, got %v", err)
	}
}

func TestListTermsScopedByCycleAndLevel(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l1 := seedLevel(t, svc)
	l2, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("second level: %v", err)
	}
	if _, err := svc.CreateTerm(ctx(), c.ID(), l1.ID(), "T1", 1, someDate, laterDate); err != nil {
		t.Fatalf("term l1: %v", err)
	}
	if _, err := svc.CreateTerm(ctx(), c.ID(), l2.ID(), "T1", 1, someDate, laterDate); err != nil {
		t.Fatalf("term l2: %v", err)
	}
	all, err := svc.ListTerms(ctx(), c.ID(), "")
	if err != nil || len(all) != 2 {
		t.Errorf("ListTerms all levels: %d %v", len(all), err)
	}
	only1, err := svc.ListTerms(ctx(), c.ID(), l1.ID())
	if err != nil || len(only1) != 1 || only1[0].LevelID() != l1.ID() {
		t.Errorf("ListTerms scoped to l1: %d %v", len(only1), err)
	}
}

// --- Group -----------------------------------------------------------------

func TestCreateGroup(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	grp, err := svc.CreateGroup(ctx(), c.ID(), g.ID(), "A", 30)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if grp.CycleID() != c.ID() || grp.GradeLevelID() != g.ID() || grp.Capacity() != 30 {
		t.Errorf("group wrong: %+v", grp)
	}
	if len(repo.groups) != 1 {
		t.Errorf("expected 1 persisted group, got %d", len(repo.groups))
	}
}

func TestCreateGroupMissingCycleIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	_, err := svc.CreateGroup(ctx(), "no-such-cycle", g.ID(), "A", 30)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing cycle: want KindInvalid, got %v", err)
	}
	if len(repo.groups) != 0 {
		t.Errorf("nothing persisted when cycle missing, got %d", len(repo.groups))
	}
}

func TestCreateGroupMissingGradeLevelIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	_, err := svc.CreateGroup(ctx(), c.ID(), "no-such-grade", "A", 30)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing grade level: want KindInvalid, got %v", err)
	}
	if len(repo.groups) != 0 {
		t.Errorf("nothing persisted when grade level missing, got %d", len(repo.groups))
	}
}

func TestCreateGroupInvalidArg(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	// negative capacity → constructor KindInvalid.
	if _, err := svc.CreateGroup(ctx(), c.ID(), g.ID(), "A", -1); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("negative capacity: want KindInvalid, got %v", err)
	}
}

func TestCreateGroupDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	if _, err := svc.CreateGroup(ctx(), c.ID(), g.ID(), "A", 30); err != nil {
		t.Fatalf("first group: %v", err)
	}
	if _, err := svc.CreateGroup(ctx(), c.ID(), g.ID(), "A", 25); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate (cycle,grade,name): want KindConflict, got %v", err)
	}
}

func TestListGroupsScopedByCycleAndGrade(t *testing.T) {
	svc, _ := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	g1 := seedGradeLevel(t, svc, l.ID())
	g2, err := svc.CreateGradeLevel(ctx(), l.ID(), "2° Primaria", 2)
	if err != nil {
		t.Fatalf("second grade: %v", err)
	}
	if _, err := svc.CreateGroup(ctx(), c.ID(), g1.ID(), "A", 30); err != nil {
		t.Fatalf("group g1: %v", err)
	}
	if _, err := svc.CreateGroup(ctx(), c.ID(), g2.ID(), "A", 30); err != nil {
		t.Fatalf("group g2: %v", err)
	}
	all, err := svc.ListGroups(ctx(), c.ID(), "")
	if err != nil || len(all) != 2 {
		t.Errorf("ListGroups all grades: %d %v", len(all), err)
	}
	only1, err := svc.ListGroups(ctx(), c.ID(), g1.ID())
	if err != nil || len(only1) != 1 || only1[0].GradeLevelID() != g1.ID() {
		t.Errorf("ListGroups scoped to g1: %d %v", len(only1), err)
	}
}

// --- Get delegation (representative) ---------------------------------------

func TestGetDelegatesNotFound(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.GetLevel(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetLevel unknown: want KindNotFound, got %v", err)
	}
	if _, err := svc.GetGradeLevel(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetGradeLevel unknown: want KindNotFound, got %v", err)
	}
	if _, err := svc.GetSubject(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetSubject unknown: want KindNotFound, got %v", err)
	}
	if _, err := svc.GetTerm(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetTerm unknown: want KindNotFound, got %v", err)
	}
	if _, err := svc.GetGroup(ctx(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetGroup unknown: want KindNotFound, got %v", err)
	}
}

// ActivateCycle in the double demotes the others exactly as the transaction does,
// so a test that expects one live year fails here rather than only against
// Postgres.
func (f *fakeCatalog) ActivateCycle(_ context.Context, c domain.Cycle) error {
	if _, ok := f.cycles[c.ID()]; !ok {
		return glerrors.ErrNotFound
	}
	for id, other := range f.cycles {
		if id == c.ID() || other.Status() != domain.CycleStatusActive {
			continue
		}
		stood, err := domain.NewCycle(other.ID(), other.TenantID(), other.Name(),
			domain.CycleStatusDraft, other.StartsOn(), other.EndsOn())
		if err != nil {
			return err
		}
		f.cycles[id] = stood
	}
	f.cycles[c.ID()] = c
	return nil
}

// --- activating a school year -----------------------------------------------

func seedNamedCycle(t *testing.T, svc *usecase.CatalogService, name string) domain.Cycle {
	t.Helper()
	c, err := svc.CreateCycle(ctx(), name, domain.CycleStatusUnspecified, someDate, laterDate)
	if err != nil {
		t.Fatalf("seed cycle %q: %v", name, err)
	}
	if c.Status() != domain.CycleStatusDraft {
		t.Fatalf("a new cycle must start as a draft, got %v", c.Status())
	}
	return c
}

func TestActivateCyclePutsTheYearIntoService(t *testing.T) {
	svc, repo := newService(t)
	cycle := seedNamedCycle(t, svc, "2026-2027")

	activated, err := svc.ActivateCycle(ctx(), cycle.ID())
	if err != nil {
		t.Fatalf("ActivateCycle: %v", err)
	}
	if activated.Status() != domain.CycleStatusActive {
		t.Errorf("status = %v, want active", activated.Status())
	}
	if stored := repo.cycles[cycle.ID()]; stored.Status() != domain.CycleStatusActive {
		t.Errorf("stored status = %v, want active — the panel reads the stored one", stored.Status())
	}
}

// The invariant the whole product leans on: everything downstream asks "which
// cycle is active", and that question must have exactly one answer.
func TestActivatingAYearStandsDownThePrevious(t *testing.T) {
	svc, repo := newService(t)
	first := seedNamedCycle(t, svc, "2025-2026")
	second := seedNamedCycle(t, svc, "2026-2027")
	if _, err := svc.ActivateCycle(ctx(), first.ID()); err != nil {
		t.Fatalf("activate the first: %v", err)
	}

	if _, err := svc.ActivateCycle(ctx(), second.ID()); err != nil {
		t.Fatalf("activate the second: %v", err)
	}

	var active []string
	for _, c := range repo.cycles {
		if c.Status() == domain.CycleStatusActive {
			active = append(active, c.Name())
		}
	}
	if len(active) != 1 || active[0] != "2026-2027" {
		t.Errorf("active cycles = %v; a school has exactly one live year", active)
	}
}

func TestActivateCycleRejectsAYearThatDoesNotExist(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.ActivateCycle(ctx(), "nope"); err == nil {
		t.Error("activating a cycle that does not exist must fail")
	}
}
