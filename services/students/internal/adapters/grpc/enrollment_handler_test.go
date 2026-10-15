package grpcadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
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
	byID    map[string]domain.Enrollment
	uniq    map[string]bool
	history map[string][]domain.EnrollmentTransfer // by student id
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

func (r *fakeEnrollmentRepo) ListTransfers(_ context.Context, studentID string) ([]domain.EnrollmentTransfer, error) {
	return r.history[studentID], nil
}

func (r *fakeEnrollmentRepo) ListTransfersIntoGroup(_ context.Context, groupID string) ([]domain.EnrollmentTransfer, error) {
	var out []domain.EnrollmentTransfer
	for _, moves := range r.history {
		for _, m := range moves {
			if m.ToGroupID() == groupID {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func (r *fakeEnrollmentRepo) TransferEnrollment(_ context.Context, cmd ports.TransferCommand) error {
	enrollmentID, toGroupID := cmd.EnrollmentID, cmd.To.ID
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
	r.byID[id] = updated.WithEndedAt(enrollmentEndedAt)
	return nil
}

func (r *fakeEnrollmentRepo) WithdrawEnrollment(_ context.Context, e domain.Enrollment) error {
	r.byID[e.ID()] = e
	return nil
}
func (r *fakeEnrollmentRepo) DrainOutbox(context.Context, int, func(context.Context, ports.OutboxEvent) error) (int, error) {
	return 0, nil
}

// enrollmentEndedAt is the status-change instant the fake stamps, as the
// repository stamps updated_at.
var enrollmentEndedAt = time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC)

// fakeCatalogValidator is an in-memory CatalogValidator: a missing group/term is
// glerrors.ErrNotFound (matching the real Connect client's NotFound mapping).
type fakeCatalogValidator struct {
	gradeGroups []ports.GroupSeats
	groups      map[string]ports.GroupRef
	terms       map[string]ports.TermRef
	deleting    bool // the pupils' level is being deleted (ADR 0021)
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
	return enrollmentHandlerWith(t, newFakeEnrollmentRepo())
}

// enrollmentHandlerWith is enrollmentHandler over a given enrollment repo.
func enrollmentHandlerWith(t *testing.T, enrollments *fakeEnrollmentRepo) (*grpcadapter.StudentsHandler, string) {
	t.Helper()
	return enrollmentHandlerOver(t, enrollments, false)
}

// enrollmentHandlerOver is enrollmentHandlerWith with the pupils' level being
// deleted or not.
func enrollmentHandlerOver(t *testing.T, enrollments *fakeEnrollmentRepo, deleting bool) (*grpcadapter.StudentsHandler, string) {
	t.Helper()
	studentRepo := newFakeRepo()
	student := seedApplicant(t, studentRepo)
	catalog := &fakeCatalogValidator{
		groups: map[string]ports.GroupRef{
			enrollGroup: {ID: enrollGroup, CycleID: enrollCycle},
			otherGroup:  {ID: otherGroup, CycleID: enrollCycle},
		},
		terms:    map[string]ports.TermRef{},
		deleting: deleting,
	}
	ids := 0
	newID := func() string { ids++; return "enr-1" }
	now := func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }
	enroll := usecase.NewEnrollmentService(studentRepo, enrollments, catalog, nil, testTenant, newID, now)
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
		Id: enrollmentID, WithdrawnOn: "2026-10-01", Reason: "cambio de escuela",
	}))
	if err != nil {
		t.Fatalf("WithdrawEnrollment: %v", err)
	}
	if got := res.Msg.GetEnrollment().GetStatus(); got != studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN {
		t.Errorf("status = %v, want WITHDRAWN", got)
	}
	if res.Msg.GetEnrollment().GetWithdrawnOn() != "2026-10-01" || res.Msg.GetEnrollment().GetWithdrawalReason() != "cambio de escuela" {
		t.Errorf("withdrawal details = %q / %q", res.Msg.GetEnrollment().GetWithdrawnOn(), res.Msg.GetEnrollment().GetWithdrawalReason())
	}
	_, err = h.WithdrawEnrollment(context.Background(), connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{Id: enrollmentID}))
	assertCode(t, err, connect.CodeFailedPrecondition)
}

func TestWithdrawEnrollmentInvalidDateIsInvalidArgument(t *testing.T) {
	h, _ := enrollmentHandler(t)
	_, err := h.WithdrawEnrollment(context.Background(), connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{Id: "id", WithdrawnOn: "01/02/2026"}))
	assertCode(t, err, connect.CodeInvalidArgument)
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

// LevelOfGrade places every grade in the level the handler tests' pupils are in.
func (f *fakeCatalogValidator) LevelOfGrade(_ context.Context, _ string) (string, error) {
	if f.deleting {
		return "", levelarchive.LevelBeingDeleted()
	}
	return "e1000000-0000-0000-0000-000000000001", nil
}

