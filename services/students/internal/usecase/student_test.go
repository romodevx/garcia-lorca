package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const tenant = "t1"

// --- fakes -----------------------------------------------------------------

type fakeRepo struct {
	medical map[string]domain.MedicalRecord
	photos  map[string]ports.PhotoRef
	byID    map[string]*domain.Student
	curps   map[string]bool // tenant+curp seen (for dup detection)
	created []*domain.Student
	updated []*domain.Student
	// numberYears is the matrícula year each Create was handed.
	numberYears []int
	createErr   error
	updateErr   error
	// corrections records what each UpdateDetails was asked to close;
	// correction is what the store answers.
	corrections []ports.DetailsCorrection
	correction  ports.CorrectionResult
}

func newRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]*domain.Student{}, curps: map[string]bool{}, medical: map[string]domain.MedicalRecord{}, photos: map[string]ports.PhotoRef{}}
}

func (r *fakeRepo) Create(_ context.Context, s *domain.Student, numberYear int) error {
	r.numberYears = append(r.numberYears, numberYear)
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

func (r *fakeRepo) UpdateDetails(_ context.Context, s *domain.Student, c ports.DetailsCorrection) (ports.CorrectionResult, error) {
	if r.updateErr != nil {
		return ports.CorrectionResult{}, r.updateErr
	}
	r.byID[s.ID()] = s
	r.updated = append(r.updated, s)
	r.corrections = append(r.corrections, c)
	return r.correction, nil
}

func (r *fakeRepo) PhotoRef(_ context.Context, id string) (ports.PhotoRef, error) {
	ref, ok := r.photos[id]
	if !ok {
		return ports.PhotoRef{}, glerrors.ErrNotFound
	}
	return ref, nil
}

func (r *fakeRepo) SavePhotoRef(_ context.Context, id string, ref ports.PhotoRef) error {
	r.photos[id] = ref
	return nil
}

func (r *fakeRepo) DeletePhotoRef(_ context.Context, id string) error {
	delete(r.photos, id)
	return nil
}

func (r *fakeRepo) GetMedical(_ context.Context, id string) (domain.MedicalRecord, error) {
	return r.medical[id], nil
}

func (r *fakeRepo) SaveMedical(_ context.Context, id string, rec domain.MedicalRecord) error {
	r.medical[id] = rec
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
	outbox   []ports.OutboxEvent
}

func (p *fakePub) PublishStudentEnrolled(_ context.Context, ev domain.StudentEnrolledEvent) error {
	p.enrolled = append(p.enrolled, ev)
	return nil
}
func (p *fakePub) PublishAdmissionChanged(_ context.Context, ev domain.AdmissionChangedEvent) error {
	p.changed = append(p.changed, ev)
	return nil
}
func (p *fakePub) PublishOutboxEvent(_ context.Context, event ports.OutboxEvent) error {
	p.outbox = append(p.outbox, event)
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
		CURP: "HEGA050312MDFRNN08", FirstName: "Ana", LastName: "Hernández", MaternalLastName: "Ríos",
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
	if s.LastName() != "Hernández" || s.MaternalLastName() != "Ríos" {
		t.Errorf("surnames = %q %q, want Hernández Ríos", s.LastName(), s.MaternalLastName())
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

// --- UpdateDetails -----------------------------------------------------------

var schoolToday = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

func TestUpdateDetailsCorrectsTheFile(t *testing.T) {
	svc, repo, _ := newService(t)
	svc = svc.WithClock(func() time.Time { return schoolToday })
	created, err := svc.CreateApplicant(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}

	result, err := svc.UpdateDetails(context.Background(), usecase.UpdateDetailsInput{
		ID: created.ID(), FirstName: "Ana María", LastName: "Hernández", MaternalLastName: "Cruz", CURP: "HEGA050312MDFRNN09",
		Profile: domain.ProfileInput{BirthDate: "2019-03-12", Sex: "F", Email: "Ana@Correo.mx",
			Phones: []domain.Phone{{Kind: "celular", Number: "5512345678"}}},
	})
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	updated := result.Student
	if updated.MaternalLastName() != "Cruz" {
		t.Errorf("maternal surname = %q, want Cruz", updated.MaternalLastName())
	}
	if updated.FirstName() != "Ana María" || updated.CURP().String() != "HEGA050312MDFRNN09" || updated.Profile().Email() != "ana@correo.mx" {
		t.Errorf("updated = %q %q %q", updated.FirstName(), updated.CURP(), updated.Profile().Email())
	}
	if len(repo.updated) != 1 {
		t.Errorf("want one persisted update, got %d", len(repo.updated))
	}
}

func TestUpdateDetailsRefusesABirthDateInTheFuture(t *testing.T) {
	svc, repo, _ := newService(t)
	svc = svc.WithClock(func() time.Time { return schoolToday })
	created, _ := svc.CreateApplicant(context.Background(), validInput())

	_, err := svc.UpdateDetails(context.Background(), usecase.UpdateDetailsInput{
		ID: created.ID(), FirstName: "Ana", LastName: "H", CURP: "HEGA050312MDFRNN08",
		Profile: domain.ProfileInput{BirthDate: "2026-09-25"},
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(repo.updated) != 0 {
		t.Error("nothing may be stored")
	}
}

func TestUpdateDetailsOfAnUnknownPupilIsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.UpdateDetails(context.Background(), usecase.UpdateDetailsInput{ID: "nope", FirstName: "A", LastName: "B", CURP: "HEGA050312MDFRNN08"})
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("want KindNotFound, got %v", err)
	}
}

// --- medical record ----------------------------------------------------------

func TestMedicalRecordIsSavedAndReadBack(t *testing.T) {
	svc, _, _ := newService(t)
	created, _ := svc.CreateApplicant(context.Background(), validInput())

	if _, err := svc.UpdateMedical(context.Background(), created.ID(), "O+", "Penicilina", "Asma"); err != nil {
		t.Fatalf("UpdateMedical: %v", err)
	}
	got, err := svc.GetMedical(context.Background(), created.ID())
	if err != nil || got.BloodType() != "O+" || got.Allergies() != "Penicilina" {
		t.Fatalf("GetMedical = (%+v, %v)", got, err)
	}
}

func TestMedicalRecordOfAnUnknownPupilIsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.GetMedical(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetMedical: want KindNotFound, got %v", err)
	}
	if _, err := svc.UpdateMedical(context.Background(), "nope", "O+", "", ""); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("UpdateMedical: want KindNotFound, got %v", err)
	}
}

func TestMedicalRecordRejectsABloodTypeOffTheList(t *testing.T) {
	svc, _, _ := newService(t)
	created, _ := svc.CreateApplicant(context.Background(), validInput())
	if _, err := svc.UpdateMedical(context.Background(), created.ID(), "Z+", "", ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// --- photo ---------------------------------------------------------------------

// memPhotos is an in-memory object store.
type memPhotos map[string][]byte

func (m memPhotos) Put(_ context.Context, key, _ string, data []byte) error {
	m[key] = data
	return nil
}

func (m memPhotos) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := m[key]
	if !ok {
		return nil, glerrors.ErrNotFound
	}
	return data, nil
}

func (m memPhotos) Delete(_ context.Context, key string) error {
	delete(m, key)
	return nil
}

var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 'x'}

func TestAPhotoIsStoredReadAndReplaced(t *testing.T) {
	svc, repo, _ := newService(t)
	store := memPhotos{}
	svc = svc.WithPhotos(store)
	created, _ := svc.CreateApplicant(context.Background(), validInput())

	if _, err := svc.UploadPhoto(context.Background(), created.ID(), jpegBytes); err != nil {
		t.Fatalf("UploadPhoto: %v", err)
	}
	first := repo.photos[created.ID()].Key
	if !strings.HasPrefix(first, tenant+"/students/"+created.ID()+"/") {
		t.Errorf("object key = %q: tenant/students/<id>/<random>, no personal data", first)
	}
	photo, err := svc.Photo(context.Background(), created.ID())
	if err != nil || photo.ContentType() != "image/jpeg" || len(photo.Data()) != len(jpegBytes) {
		t.Fatalf("Photo = (%q, %v)", photo.ContentType(), err)
	}

	if _, err := svc.UploadPhoto(context.Background(), created.ID(), jpegBytes); err != nil {
		t.Fatalf("second UploadPhoto: %v", err)
	}
	if _, stillThere := store[first]; stillThere {
		t.Error("the replaced photo's object must be deleted")
	}
	if len(store) != 1 {
		t.Errorf("store holds %d objects, want 1", len(store))
	}
}

func TestPhotoOfAPupilWithoutOneIsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	svc = svc.WithPhotos(memPhotos{})
	created, _ := svc.CreateApplicant(context.Background(), validInput())
	if _, err := svc.Photo(context.Background(), created.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

func TestDeletingAPhotoRemovesTheObjectAndTheReference(t *testing.T) {
	svc, repo, _ := newService(t)
	store := memPhotos{}
	svc = svc.WithPhotos(store)
	created, _ := svc.CreateApplicant(context.Background(), validInput())
	_, _ = svc.UploadPhoto(context.Background(), created.ID(), jpegBytes)

	if err := svc.DeletePhoto(context.Background(), created.ID()); err != nil {
		t.Fatalf("DeletePhoto: %v", err)
	}
	if len(store) != 0 || len(repo.photos) != 0 {
		t.Errorf("left %d objects and %d references", len(store), len(repo.photos))
	}
}

func TestAPhotoForAnUnknownPupilOrNotAnImageIsRefused(t *testing.T) {
	svc, _, _ := newService(t)
	svc = svc.WithPhotos(memPhotos{})
	if _, err := svc.UploadPhoto(context.Background(), "nope", jpegBytes); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown pupil: want KindNotFound, got %v", err)
	}
	created, _ := svc.CreateApplicant(context.Background(), validInput())
	if _, err := svc.UploadPhoto(context.Background(), created.ID(), []byte("%PDF")); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("not an image: want KindInvalid, got %v", err)
	}
}

