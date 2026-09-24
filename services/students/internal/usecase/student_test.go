package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const tenant = "t1"

// --- fakes -----------------------------------------------------------------

type fakeRepo struct {
	byID      map[string]*domain.Student
	curps     map[string]bool // tenant+curp seen (for dup detection)
	created   []*domain.Student
	updated   []*domain.Student
	createErr error
	updateErr error
}

func newRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]*domain.Student{}, curps: map[string]bool{}}
}

func (r *fakeRepo) Create(_ context.Context, s *domain.Student) error {
	if r.createErr != nil {
		return r.createErr
	}
	key := s.TenantID() + "|" + s.CURP().String()
	if r.curps[key] {
		return glerrors.New(glerrors.KindConflict, "duplicate CURP")
	}
	r.curps[key] = true
	r.byID[s.ID()] = s
	r.created = append(r.created, s)
	return nil
}

func (r *fakeRepo) Update(_ context.Context, s *domain.Student) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.byID[s.ID()] = s
	r.updated = append(r.updated, s)
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (*domain.Student, error) {
	if s, ok := r.byID[id]; ok {
		return s, nil
	}
	return nil, glerrors.ErrNotFound
}

func (r *fakeRepo) List(_ context.Context, _ ports.ListFilter) ([]*domain.Student, string, error) {
	out := make([]*domain.Student, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, s)
	}
	return out, "", nil
}

type fakePub struct {
	enrolled []domain.StudentEnrolledEvent
	changed  []domain.AdmissionChangedEvent
}

func (p *fakePub) PublishStudentEnrolled(_ context.Context, ev domain.StudentEnrolledEvent) error {
	p.enrolled = append(p.enrolled, ev)
	return nil
}
func (p *fakePub) PublishAdmissionChanged(_ context.Context, ev domain.AdmissionChangedEvent) error {
	p.changed = append(p.changed, ev)
	return nil
}

// fakeLevels is config's level catalog: an unknown id is KindNotFound, exactly
// like the Connect client reports it.
type fakeLevels map[string]string

func (f fakeLevels) LevelByID(_ context.Context, id string) (domain.Level, error) {
	name, ok := f[id]
	if !ok {
		return domain.Level{}, glerrors.ErrNotFound
	}
	return domain.NewLevel(id, name)
}

// levelsMustNotBeAsked fails the test if config is consulted: a malformed id
// must be refused before it reaches config, where it would be a query error.
type levelsMustNotBeAsked struct{ t *testing.T }

func (l levelsMustNotBeAsked) LevelByID(_ context.Context, id string) (domain.Level, error) {
	l.t.Errorf("config was asked about %q", id)
	return domain.Level{}, glerrors.ErrNotFound
}

var catalogLevels = fakeLevels{"e1000000-0000-0000-0000-000000000001": "Primaria", "e1000000-0000-0000-0000-000000000002": "Preescolar"}

func newService(t *testing.T) (*usecase.StudentService, *fakeRepo, *fakePub) {
	t.Helper()
	repo := newRepo()
	pub := &fakePub{}
	ids := 0
	svc := usecase.NewStudentService(repo, pub, catalogLevels, tenant, func() string { ids++; return "id-" + string(rune('0'+ids)) })
	return svc, repo, pub
}

func validInput() usecase.CreateApplicantInput {
	return usecase.CreateApplicantInput{
		CURP: "HEGA050312MDFRNN08", FirstName: "Ana", LastName: "Hernández",
		LevelID: "e1000000-0000-0000-0000-000000000001", GradeGroup: "4° A", GuardianName: "Marisol",
	}
}

// --- CreateApplicant -------------------------------------------------------