func (f *fakeCatalogValidator) GroupsOfGrade(_ context.Context, _ string) ([]ports.GroupSeats, error) {
	return f.gradeGroups, nil
}

// A withdrawn enrollment says on the wire when it stopped being active; an
// active one carries no end.
func TestEnrollmentCarriesItsEndInstant(t *testing.T) {
	h, studentID := enrollmentHandler(t)
	enrolled, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID, GroupId: enrollGroup,
	}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if enrolled.Msg.GetEnrollment().GetEndedAt() != nil {
		t.Error("an active enrollment must carry no ended_at")
	}
	res, err := h.WithdrawEnrollment(context.Background(), connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{
		Id: enrolled.Msg.GetEnrollment().GetId(), WithdrawnOn: "2026-10-03",
	}))
	if err != nil {
		t.Fatalf("WithdrawEnrollment: %v", err)
	}
	if got := res.Msg.GetEnrollment().GetEndedAt().AsTime(); !got.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ended_at = %v, want withdrawal date", got)
	}
}

// ListTransfers projects a pupil's moves onto the wire, oldest first; a
// malformed student id is InvalidArgument.
func TestListTransfersProjectsTheHistory(t *testing.T) {
	const pupil = "dddddddd-0000-0000-0000-000000000801"
	moved := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	move, err := domain.NewEnrollmentTransfer("tr-1", testTenant, "enr-1", enrollGroup, otherGroup, "balanceo", moved)
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeEnrollmentRepo()
	repo.history = map[string][]domain.EnrollmentTransfer{pupil: {move}}
	h, _ := enrollmentHandlerWith(t, repo)

	res, err := h.ListTransfers(context.Background(), connect.NewRequest(&studentsv1.ListTransfersRequest{StudentId: pupil}))
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	got := res.Msg.GetTransfers()
	if len(got) != 1 || got[0].GetFromGroupId() != enrollGroup || got[0].GetToGroupId() != otherGroup ||
		got[0].GetEnrollmentId() != "enr-1" || !got[0].GetMovedAt().AsTime().Equal(moved) {
		t.Fatalf("transfers = %+v", got)
	}

	_, err = h.ListTransfers(context.Background(), connect.NewRequest(&studentsv1.ListTransfersRequest{StudentId: "nope"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("malformed id: code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// ListTransfers by to_group_id answers every move INTO the group (one read for a
// whole class list); naming both filters, or neither, is InvalidArgument.
func TestListTransfersIntoGroup(t *testing.T) {
	const pupil = "dddddddd-0000-0000-0000-000000000801"
	moved := time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)
	in, err := domain.NewEnrollmentTransfer("tr-in", testTenant, "enr-1", enrollGroup, otherGroup, "balanceo", moved)
	if err != nil {
		t.Fatal(err)
	}
	out, err := domain.NewEnrollmentTransfer("tr-out", testTenant, "enr-2", otherGroup, enrollGroup, "regresa", moved)
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeEnrollmentRepo()
	repo.history = map[string][]domain.EnrollmentTransfer{pupil: {in, out}}
	h, _ := enrollmentHandlerWith(t, repo)

	res, err := h.ListTransfers(context.Background(), connect.NewRequest(&studentsv1.ListTransfersRequest{ToGroupId: otherGroup}))
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	if got := res.Msg.GetTransfers(); len(got) != 1 || got[0].GetId() != "tr-in" {
		t.Fatalf("transfers = %+v, want only the move into the group", got)
	}
	for name, req := range map[string]*studentsv1.ListTransfersRequest{
		"both":      {StudentId: pupil, ToGroupId: otherGroup},
		"neither":   {},
		"bad group": {ToGroupId: "nope"},
	} {
		if _, err := h.ListTransfers(context.Background(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, connect.CodeOf(err))
		}
	}
}

// A placement into a level being deleted is a 409 the gateway forwards with
// its machine code, so the screen can say so without parsing Spanish.
func TestEnrollIntoALevelBeingDeletedCarriesItsCode(t *testing.T) {
	h, studentID := enrollmentHandlerOver(t, newFakeEnrollmentRepo(), true)
	_, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{StudentId: studentID, GroupId: enrollGroup}))
	assertCode(t, err, connect.CodeAlreadyExists)
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("not a connect error: %v", err)
	}
	if got := ce.Meta().Get("X-Error-Code"); got != levelarchive.CodeLevelBeingDeleted {
		t.Errorf("X-Error-Code = %q, want %s", got, levelarchive.CodeLevelBeingDeleted)
	}
	if ce.Message() != "Este nivel se está eliminando" {
		t.Errorf("message = %q", ce.Message())
	}
}