// A deployment without a bucket has no photos: that is "there is none" (404),
// not a server failure (500) on every edit-page load.
func TestWithoutABucketThereIsNoPhoto(t *testing.T) {
	svc, _, _ := newService(t)
	created, _ := svc.CreateApplicant(context.Background(), validInput())
	if _, err := svc.Photo(context.Background(), created.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("Photo: want KindNotFound, got %v", err)
	}
	if _, err := svc.UploadPhoto(context.Background(), created.ID(), jpegBytes); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("UploadPhoto: want KindNotFound, got %v", err)
	}
	if err := svc.DeletePhoto(context.Background(), created.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("DeletePhoto: want KindNotFound, got %v", err)
	}
}

// An ids filter names students by uuid, at most one page of them.
func TestListStudentsValidatesIDs(t *testing.T) {
	svc := newStudentSvcForIDs(t)
	ok := []string{"dddddddd-0000-0000-0000-000000000001"}
	if _, _, err := svc.ListStudents(context.Background(), ports.ListFilter{IDs: ok}); err != nil {
		t.Fatalf("valid ids: %v", err)
	}
	tooMany := make([]string, 201)
	for i := range tooMany {
		tooMany[i] = ok[0]
	}
	for name, ids := range map[string][]string{"malformed": {"nope"}, "too many": tooMany} {
		if _, _, err := svc.ListStudents(context.Background(), ports.ListFilter{IDs: ids}); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%s: want KindInvalid, got %v", name, err)
		}
	}
}

