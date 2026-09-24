package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// The enrollment handler tests drive the real EnrollmentService through the
// handler with in-memory fakes for the three seams (student repo, enrollment
// repo, catalog), asserting proto↔usecase translation and the error→Connect-code
// mapping. Logic lives in usecase/domain; these tests own the wire boundary.

const (
	enrollCycle = "c0000000-0000-0000-0000-000000000001"
	enrollGroup = "40000000-0000-0000-0000-000000000001"
	otherGroup  = "40000000-0000-0000-0000-000000000002"
)

// fakeEnrollmentRepo is an in-memory EnrollmentRepository sufficient for the
// handler tests (uniqueness + transfer/group-move contract).
type fakeEnrollmentRepo struct {
	byID map[string]domain.Enrollment
	uniq map[string]bool
}

func newFakeEnrollmentRepo() *fakeEnrollmentRepo {
	return &fakeEnrollmentRepo{byID: map[string]domain.Enrollment{}, uniq: map[string]bool{}}
}

func (r *fakeEnrollmentRepo) CreateEnrollment(_ context.Context, e domain.Enrollment) error {
	key := e.TenantID() + "|" + e.StudentID() + "|" + e.CycleID()
	if r.uniq[key] {
		return glerrors.New(glerrors.KindConflict, "already enrolled this cycle")
	}
	r.uniq[key] = true
	r.byID[e.ID()] = e
	return nil
}

func (r *fakeEnrollmentRepo) GetEnrollment(_ context.Context, id string) (domain.Enrollment, error) {
	if e, ok := r.byID[id]; ok {
		return e, nil
	}
	return domain.Enrollment{}, glerrors.ErrNotFound
}

func (r *fakeEnrollmentRepo) ListEnrollments(_ context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error) {
	out := make([]domain.Enrollment, 0, len(r.byID))
	for _, e := range r.byID {
		if studentID != "" && e.StudentID() != studentID {
			continue
		}
		if cycleID != "" && e.CycleID() != cycleID {
			continue
		}
		if groupID != "" && e.GroupID() != groupID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *fakeEnrollmentRepo) TransferEnrollment(_ context.Context, enrollmentID, toGroupID string, _ domain.EnrollmentTransfer) error {
	cur, ok := r.byID[enrollmentID]
	if !ok {
		return glerrors.ErrNotFound
	}
	moved, err := domain.NewEnrollment(cur.ID(), cur.TenantID(), cur.StudentID(), cur.CycleID(),
		toGroupID, cur.TermID(), cur.Status(), cur.EnrolledAt())
	if err != nil {
		return err
	}
	r.byID[enrollmentID] = moved
	return nil
}

func (r *fakeEnrollmentRepo) SetEnrollmentStatus(_ context.Context, id string, status domain.EnrollmentStatus) error {
	cur, ok := r.byID[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	updated, err := domain.NewEnrollment(cur.ID(), cur.TenantID(), cur.StudentID(), cur.CycleID(),
		cur.GroupID(), cur.TermID(), status, cur.EnrolledAt())
	if err != nil {
		return err
	}
	r.byID[id] = updated
	return nil
}

// fakeCatalogValidator is an in-memory CatalogValidator: a missing group/term is
// glerrors.ErrNotFound (matching the real Connect client's NotFound mapping).
type fakeCatalogValidator struct {
	gradeGroups []ports.GroupSeats
	groups      map[string]ports.GroupRef
	terms       map[string]ports.TermRef
}

func (c *fakeCatalogValidator) GroupByID(_ context.Context, id string) (ports.GroupRef, error) {
	if g, ok := c.groups[id]; ok {
		return g, nil
	}
	return ports.GroupRef{}, glerrors.ErrNotFound
}

func (c *fakeCatalogValidator) TermByID(_ context.Context, id string) (ports.TermRef, error) {
	if t, ok := c.terms[id]; ok {
		return t, nil
	}
	return ports.TermRef{}, glerrors.ErrNotFound
}

// enrollmentHandler wires the handler with a pre-seeded student and two groups in
// the same cycle, returning the handler and the seeded student id.
func enrollmentHandler(t *testing.T) (*grpcadapter.StudentsHandler, string) {
	t.Helper()
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	catalog := &fakeCatalogValidator{
		groups: map[string]ports.GroupRef{
			enrollGroup: {ID: enrollGroup, CycleID: enrollCycle},
			otherGroup:  {ID: otherGroup, CycleID: enrollCycle},
		},
		terms: map[string]ports.TermRef{},
	}
	ids := 0
	newID := func() string { ids++; return "enr-1" }
	now := func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }
	enroll := usecase.NewEnrollmentService(studentRepo, newFakeEnrollmentRepo(), catalog, nil, testTenant, newID, now)
	stu := usecase.NewStudentService(studentRepo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-x" })
	return grpcadapter.NewStudentsHandler(stu, enroll, nil, usecase.NewHealthService("students")), student.ID()
}

// seedApplicant inserts an applicant directly through the student repo and
// returns it, so the enroll knot's "student exists" precondition is satisfiable.
func seedApplicant(t *testing.T, repo *fakeRepo) *domain.Student {
	t.Helper()
	c, err := domain.NewCURP(validCURP)
	if err != nil {
		t.Fatalf("curp: %v", err)
	}
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID: "stu-enroll", TenantID: testTenant, CURP: c, FirstName: "Ana", LastName: "H",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"), GradeGroup: "1° A",
	})
	if err != nil {
		t.Fatalf("applicant: %v", err)
	}
	if err := repo.Create(context.Background(), s); err != nil {
		t.Fatalf("seed applicant: %v", err)
	}
	return s
}

