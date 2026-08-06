package grpcadapter_test

import (
	"context"
	"errors"
	"strings"
	"testing"

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

type fakeRepo struct {
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
	r.byID[s.ID()] = s
	return nil
}

func (r *fakeRepo) Update(_ context.Context, s *domain.Student) error {
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
	svc := usecase.NewStudentService(newFakeRepo(), pub, testTenant, newID)
	return grpcadapter.NewStudentsHandler(svc, nil, usecase.NewHealthService("students")), pub
}

func createValid(t *testing.T, h *grpcadapter.StudentsHandler) *studentsv1.Student {
	t.Helper()
	res, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:         validCURP,
		FirstName:    "Ana",
		LastName:     "Herrera",
		Level:        studentsv1.Level_LEVEL_PRIMARIA,
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
	if created.GetLevel() != studentsv1.Level_LEVEL_PRIMARIA {
		t.Errorf("level = %v", created.GetLevel())
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
		Level:     studentsv1.Level_LEVEL_PRIMARIA,
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
		Level:     studentsv1.Level_LEVEL_PRIMARIA,
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
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, usecase.NewHealthService("students"))

	res, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{
		Level:      studentsv1.Level_LEVEL_SECUNDARIA,
		Status:     studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED,
		GradeGroup: "3° B",
		PageSize:   25,
		PageToken:  "tok-1",
	}))
	if err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	got := repo.lastFilter
	if got.Level != domain.LevelSecundaria || got.Status != domain.AdmissionAccepted ||
		got.GradeGroup != "3° B" || got.PageSize != 25 || got.PageToken != "tok-1" {
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
	svc := usecase.NewStudentService(explodingRepo{newFakeRepo()}, &fakePublisher{}, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, usecase.NewHealthService("students"))

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