func TestCreateApplicant(t *testing.T) {
	svc, repo, _ := newService(t)
	s, err := svc.CreateApplicant(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if s.Status() != domain.AdmissionPending || s.TenantID() != tenant || s.ID() == "" {
		t.Errorf("applicant wrong: %+v", s)
	}
	if len(repo.created) != 1 {
		t.Errorf("expected 1 created, got %d", len(repo.created))
	}
}

// Any level the school defined in config takes pupils — Preescolar included —
// and the name stored is the catalog's, not whatever the caller sent.
func TestCreateApplicantOnAnyCatalogLevel(t *testing.T) {
	svc, _, _ := newService(t)
	in := validInput()
	in.LevelID = "e1000000-0000-0000-0000-000000000002"
	s, err := svc.CreateApplicant(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if s.Level().ID() != "e1000000-0000-0000-0000-000000000002" || s.Level().Name() != "Preescolar" {
		t.Errorf("level = (%q, %q)", s.Level().ID(), s.Level().Name())
	}
}

// A level id that is not even shaped like one is bad input, answered here —
// not a query error in config that would surface as a 500.
func TestCreateApplicantMalformedLevelIDIsInvalid(t *testing.T) {
	svc := usecase.NewStudentService(newRepo(), &fakePub{}, levelsMustNotBeAsked{t}, tenant, func() string { return "id-1" })
	in := validInput()
	in.LevelID = "kinder"
	if _, err := svc.CreateApplicant(context.Background(), in); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// A level config does not know is bad input, not a missing student.
func TestCreateApplicantUnknownLevelIsInvalid(t *testing.T) {
	svc, repo, _ := newService(t)
	in := validInput()
	in.LevelID = "e1000000-0000-0000-0000-0000000000ff"
	if _, err := svc.CreateApplicant(context.Background(), in); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Error("nothing may be stored for an unknown level")
	}
}

func TestCreateApplicantInvalidCURP(t *testing.T) {
	svc, _, _ := newService(t)
	in := validInput()
	in.CURP = "not-a-curp"
	if _, err := svc.CreateApplicant(context.Background(), in); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCreateApplicantDuplicateIsConflict(t *testing.T) {
	svc, _, _ := newService(t)
	_, _ = svc.CreateApplicant(context.Background(), validInput())
	if _, err := svc.CreateApplicant(context.Background(), validInput()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict on duplicate CURP, got %v", err)
	}
}

// --- ChangeAdmissionStatus -------------------------------------------------

func seedApplicant(t *testing.T, svc *usecase.StudentService) *domain.Student {
	t.Helper()
	s, err := svc.CreateApplicant(context.Background(), validInput())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func TestAcceptPublishesChange(t *testing.T) {
	svc, repo, pub := newService(t)
	s := seedApplicant(t, svc)

	got, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionAccepted})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got.Status() != domain.AdmissionAccepted {
		t.Errorf("status = %v", got.Status())
	}
	if len(repo.updated) != 1 || len(pub.changed) != 1 || len(pub.enrolled) != 0 {
		t.Errorf("expected 1 update + 1 change event: updated=%d changed=%d enrolled=%d", len(repo.updated), len(pub.changed), len(pub.enrolled))
	}
}

func TestEnrollPublishesEnrolledAndChange(t *testing.T) {
	svc, _, pub := newService(t)
	s := seedApplicant(t, svc)
	_, _ = svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionAccepted})

	got, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionEnrolled, CycleID: "cycle-2026"})
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if got.Status() != domain.AdmissionEnrolled {
		t.Errorf("status = %v", got.Status())
	}
	if len(pub.enrolled) != 1 || pub.enrolled[0].CycleID != "cycle-2026" {
		t.Errorf("StudentEnrolled not published correctly: %+v", pub.enrolled)
	}
	if len(pub.changed) != 2 { // accept + enroll
		t.Errorf("expected 2 change events, got %d", len(pub.changed))
	}
}

func TestIllegalTransitionIsConflictAndPersistsNothing(t *testing.T) {
	svc, repo, pub := newService(t)
	s := seedApplicant(t, svc) // pending

	// enroll from pending is illegal (must accept first).
	_, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionEnrolled})
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict, got %v", err)
	}
	if len(repo.updated) != 0 || len(pub.changed) != 0 {
		t.Errorf("nothing should be persisted/published on illegal transition")
	}
}

func TestRejectAndWithdrawViaService(t *testing.T) {
	// reject from pending
	svc, _, pub := newService(t)
	s := seedApplicant(t, svc)
	got, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionRejected, Reason: "docs"})
	if err != nil || got.Status() != domain.AdmissionRejected {
		t.Fatalf("reject: %v status=%v", err, got.Status())
	}
	if pub.changed[0].Reason != "docs" {
		t.Errorf("reject reason not carried: %+v", pub.changed[0])
	}

	// withdraw from accepted
	svc2, _, _ := newService(t)
	s2 := seedApplicant(t, svc2)
	_, _ = svc2.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s2.ID(), Target: domain.AdmissionAccepted})
	got2, err := svc2.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s2.ID(), Target: domain.AdmissionWithdrawn, Reason: "moved"})
	if err != nil || got2.Status() != domain.AdmissionWithdrawn {
		t.Errorf("withdraw: %v status=%v", err, got2.Status())
	}
}

func TestUpdateErrorPublishesNothing(t *testing.T) {
	svc, repo, pub := newService(t)
	s := seedApplicant(t, svc)
	repo.updateErr = glerrors.New(glerrors.KindInternal, "db down")

	_, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionAccepted})
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("want KindInternal, got %v", err)
	}
	if len(pub.changed) != 0 || len(pub.enrolled) != 0 {
		t.Error("no events must be published when Update fails")
	}
}

func TestChangeStatusUnknownStudentIsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: "nope", Target: domain.AdmissionAccepted})
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

func TestChangeStatusUnknownTargetIsInvalid(t *testing.T) {
	svc, _, _ := newService(t)
	s := seedApplicant(t, svc)
	_, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: s.ID(), Target: domain.AdmissionUnspecified})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// --- Get / List ------------------------------------------------------------

func TestGetStudent(t *testing.T) {
	svc, _, _ := newService(t)
	s := seedApplicant(t, svc)
	got, err := svc.GetStudent(context.Background(), s.ID())
	if err != nil || got.ID() != s.ID() {
		t.Errorf("GetStudent: %v %v", got, err)
	}
	if _, err := svc.GetStudent(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

func TestListStudents(t *testing.T) {
	svc, _, _ := newService(t)
	seedApplicant(t, svc)
	got, _, err := svc.ListStudents(context.Background(), ports.ListFilter{})
	if err != nil || len(got) != 1 {
		t.Errorf("ListStudents: %d %v", len(got), err)
	}
}

// A list filter that is not a level id is bad input, answered before the query.
func TestListStudentsMalformedLevelFilterIsInvalid(t *testing.T) {
	svc, _, _ := newService(t)
	if _, _, err := svc.ListStudents(context.Background(), ports.ListFilter{LevelID: "kinder"}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}
