package grpcadapter_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	validCURP  = "HEGA050312MDFRNN08"
	testTenant = "t1"
)

// --- fakes (only what the handler exercises) -------------------------------

// fakeLevels stands in for config's level catalog.
type fakeLevels map[string]string

func (f fakeLevels) LevelByID(_ context.Context, id string) (domain.Level, error) {
	name, ok := f[id]
	if !ok {
		return domain.Level{}, glerrors.ErrNotFound
	}
	return domain.NewLevel(id, name)
}

var testLevels = fakeLevels{"e1000000-0000-0000-0000-000000000001": "Primaria", "e1000000-0000-0000-0000-000000000003": "Secundaria"}

type fakeRepo struct {
	medical    domain.MedicalRecord
	photo      *ports.PhotoRef
	byID       map[string]*domain.Student
	byCURP     map[string]bool
	lastFilter ports.ListFilter
	nextToken  string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]*domain.Student{}, byCURP: map[string]bool{}}
}

func (r *fakeRepo) Create(_ context.Context, s *domain.Student) error {
	if r.byCURP[s.CURP().String()] {
		return glerrors.New(glerrors.KindConflict, "duplicate curp")
	}
	r.byCURP[s.CURP().String()] = true
	// As the store does: the pupil leaves with its matrícula.
	s.AssignEnrollmentNumber("2026-00001")
	r.byID[s.ID()] = s
	return nil
}

func (r *fakeRepo) Update(_ context.Context, s *domain.Student) error {
	r.byID[s.ID()] = s
	return nil
}

func (r *fakeRepo) PhotoRef(_ context.Context, _ string) (ports.PhotoRef, error) {
	if r.photo == nil {
		return ports.PhotoRef{}, glerrors.ErrNotFound
	}
	return *r.photo, nil
}

func (r *fakeRepo) SavePhotoRef(_ context.Context, _ string, ref ports.PhotoRef) error {
	r.photo = &ref
	return nil
}

func (r *fakeRepo) DeletePhotoRef(_ context.Context, _ string) error {
	r.photo = nil
	return nil
}

func (r *fakeRepo) GetMedical(_ context.Context, _ string) (domain.MedicalRecord, error) {
	return r.medical, nil
}

func (r *fakeRepo) SaveMedical(_ context.Context, _ string, rec domain.MedicalRecord) error {
	r.medical = rec
	return nil
}

func (r *fakeRepo) UpdateDetails(_ context.Context, s *domain.Student) error {
	r.byID[s.ID()] = s
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (*domain.Student, error) {
	if s, ok := r.byID[id]; ok {
		return s, nil
	}
	return nil, glerrors.ErrNotFound
}

func (r *fakeRepo) List(_ context.Context, f ports.ListFilter) ([]*domain.Student, string, error) {
	r.lastFilter = f
	out := make([]*domain.Student, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, s)
	}
	return out, r.nextToken, nil
}

type fakePublisher struct {
	enrolled []domain.StudentEnrolledEvent
	changed  []domain.AdmissionChangedEvent
}

func (p *fakePublisher) PublishStudentEnrolled(_ context.Context, ev domain.StudentEnrolledEvent) error {
	p.enrolled = append(p.enrolled, ev)
	return nil
}

func (p *fakePublisher) PublishAdmissionChanged(_ context.Context, ev domain.AdmissionChangedEvent) error {
	p.changed = append(p.changed, ev)
	return nil
}

func newHandler(t *testing.T) (*grpcadapter.StudentsHandler, *fakePublisher) {
	t.Helper()
	pub := &fakePublisher{}
	ids := 0
	newID := func() string { ids++; return "stu-1" }
	svc := usecase.NewStudentService(newFakeRepo(), pub, testLevels, testTenant, newID)
	return grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students")), pub
}

func createValid(t *testing.T, h *grpcadapter.StudentsHandler) *studentsv1.Student {
	t.Helper()
	res, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:         validCURP,
		FirstName:    "Ana",
		LastName:     "Herrera",
		LevelId:      "e1000000-0000-0000-0000-000000000001",
		GradeGroup:   "4° A",
		GuardianName: "Rosa Herrera",
	}))
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	return res.Msg.GetStudent()
}