func newStudentSvcForIDs(t *testing.T) *usecase.StudentService {
	t.Helper()
	return usecase.NewStudentService(newRepo(), &fakePub{}, levelsMustNotBeAsked{t}, tenant, func() string { return "id-1" })
}

// T13: the file says whether it has a photo, so the page asks for one only
// when there is one — a 404 per file opened is a console error per file.
func TestHasPhoto(t *testing.T) {
	cases := []struct {
		name   string
		bucket bool
		upload bool
		want   bool
	}{
		{"a pupil with a photo", true, true, true},
		{"a pupil without one", true, false, false},
		{"a deployment without a bucket", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newService(t)
			if tc.bucket {
				svc = svc.WithPhotos(memPhotos{})
			}
			created, _ := svc.CreateApplicant(context.Background(), validInput())
			if tc.upload {
				if _, err := svc.UploadPhoto(context.Background(), created.ID(), jpegBytes); err != nil {
					t.Fatalf("UploadPhoto: %v", err)
				}
			}
			got, err := svc.HasPhoto(context.Background(), created.ID())
			if err != nil || got != tc.want {
				t.Errorf("HasPhoto = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// T14-009: after a baja the pupil's identity, medical record and photo are
// read-only. The web hides the entry points, but the edit page has its own
// address, so the server refuses each write with one detectable refusal.
// Contacts stay writable: the remaining debt is still collected.
func TestAWithdrawnPupilsFileIsReadOnly(t *testing.T) {
	writes := []struct {
		name  string
		write func(svc *usecase.StudentService, id string) error
	}{
		{"identity", func(svc *usecase.StudentService, id string) error {
			_, err := svc.UpdateDetails(context.Background(), usecase.UpdateDetailsInput{
				ID: id, FirstName: "Ana", LastName: "Hernández", CURP: "HEGA050312MDFRNN09",
			})
			return err
		}},
		{"medical", func(svc *usecase.StudentService, id string) error {
			_, err := svc.UpdateMedical(context.Background(), id, "O+", "", "")
			return err
		}},
		{"photo upload", func(svc *usecase.StudentService, id string) error {
			_, err := svc.UploadPhoto(context.Background(), id, jpegBytes)
			return err
		}},
		{"photo removal", func(svc *usecase.StudentService, id string) error {
			return svc.DeletePhoto(context.Background(), id)
		}},
	}
	for _, w := range writes {
		t.Run(w.name, func(t *testing.T) {
			svc, repo, _ := newService(t)
			store := memPhotos{}
			svc = svc.WithClock(func() time.Time { return schoolToday }).WithPhotos(store)
			created, err := svc.CreateApplicant(context.Background(), validInput())
			if err != nil {
				t.Fatalf("CreateApplicant: %v", err)
			}
			if _, err := svc.UploadPhoto(context.Background(), created.ID(), jpegBytes); err != nil {
				t.Fatalf("UploadPhoto before the baja: %v", err)
			}
			photo := repo.photos[created.ID()]
			if _, err := svc.ChangeAdmissionStatus(context.Background(), usecase.ChangeStatusInput{ID: created.ID(), Target: domain.AdmissionWithdrawn, Reason: "cambio de escuela"}); err != nil {
				t.Fatalf("withdraw: %v", err)
			}

			err = w.write(svc, created.ID())
			if !errors.Is(err, domain.ErrWithdrawnReadOnly) {
				t.Fatalf("err = %v, want ErrWithdrawnReadOnly", err)
			}
			if glerrors.KindOf(err) != glerrors.KindConflict {
				t.Errorf("kind = %v, want KindConflict", glerrors.KindOf(err))
			}
			if len(repo.updated) != 1 || len(repo.medical) != 0 || repo.photos[created.ID()] != photo || len(store) != 1 {
				t.Errorf("a refused write persisted: updates=%d medical=%d photo=%+v objects=%d",
					len(repo.updated), len(repo.medical), repo.photos[created.ID()], len(store))
			}
		})
	}
}