func TestEnrollDerivesCycleFromGroup(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	res, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID,
		GroupId:   enrollGroup,
		// No cycle_id on the wire — it is derived from the group.
	}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	got := res.Msg.GetEnrollment()
	if got.GetCycleId() != enrollCycle {
		t.Errorf("cycle_id = %q, want the group's cycle %q (derived)", got.GetCycleId(), enrollCycle)
	}
	if got.GetGroupId() != enrollGroup || got.GetStudentId() != studentID {
		t.Errorf("enrollment not mapped 1:1: %+v", got)
	}
	if got.GetStatus() != studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE {
		t.Errorf("status = %v, want ACTIVE", got.GetStatus())
	}
}

func TestEnrollUnknownStudentIsNotFound(t *testing.T) {
	h, _ := enrollmentHandler(t)
	_, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: "ghost",
		GroupId:   enrollGroup,
	}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestEnrollUnknownGroupIsInvalidArgument(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	_, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID,
		GroupId:   "no-such-group",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestEnrollDuplicateIsAlreadyExists(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	req := connect.NewRequest(&studentsv1.EnrollRequest{StudentId: studentID, GroupId: enrollGroup})
	if _, err := h.Enroll(context.Background(), req); err != nil {
		t.Fatalf("first Enroll: %v", err)
	}
	// Re-enrolling the same student in the same cycle is a conflict.
	_, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: otherGroup,
	}))
	assertCode(t, err, connect.CodeAlreadyExists)
}

func TestTransferStudentMovesGroup(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	enrolled, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: enrollGroup,
	}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	enrollmentID := enrolled.Msg.GetEnrollment().GetId()

	res, err := h.TransferStudent(context.Background(), connect.NewRequest(&studentsv1.TransferStudentRequest{
		EnrollmentId: enrollmentID,
		ToGroupId:    otherGroup,
		Reason:       "grupo balanceado",
	}))
	if err != nil {
		t.Fatalf("TransferStudent: %v", err)
	}
	if got := res.Msg.GetEnrollment().GetGroupId(); got != otherGroup {
		t.Errorf("group after transfer = %q, want %q", got, otherGroup)
	}
}

func TestTransferToSameGroupIsInvalidArgument(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	enrolled, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: enrollGroup,
	}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	_, err = h.TransferStudent(context.Background(), connect.NewRequest(&studentsv1.TransferStudentRequest{
		EnrollmentId: enrolled.Msg.GetEnrollment().GetId(),
		ToGroupId:    enrollGroup, // same group
		Reason:       "noop",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestTransferUnknownEnrollmentIsNotFound(t *testing.T) {
	h, _ := enrollmentHandler(t)
	_, err := h.TransferStudent(context.Background(), connect.NewRequest(&studentsv1.TransferStudentRequest{
		EnrollmentId: "ghost",
		ToGroupId:    otherGroup,
		Reason:       "x",
	}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestWithdrawEnrollmentSetsWithdrawn(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	enrolled, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: enrollGroup,
	}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	enrollmentID := enrolled.Msg.GetEnrollment().GetId()

	res, err := h.WithdrawEnrollment(context.Background(), connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{
		Id: enrollmentID,
	}))
	if err != nil {
		t.Fatalf("WithdrawEnrollment: %v", err)
	}
	if got := res.Msg.GetEnrollment().GetStatus(); got != studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN {
		t.Errorf("status = %v, want WITHDRAWN", got)
	}
}

func TestListEnrollmentsFiltersByStudent(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	if _, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: enrollGroup,
	})); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	res, err := h.ListEnrollments(context.Background(), connect.NewRequest(&studentsv1.ListEnrollmentsRequest{
		StudentId: studentID,
	}))
	if err != nil {
		t.Fatalf("ListEnrollments: %v", err)
	}
	if len(res.Msg.GetEnrollments()) != 1 {
		t.Errorf("got %d enrollments, want 1", len(res.Msg.GetEnrollments()))
	}
}

// --- doubles for the capacity-aware placement -------------------------------
//
// The handler's own tests do not exercise the fill order — that lives in the use
// case, against a double that models capacity. These exist so the handler
// compiles against the ports.

func (f *fakeEnrollmentRepo) CreateEnrollmentInFirstFreeGroup(
	ctx context.Context,
	e domain.Enrollment,
	groups []ports.GroupSeats,
) (ports.GroupSeats, error) {
	if len(groups) == 0 {
		return ports.GroupSeats{}, glerrors.New(glerrors.KindConflict, "no quedan lugares")
	}
	if err := f.CreateEnrollment(ctx, e); err != nil {
		return ports.GroupSeats{}, err
	}
	return groups[0], nil
}

func (f *fakeCatalogValidator) GroupsOfGrade(_ context.Context, _ string) ([]ports.GroupSeats, error) {
	return f.gradeGroups, nil
}