// --- tests -----------------------------------------------------------------

func TestHealth(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.Health(context.Background(), connect.NewRequest(&studentsv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

func TestCreateApplicantThenGet(t *testing.T) {
	h, _ := newHandler(t)
	created := createValid(t, h)

	if created.GetId() != "stu-1" {
		t.Errorf("id = %q, want stu-1", created.GetId())
	}
	if created.GetCurp() != validCURP {
		t.Errorf("curp = %q", created.GetCurp())
	}
	if created.GetLevelId() != "e1000000-0000-0000-0000-000000000001" || created.GetLevelName() != "Primaria" {
		t.Errorf("level = (%q, %q)", created.GetLevelId(), created.GetLevelName())
	}
	if created.GetAdmissionStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING {
		t.Errorf("status = %v, want PENDING", created.GetAdmissionStatus())
	}

	got, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: "stu-1"}))
	if err != nil {
		t.Fatalf("GetStudent: %v", err)
	}
	if got.Msg.GetStudent().GetFirstName() != "Ana" {
		t.Errorf("first_name = %q", got.Msg.GetStudent().GetFirstName())
	}
}

func TestCreateApplicantInvalidCURPIsInvalidArgument(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:      "not-a-curp",
		FirstName: "Ana",
		LastName:  "Herrera",
		LevelId:   "e1000000-0000-0000-0000-000000000001",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestCreateApplicantDuplicateCURPIsAlreadyExists(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	_, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:      validCURP,
		FirstName: "Otro",
		LastName:  "Nombre",
		LevelId:   "e1000000-0000-0000-0000-000000000001",
	}))
	assertCode(t, err, connect.CodeAlreadyExists)
}

func TestGetUnknownIsNotFound(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: "nope"}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestChangeAdmissionAcceptThenEnrollPublishesEvents(t *testing.T) {
	h, pub := newHandler(t)
	createValid(t, h)
	ctx := context.Background()

	accepted, err := h.ChangeAdmissionStatus(ctx, connect.NewRequest(&studentsv1.ChangeAdmissionStatusRequest{
		Id:     "stu-1",
		Target: studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED,
	}))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Msg.GetStudent().GetAdmissionStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED {
		t.Errorf("status = %v, want ACCEPTED", accepted.Msg.GetStudent().GetAdmissionStatus())
	}

	enrolled, err := h.ChangeAdmissionStatus(ctx, connect.NewRequest(&studentsv1.ChangeAdmissionStatusRequest{
		Id:      "stu-1",
		Target:  studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED,
		CycleId: "2026-2027",
	}))
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if enrolled.Msg.GetStudent().GetAdmissionStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED {
		t.Errorf("status = %v, want ENROLLED", enrolled.Msg.GetStudent().GetAdmissionStatus())
	}

	if len(pub.changed) != 2 {
		t.Errorf("AdmissionChanged events = %d, want 2", len(pub.changed))
	}
	if len(pub.enrolled) != 1 {
		t.Fatalf("StudentEnrolled events = %d, want 1", len(pub.enrolled))
	}
	if pub.enrolled[0].CycleID != "2026-2027" {
		t.Errorf("enrolled cycle = %q", pub.enrolled[0].CycleID)
	}
}

func TestIllegalTransitionIsFailedPrecondition(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	// enroll straight from pending (skips accepted) — illegal per the state machine.
	_, err := h.ChangeAdmissionStatus(context.Background(), connect.NewRequest(&studentsv1.ChangeAdmissionStatusRequest{
		Id:      "stu-1",
		Target:  studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED,
		CycleId: "2026-2027",
	}))
	assertCode(t, err, connect.CodeFailedPrecondition)
}

func TestListStudents(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	res, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{}))
	if err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	if len(res.Msg.GetStudents()) != 1 {
		t.Errorf("students = %d, want 1", len(res.Msg.GetStudents()))
	}
}

func TestListStudentsMapsFilterAndToken(t *testing.T) {
	repo := newFakeRepo()
	repo.nextToken = "next-42"
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))

	res, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{
		LevelId:    "e1000000-0000-0000-0000-000000000003",
		Status:     studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED,
		GradeGroup: "3° B",
		PageSize:   25,
		PageToken:  "tok-1",
		Ids:        []string{"dddddddd-0000-0000-0000-000000000001"},
	}))
	if err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	got := repo.lastFilter
	if got.LevelID != "e1000000-0000-0000-0000-000000000003" || got.Status != domain.AdmissionAccepted ||
		got.GradeGroup != "3° B" || got.PageSize != 25 || got.PageToken != "tok-1" ||
		len(got.IDs) != 1 || got.IDs[0] != "dddddddd-0000-0000-0000-000000000001" {
		t.Errorf("filter not mapped 1:1 from the wire: %+v", got)
	}
	if res.Msg.GetNextPageToken() != "next-42" {
		t.Errorf("next_page_token = %q, want next-42", res.Msg.GetNextPageToken())
	}
}

func TestChangeAdmissionUnknownIDIsNotFound(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ChangeAdmissionStatus(context.Background(), connect.NewRequest(&studentsv1.ChangeAdmissionStatusRequest{
		Id:     "nope",
		Target: studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED,
	}))
	assertCode(t, err, connect.CodeNotFound)
}

// explodingRepo returns a raw internal error carrying a sensitive detail, to
// prove connectErr's default branch never leaks it to the client.
type explodingRepo struct{ *fakeRepo }

func (explodingRepo) FindByID(context.Context, string) (*domain.Student, error) {
	return nil, errors.New("db exploded: secret-host connection string leaked")
}

func TestInternalErrorIsGenericAndDoesNotLeak(t *testing.T) {
	svc := usecase.NewStudentService(explodingRepo{newFakeRepo()}, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))

	_, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: "stu-1"}))
	assertCode(t, err, connect.CodeInternal)
	if strings.Contains(err.Error(), "secret-host") || strings.Contains(err.Error(), "db exploded") {
		t.Errorf("internal detail leaked to client: %v", err)
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Errorf("expected generic message, got %v", err)
	}
}

func assertCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v (err=%v)", got, want, err)
	}
}

// UpdateStudent carries the whole editable file in and out.
func TestUpdateStudentRoundTripsTheProfile(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	res, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana María", LastName: "Herrera", Curp: validCURP,
		Profile: &studentsv1.StudentProfile{
			BirthDate: "2019-03-12", Sex: "F", Email: "Ana@Correo.mx",
			Phones:  []*studentsv1.StudentPhone{{Kind: "celular", Number: "55-1234-5678"}},
			Address: &studentsv1.StudentAddress{Street: "Av. Juárez", PostalCode: "44100", State: "Jalisco"},
		},
	}))
	if err != nil {
		t.Fatalf("UpdateStudent: %v", err)
	}
	p := res.Msg.GetStudent().GetProfile()
	if res.Msg.GetStudent().GetFirstName() != "Ana María" || p.GetEmail() != "ana@correo.mx" || p.GetBirthDate() != "2019-03-12" {
		t.Errorf("student = %+v", res.Msg.GetStudent())
	}
	if len(p.GetPhones()) != 1 || p.GetPhones()[0].GetNumber() != "5512345678" || p.GetAddress().GetState() != "Jalisco" {
		t.Errorf("profile = %+v", p)
	}
}

func TestUpdateStudentMalformedProfileIsInvalidArgument(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	_, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana", LastName: "Herrera", Curp: validCURP,
		Profile: &studentsv1.StudentProfile{Address: &studentsv1.StudentAddress{PostalCode: "4410"}},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestStudentMedicalRoundTrips(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	if _, err := h.UpdateStudentMedical(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentMedicalRequest{
		StudentId: "stu-1", Medical: &studentsv1.StudentMedical{BloodType: "O+", Allergies: "Penicilina"},
	})); err != nil {
		t.Fatalf("UpdateStudentMedical: %v", err)
	}
	res, err := h.GetStudentMedical(context.Background(), connect.NewRequest(&studentsv1.GetStudentMedicalRequest{StudentId: "stu-1"}))
	if err != nil {
		t.Fatalf("GetStudentMedical: %v", err)
	}
	if m := res.Msg.GetMedical(); m.GetBloodType() != "O+" || m.GetAllergies() != "Penicilina" {
		t.Errorf("medical = %+v", m)
	}
}

func TestStudentMedicalOffTheListIsInvalidArgument(t *testing.T) {
	h, _ := newHandler(t)
	createValid(t, h)
	_, err := h.UpdateStudentMedical(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentMedicalRequest{
		StudentId: "stu-1", Medical: &studentsv1.StudentMedical{BloodType: "Z+"},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

// memPhotoStore is an in-memory object store.
type memPhotoStore map[string][]byte

func (m memPhotoStore) Put(_ context.Context, key, _ string, data []byte) error {
	m[key] = data
	return nil
}
func (m memPhotoStore) Get(_ context.Context, key string) ([]byte, error) {
	d, ok := m[key]
	if !ok {
		return nil, glerrors.ErrNotFound
	}
	return d, nil
}
func (m memPhotoStore) Delete(_ context.Context, key string) error { delete(m, key); return nil }

func TestStudentPhotoRoundTrips(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" }).
		WithPhotos(memPhotoStore{})
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))
	createValid(t, h)

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 1}
	up, err := h.UploadStudentPhoto(context.Background(), connect.NewRequest(&studentsv1.UploadStudentPhotoRequest{StudentId: "stu-1", Data: png}))
	if err != nil || up.Msg.GetContentType() != "image/png" {
		t.Fatalf("Upload = (%v, %v)", up, err)
	}
	got, err := h.GetStudentPhoto(context.Background(), connect.NewRequest(&studentsv1.GetStudentPhotoRequest{StudentId: "stu-1"}))
	if err != nil || got.Msg.GetContentType() != "image/png" || len(got.Msg.GetData()) != len(png) {
		t.Fatalf("Get = (%v, %v)", got, err)
	}
	if _, err := h.DeleteStudentPhoto(context.Background(), connect.NewRequest(&studentsv1.DeleteStudentPhotoRequest{StudentId: "stu-1"})); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = h.GetStudentPhoto(context.Background(), connect.NewRequest(&studentsv1.GetStudentPhotoRequest{StudentId: "stu-1"}))
	assertCode(t, err, connect.CodeNotFound)

	_, err = h.UploadStudentPhoto(context.Background(), connect.NewRequest(&studentsv1.UploadStudentPhotoRequest{StudentId: "stu-1", Data: []byte("%PDF")}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

// Task 13: the student on the wire carries its matrícula.
func TestAStudentCarriesItsMatricula(t *testing.T) {
	h, _ := newHandler(t)
	if got := createValid(t, h).GetEnrollmentNumber(); got != "2026-00001" {
		t.Errorf("enrollment number = %q, want the one the store gave", got)
	}
}

// memCards is an in-memory card store for the handler tests.
type memCards struct{ byToken map[string]domain.CardToken }

func (m *memCards) ActiveCard(_ context.Context, studentID string) (domain.CardToken, bool, error) {
	for _, c := range m.byToken {
		if c.StudentID() == studentID && c.Active() {
			return c, true, nil
		}
	}
	return domain.CardToken{}, false, nil
}

func (m *memCards) SaveCard(_ context.Context, c domain.CardToken) error {
	m.byToken[c.Token()] = c
	return nil
}

func (m *memCards) RevokeCards(_ context.Context, studentID string, at time.Time) error {
	for k, c := range m.byToken {
		if c.StudentID() == studentID && c.Active() {
			m.byToken[k] = c.Revoke(at)
		}
	}
	return nil
}

func (m *memCards) EverIssued(_ context.Context, studentID string) (bool, error) {
	for _, c := range m.byToken {
		if c.StudentID() == studentID {
			return true, nil
		}
	}
	return false, nil
}

func (m *memCards) FindCard(_ context.Context, token string) (domain.CardToken, error) {
	c, ok := m.byToken[token]
	if !ok {
		return domain.CardToken{}, glerrors.ErrNotFound
	}
	return c, nil
}

// Task 13: the card's code over the wire — issued once, verified with whose it
// is, revoked; an unknown code is NotFound, a non-code InvalidArgument.
func TestCardRPCs(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	n := byte(0)
	cards := usecase.NewCardService(&memCards{byToken: map[string]domain.CardToken{}}, repo, testTenant,
		func() string { n++; return "ct-" + string(rune('0'+n)) },
		func() ([16]byte, error) { return [16]byte{n}, nil },
		func() time.Time { return time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC) })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students")).WithCards(cards)
	pupil := createValid(t, h)
	ctx := context.Background()

	code, err := h.GetCardCode(ctx, connect.NewRequest(&studentsv1.GetCardCodeRequest{StudentId: pupil.GetId()}))
	if err != nil || !strings.HasPrefix(code.Msg.GetCode(), "GLCARD:") {
		t.Fatalf("GetCardCode = %q (%v)", code.Msg.GetCode(), err)
	}
	got, err := h.VerifyCard(ctx, connect.NewRequest(&studentsv1.VerifyCardRequest{Code: code.Msg.GetCode()}))
	if err != nil || got.Msg.GetStudent().GetId() != pupil.GetId() {
		t.Fatalf("VerifyCard = %+v (%v)", got.Msg, err)
	}
	if got.Msg.GetValid() {
		t.Error("a pupil not yet enrolled verifies as valid")
	}
	if got.Msg.GetRevoked() {
		t.Error("an active card of a pupil not yet enrolled reads as revoked")
	}
	if _, err := h.RevokeCard(ctx, connect.NewRequest(&studentsv1.RevokeCardRequest{StudentId: pupil.GetId()})); err != nil {
		t.Fatalf("RevokeCard: %v", err)
	}
	old, err := h.VerifyCard(ctx, connect.NewRequest(&studentsv1.VerifyCardRequest{Code: code.Msg.GetCode()}))
	if err != nil || old.Msg.GetValid() || !old.Msg.GetRevoked() {
		t.Fatalf("VerifyCard(revoked) = %+v (%v), want not valid and revoked", old.Msg, err)
	}
	// Revoked: suspended, no code, until the office reissues it.
	suspended, err := h.GetCardCode(ctx, connect.NewRequest(&studentsv1.GetCardCodeRequest{StudentId: pupil.GetId()}))
	if err != nil || !suspended.Msg.GetSuspended() || suspended.Msg.GetCode() != "" {
		t.Fatalf("after revoking = %+v (%v), want suspended", suspended.Msg, err)
	}
	if _, err := h.ReissueCard(ctx, connect.NewRequest(&studentsv1.ReissueCardRequest{StudentId: pupil.GetId()})); err != nil {
		t.Fatalf("ReissueCard: %v", err)
	}
	if _, err := h.ReissueCard(ctx, connect.NewRequest(&studentsv1.ReissueCardRequest{StudentId: pupil.GetId()})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("reissuing a valid card: %v, want AlreadyExists", connect.CodeOf(err))
	}
	if _, err := h.VerifyCard(ctx, connect.NewRequest(&studentsv1.VerifyCardRequest{Code: "GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("unknown code: %v (%v), want NotFound", connect.CodeOf(err), err)
	}
	if _, err := h.VerifyCard(ctx, connect.NewRequest(&studentsv1.VerifyCardRequest{Code: "hola"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("not a code: %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// T3: the maternal surname travels in both directions — captured on create,
// corrected on update, returned on every read.
func TestTheMaternalSurnameRoundTrips(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp: validCURP, FirstName: "Ana", LastName: "Herrera", MaternalLastName: "Ríos",
		LevelId: "e1000000-0000-0000-0000-000000000001",
	}))
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if got := res.Msg.GetStudent().GetMaternalLastName(); got != "Ríos" {
		t.Errorf("created maternal_last_name = %q, want Ríos", got)
	}
	up, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana", LastName: "Herrera", MaternalLastName: "Cruz", Curp: validCURP,
		Profile: &studentsv1.StudentProfile{},
	}))
	if err != nil {
		t.Fatalf("UpdateStudent: %v", err)
	}
	if got := up.Msg.GetStudent().GetMaternalLastName(); got != "Cruz" {
		t.Errorf("updated maternal_last_name = %q, want Cruz", got)
	}
}
