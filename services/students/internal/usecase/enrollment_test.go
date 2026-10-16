package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// Short aliases for the students-owned catalog view types used across the fakes.
type (
	groupRef = ports.GroupRef
	termRef  = ports.TermRef
)

// --- fakes -----------------------------------------------------------------

// fakeEnrollRepo is an in-memory EnrollmentRepository. It enforces the
// (student, cycle) uniqueness the real table's UNIQUE constraint owns, and
// records transfers so tests can assert the tx contract (history + group update).
type fakeEnrollRepo struct {
	byID         map[string]domain.Enrollment
	uniq         map[string]bool // tenant|student|cycle
	transfers    []domain.EnrollmentTransfer
	lastTransfer ports.TransferCommand
	createErr    error
	createSeen   []domain.Enrollment
	history      map[string][]domain.EnrollmentTransfer // by student id
	outbox       []domain.Enrollment
	drainLimit   int
	drainTimeout time.Duration
}

// CreateEnrollmentInFirstFreeGroup models what the transaction does: it counts
// what is already in each group and takes the first with room. Modelling the
// capacity here rather than answering "yes" to everything is what lets the
// overflow tests fail in a unit test instead of only against Postgres.
func (r *fakeEnrollRepo) CreateEnrollmentInFirstFreeGroup(
	ctx context.Context,
	e domain.Enrollment,
	groups []ports.GroupSeats,
) (ports.GroupSeats, error) {
	for _, g := range groups {
		taken := 0
		for _, existing := range r.byID {
			if existing.GroupID() == g.ID && existing.Status() == domain.EnrollmentStatusActive {
				taken++
			}
		}
		if taken >= g.Capacity {
			continue
		}
		placed, err := domain.NewEnrollment(e.ID(), e.TenantID(), e.StudentID(), g.CycleID, g.ID,
			e.TermID(), domain.EnrollmentStatusActive, e.EnrolledAt())
		if err != nil {
			return ports.GroupSeats{}, err
		}
		if err := r.CreateEnrollment(ctx, placed); err != nil {
			return ports.GroupSeats{}, err
		}
		return g, nil
	}
	return ports.GroupSeats{}, glerrors.New(glerrors.KindConflict, "no quedan lugares")
}

func (r *fakeEnrollRepo) ListTransfers(_ context.Context, studentID string) ([]domain.EnrollmentTransfer, error) {
	return r.history[studentID], nil
}

func (r *fakeEnrollRepo) ListTransfersIntoGroup(_ context.Context, groupID string) ([]domain.EnrollmentTransfer, error) {
	return r.history[groupID], nil
}

func newEnrollRepo() *fakeEnrollRepo {
	return &fakeEnrollRepo{byID: map[string]domain.Enrollment{}, uniq: map[string]bool{}}
}

func (r *fakeEnrollRepo) CreateEnrollment(_ context.Context, e domain.Enrollment) error {
	if r.createErr != nil {
		return r.createErr
	}
	key := e.TenantID() + "|" + e.StudentID() + "|" + e.CycleID()
	if r.uniq[key] {
		return glerrors.New(glerrors.KindConflict, "already enrolled this cycle")
	}
	r.uniq[key] = true
	r.byID[e.ID()] = e
	r.createSeen = append(r.createSeen, e)
	return nil
}

func (r *fakeEnrollRepo) GetEnrollment(_ context.Context, id string) (domain.Enrollment, error) {
	if e, ok := r.byID[id]; ok {
		return e, nil
	}
	return domain.Enrollment{}, glerrors.ErrNotFound
}

func (r *fakeEnrollRepo) ListEnrollments(_ context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error) {
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

func (r *fakeEnrollRepo) TransferEnrollment(_ context.Context, cmd ports.TransferCommand) error {
	cur, ok := r.byID[cmd.EnrollmentID]
	if !ok {
		return glerrors.ErrNotFound
	}
	// Mirror the transaction: a group at capacity refuses the move.
	taken := 0
	for _, e := range r.byID {
		if e.GroupID() == cmd.To.ID && e.Status() == domain.EnrollmentStatusActive {
			taken++
		}
	}
	if cmd.To.Capacity <= 0 || taken >= cmd.To.Capacity {
		return glerrors.New(glerrors.KindConflict, "full")
	}
	// Record history, move the group AND name it on the file — one tx.
	r.transfers = append(r.transfers, cmd.Transfer)
	r.lastTransfer = cmd
	enrollmentID, toGroupID := cmd.EnrollmentID, cmd.To.ID
	moved, err := domain.NewEnrollment(cur.ID(), cur.TenantID(), cur.StudentID(), cur.CycleID(),
		toGroupID, cur.TermID(), cur.Status(), cur.EnrolledAt())
	if err != nil {
		return err
	}
	r.byID[enrollmentID] = moved
	return nil
}

func (r *fakeEnrollRepo) SetEnrollmentStatus(_ context.Context, id string, status domain.EnrollmentStatus) error {
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

func (r *fakeEnrollRepo) WithdrawEnrollment(_ context.Context, e domain.Enrollment) error {
	if _, ok := r.byID[e.ID()]; !ok {
		return glerrors.ErrNotFound
	}
	r.byID[e.ID()] = e
	r.outbox = append(r.outbox, e)
	return nil
}

func (r *fakeEnrollRepo) DrainOutbox(ctx context.Context, limit int, publish func(context.Context, ports.OutboxEvent) error) (int, error) {
	r.drainLimit = limit
	if deadline, ok := ctx.Deadline(); ok {
		r.drainTimeout = time.Until(deadline)
	}
	published := 0
	for range r.outbox {
		if err := publish(ctx, ports.OutboxEvent{TenantID: tenant, Type: "StudentWithdrawn", Payload: []byte("event")}); err != nil {
			return published, err
		}
		published++
	}
	r.outbox = nil
	return published, nil
}

// fakeCatalog is an in-memory CatalogValidator. A missing group/term returns
// glerrors.ErrNotFound, matching the real Connect client's NotFound mapping.
type fakeCatalog struct {
	groups      map[string]groupRef
	terms       map[string]termRef
	gradeGroups map[string][]ports.GroupSeats
	// gradeLevels maps a grade to the config level it belongs to.
	gradeLevels map[string]string
	// deleting marks config levels whose deletion is in flight (ADR 0021).
	deleting map[string]bool
}

// Config level ids the fixtures use: seedStudent places pupils in primaria.
const (
	primariaLevel = "e1000000-0000-0000-0000-000000000001"
	kinderLevel   = "e1000000-0000-0000-0000-000000000009"
)

func newCatalog() *fakeCatalog {
	return &fakeCatalog{
		groups:      map[string]groupRef{},
		terms:       map[string]termRef{},
		gradeGroups: map[string][]ports.GroupSeats{},
		gradeLevels: map[string]string{"gl-1": primariaLevel, "": primariaLevel, "gl-k": kinderLevel},
	}
}

func (c *fakeCatalog) LevelOfGrade(_ context.Context, gradeLevelID string) (string, error) {
	if l, ok := c.gradeLevels[gradeLevelID]; ok {
		if c.deleting[l] {
			return "", levelarchive.LevelBeingDeleted()
		}
		return l, nil
	}
	return "", glerrors.ErrNotFound
}

func (c *fakeCatalog) GroupByID(_ context.Context, id string) (groupRef, error) {
	if g, ok := c.groups[id]; ok {
		return g, nil
	}
	return groupRef{}, glerrors.ErrNotFound
}

// GroupsOfGrade answers with the school's groups for a grade, in fill order.
func (c *fakeCatalog) GroupsOfGrade(_ context.Context, gradeLevelID string) ([]ports.GroupSeats, error) {
	return c.gradeGroups[gradeLevelID], nil
}

func (c *fakeCatalog) TermByID(_ context.Context, id string) (termRef, error) {
	if tm, ok := c.terms[id]; ok {
		return tm, nil
	}
	return termRef{}, glerrors.ErrNotFound
}

// --- helpers ---------------------------------------------------------------

func seedStudent(t *testing.T, repo *fakeRepo, id string) {
	t.Helper()
	s := domain.Rehydrate(domain.NewApplicantParams{
		ID: id, TenantID: tenant, FirstName: "Ana", LastName: "Hernández",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionAccepted)
	repo.byID[id] = s
}

func newEnrollService(t *testing.T) (*usecase.EnrollmentService, *fakeRepo, *fakeEnrollRepo, *fakeCatalog, *fakePub) {
	t.Helper()
	students := newRepo()
	enrolls := newEnrollRepo()
	catalog := newCatalog()
	pub := &fakePub{}
	ids := 0
	fixed := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	svc := usecase.NewEnrollmentService(students, enrolls, catalog, pub, tenant,
		func() string { ids++; return "gen-" + string(rune('0'+ids)) },
		func() time.Time { return fixed })
	return svc, students, enrolls, catalog, pub
}

// --- Enroll ----------------------------------------------------------------

func TestEnrollHappyDerivesCycleFromGroup(t *testing.T) {
	svc, students, enrolls, catalog, pub := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99", GradeLevelID: "gl-1"}

	e, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "")
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if e.CycleID() != "cyc-99" {
		t.Errorf("cycle must be DERIVED from the group: got %q, want cyc-99", e.CycleID())
	}
	if e.Status() != domain.EnrollmentStatusActive {
		t.Errorf("status = %v, want active", e.Status())
	}
	if len(enrolls.createSeen) != 1 || enrolls.createSeen[0].CycleID() != "cyc-99" {
		t.Errorf("persisted enrollment cycle mismatch: %+v", enrolls.createSeen)
	}
	if len(pub.enrolled) != 1 || pub.enrolled[0].StudentID != "stu-1" {
		t.Errorf("StudentEnrolled must be published: %+v", pub.enrolled)
	}
}

func TestEnrollWithValidTerm(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	catalog.terms["term-1"] = termRef{ID: "term-1", CycleID: "cyc-99"}

	e, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "term-1")
	if err != nil {
		t.Fatalf("Enroll with term: %v", err)
	}
	if e.TermID() != "term-1" {
		t.Errorf("term not persisted: %q", e.TermID())
	}
}

func TestEnrollMissingStudentIsNotFound(t *testing.T) {
	svc, _, _, catalog, _ := newEnrollService(t)
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	_, err := svc.Enroll(context.Background(), "ghost", "grp-1", "")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

func TestEnrollMissingGroupIsInvalid(t *testing.T) {
	svc, students, enrolls, _, pub := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	_, err := svc.Enroll(context.Background(), "stu-1", "ghost-grp", "")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
	if len(enrolls.createSeen) != 0 || len(pub.enrolled) != 0 {
		t.Error("nothing must be persisted/published on a missing group")
	}
}

func TestEnrollTermNotInCycleIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	catalog.terms["term-x"] = termRef{ID: "term-x", CycleID: "cyc-other"}
	_, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "term-x")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestEnrollMissingTermIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	_, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "ghost-term")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestEnrollDuplicateIsConflict(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	if _, err := svc.Enroll(context.Background(), "stu-1", "grp-1", ""); err != nil {
		t.Fatalf("first enroll: %v", err)
	}
	_, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict on duplicate (student,cycle), got %v", err)
	}
}

// --- Transfer --------------------------------------------------------------

func enrolledStudent(t *testing.T, svc *usecase.EnrollmentService, students *fakeRepo, catalog *fakeCatalog) domain.Enrollment {
	t.Helper()
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99"}
	e, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "")
	if err != nil {
		t.Fatalf("seed enroll: %v", err)
	}
	return e
}

func TestTransferHappyWritesHistoryAndMovesGroup(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", Capacity: 30}

	updated, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "seat rebalancing")
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if updated.GroupID() != "grp-2" {
		t.Errorf("group must move to grp-2, got %q", updated.GroupID())
	}
	if len(enrolls.transfers) != 1 {
		t.Fatalf("expected 1 transfer history row, got %d", len(enrolls.transfers))
	}
	tr := enrolls.transfers[0]
	if tr.FromGroupID() != "grp-1" || tr.ToGroupID() != "grp-2" || tr.Reason() != "seat rebalancing" {
		t.Errorf("transfer row wrong: %+v", tr)
	}
	if enrolls.byID[e.ID()].GroupID() != "grp-2" {
		t.Error("stored enrollment group not updated")
	}
}

// The pupil's file follows a transfer: after moving to 1° B it says 1° B, the
// same guarantee both enrolment paths already give.
func TestTransferRecordsTheNewGroupOnTheFile(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", Name: "1° B", GradeLevelID: "gl-1", Capacity: 30}

	if _, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "cambio de turno"); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	// The repository writes the file in the SAME transaction as the move.
	if cmd := enrolls.lastTransfer; cmd.EnrollmentID != e.ID() || cmd.To.Name != "1° B" {
		t.Errorf("transfer command = enrollment %q group name %q", cmd.EnrollmentID, cmd.To.Name)
	}
}

// A transfer is a placement: a full group refuses it, as the by-grade path does.
func TestTransferIntoAFullGroupIsConflict(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", GradeLevelID: "gl-1", Capacity: 1}
	occupant, _ := domain.NewEnrollment("enr-x", tenant, "stu-x", "cyc-99", "grp-2", "", domain.EnrollmentStatusActive, time.Now())
	enrolls.byID["enr-x"] = occupant

	if _, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "x"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
}

// A withdrawn or rejected file is not moved around, as neither enrol path does.
func TestTransferRefusesATerminalFile(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", GradeLevelID: "gl-1", Capacity: 30}
	withdrawn := students.byID[e.StudentID()]
	if _, err := withdrawn.Withdraw("se fue"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if _, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "x"); err == nil {
		t.Fatal("a withdrawn pupil must not be transferred")
	}
}

// A group belongs to one level. Placing a primaria pupil in a kínder group —
// easy when four groups are all called "1° A" — is refused, by both paths and
// by a transfer.
func TestEnrollIntoAGroupOfAnotherLevelIsInvalid(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.groups["grp-k"] = groupRef{ID: "grp-k", CycleID: "cyc-99", Name: "1° A", GradeLevelID: "gl-k"}

	_, err := svc.Enroll(context.Background(), "stu-1", "grp-k", "")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(enrolls.createSeen) != 0 {
		t.Error("nothing may be enrolled in another level's group")
	}
}

func TestEnrollIntoAGradeOfAnotherLevelIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.gradeGroups["gl-k"] = []ports.GroupSeats{{ID: "grp-k", Name: "1° A", CycleID: "cyc-99", Capacity: 30}}

	if _, err := svc.EnrollIntoGrade(context.Background(), "stu-1", "gl-k"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
}

func TestTransferToAGroupOfAnotherLevelIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-k"] = groupRef{ID: "grp-k", CycleID: "cyc-99", Name: "1° A", GradeLevelID: "gl-k", Capacity: 30}

	if _, err := svc.Transfer(context.Background(), e.ID(), "grp-k", "x"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
}

func TestTransferNonActiveIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", Capacity: 30}
	if _, err := svc.Withdraw(context.Background(), e.ID(), e.EnrolledAt(), "x"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	_, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "why")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid transferring a non-active enrollment, got %v", err)
	}
}

func TestTransferCrossCycleIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-other"}
	_, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "why")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid on a cross-cycle transfer, got %v", err)
	}
}

func TestTransferSameGroupIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	_, err := svc.Transfer(context.Background(), e.ID(), "grp-1", "why")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid transferring to the same group, got %v", err)
	}
}

func TestTransferMissingToGroupIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	_, err := svc.Transfer(context.Background(), e.ID(), "ghost-grp", "why")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid on a missing destination group, got %v", err)
	}
}

// --- Withdraw --------------------------------------------------------------

func TestWithdrawActiveThenAlreadyWithdrawn(t *testing.T) {
	svc, students, enrollments, catalog, pub := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)

	withdrawnOn := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	updated, err := svc.Withdraw(context.Background(), e.ID(), withdrawnOn, "  cambio de escuela  ")
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if updated.Status() != domain.EnrollmentStatusWithdrawn {
		t.Errorf("status = %v, want withdrawn", updated.Status())
	}
	if !updated.WithdrawnOn().Equal(withdrawnOn) || updated.WithdrawalReason() != "cambio de escuela" {
		t.Errorf("withdrawal = %v / %q", updated.WithdrawnOn(), updated.WithdrawalReason())
	}
	if len(students.byID) == 0 || len(enrollments.outbox) != 0 {
		t.Fatal("withdrawal outbox event was not drained after commit")
	}
	if len(pub.outbox) != 1 || pub.outbox[0].Type != "StudentWithdrawn" || pub.outbox[0].TenantID != tenant {
		t.Fatalf("published events = %+v", pub.outbox)
	}
	if enrollments.drainLimit != 5 || enrollments.drainTimeout <= 0 || enrollments.drainTimeout > 2*time.Second {
		t.Errorf("synchronous drain = limit %d, timeout %s; want limit 5 and a deadline <= 2s", enrollments.drainLimit, enrollments.drainTimeout)
	}

	_, err = svc.Withdraw(context.Background(), e.ID(), withdrawnOn, "again")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindInvalid withdrawing an already-withdrawn enrollment, got %v", err)
	}
}

// --- Get / List ------------------------------------------------------------

func TestEnrollmentGetAndList(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)

	got, err := svc.GetEnrollment(context.Background(), e.ID())
	if err != nil || got.ID() != e.ID() {
		t.Errorf("GetEnrollment: %v %v", got, err)
	}
	list, err := svc.ListEnrollments(context.Background(), "stu-1", "", "")
	if err != nil || len(list) != 1 {
		t.Errorf("ListEnrollments: %d %v", len(list), err)
	}
}

// --- the file and the placement must agree ---------------------------------
//
// Two paths could move a pupil: ChangeAdmissionStatus (the file) and Enroll
// (the placement in a group). Enroll persisted the placement and published the
// event but never touched the file, so a school could — and in the demo data
// did — end up with four hundred pupils actively enrolled in groups whose
// records all still read "Trámite". The roster's own tabs mean nothing when that
// happens, and neither does any report built on admission status.

func seedPendingApplicant(t *testing.T, repo *fakeRepo, id string) {
	t.Helper()
	s := domain.Rehydrate(domain.NewApplicantParams{
		ID: id, TenantID: tenant, FirstName: "Ana", LastName: "Hernández",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionPending)
	repo.byID[id] = s
}

func TestEnrollRecordsThePlacementWithoutCallingThemEnrolled(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedPendingApplicant(t, students, "stu-1")
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99", Name: "3° B", GradeLevelID: "gl-1"}

	if _, err := svc.Enroll(context.Background(), "stu-1", "grp-1", ""); err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	if len(students.updated) == 0 {
		t.Fatal("the file was never persisted — the roster would still say Trámite")
	}
	saved := students.updated[len(students.updated)-1]
	// A pupil placed in a group is NOT yet "inscrito": the school counts them
	// enrolled when the inscription is PAID, and the ledger is what says so.
	// Flipping the file here would have the panel showing 400 enrolled pupils in
	// a school that had collected from 260 of them.
	if saved.Status() != domain.AdmissionPending {
		t.Errorf("persisted admission status = %v, want pending — enrolled comes with payment", saved.Status())
	}
	// The record still says where they sit. Both enrolment paths write it.
	if saved.GradeGroup() != "3° B" {
		t.Errorf("persisted grade group = %q, want 3° B", saved.GradeGroup())
	}
}

// Paying the inscription is what makes a pupil "inscrito" — the school's rule in
// its own words. The walk is pending → accepted → enrolled rather than a jump,
// so an audit can tell a legal path from a bypassed state machine.
func TestPayingTheInscriptionIsWhatEnrolsThem(t *testing.T) {
	svc, students, _, _, pub := newEnrollService(t)
	seedPendingApplicant(t, students, "stu-2")

	if err := svc.MarkInscriptionPaid(context.Background(), "stu-2"); err != nil {
		t.Fatalf("MarkInscriptionPaid: %v", err)
	}

	if len(students.updated) == 0 {
		t.Fatal("the file was never persisted")
	}
	if got := students.updated[len(students.updated)-1].Status(); got != domain.AdmissionEnrolled {
		t.Fatalf("persisted admission status = %v, want enrolled", got)
	}
	if len(pub.changed) != 2 {
		t.Errorf("published %d admission changes, want 2 (accepted, then enrolled)", len(pub.changed))
	}
}

func TestMarkInscriptionPaidDoesNotPublishWhenUpdatePreservesTerminalStatus(t *testing.T) {
	svc, students, _, _, pub := newEnrollService(t)
	seedPendingApplicant(t, students, "stu-raced-withdrawal")
	students.updateErr = glerrors.New(glerrors.KindConflict, "admission became terminal while updating")

	err := svc.MarkInscriptionPaid(context.Background(), "stu-raced-withdrawal")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("MarkInscriptionPaid error kind = %v, want conflict", glerrors.KindOf(err))
	}
	if len(pub.changed) != 0 {
		t.Errorf("published %d admission changes after terminal status was preserved, want 0", len(pub.changed))
	}
}

// Payments arrive at least once. A redelivery must be a no-op, or Pub/Sub keeps
// redelivering it forever.
func TestMarkingAnAlreadyEnrolledPupilIsHarmless(t *testing.T) {
	svc, students, _, _, _ := newEnrollService(t)
	seedPendingApplicant(t, students, "stu-3")
	if err := svc.MarkInscriptionPaid(context.Background(), "stu-3"); err != nil {
		t.Fatalf("first: %v", err)
	}

	if err := svc.MarkInscriptionPaid(context.Background(), "stu-3"); err != nil {
		t.Errorf("a redelivered payment must be a no-op, not a failure: %v", err)
	}
}

func TestMarkInscriptionPaidAcknowledgesTerminalAdmissions(t *testing.T) {
	for _, status := range []domain.AdmissionStatus{domain.AdmissionWithdrawn, domain.AdmissionRejected} {
		t.Run(status.String(), func(t *testing.T) {
			svc, students, _, _, pub := newEnrollService(t)
			student := domain.Rehydrate(domain.NewApplicantParams{
				ID: "terminal-student", TenantID: tenant, FirstName: "Ana", LastName: "Hernández",
				Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
			}, status)
			students.byID[student.ID()] = student

			if err := svc.MarkInscriptionPaid(context.Background(), student.ID()); err != nil {
				t.Fatalf("terminal admission must be acknowledged as a no-op: %v", err)
			}
			if len(students.updated) != 0 || len(pub.changed) != 0 {
				t.Errorf("terminal admission was changed: updates=%d events=%d", len(students.updated), len(pub.changed))
			}
		})
	}
}

// A withdrawn pupil cannot be quietly re-enrolled by dropping them in a group:
// withdrawal is terminal, and the refusal has to come from the state machine
// rather than from nobody noticing.
func TestEnrollRefusesAStudentWhoseFileIsTerminal(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	s := domain.Rehydrate(domain.NewApplicantParams{
		ID: "stu-3", TenantID: tenant, FirstName: "Ana", LastName: "Hernández",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionWithdrawn)
	students.byID["stu-3"] = s
	catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99", GradeLevelID: "gl-1"}

	if _, err := svc.Enroll(context.Background(), "stu-3", "grp-1", ""); err == nil {
		t.Fatal("a withdrawn pupil must not be re-enrolled by placing them in a group")
	}
	if len(enrolls.createSeen) != 0 {
		t.Error("the placement was persisted even though the file refused it")
	}
}

// --- placing a pupil by GRADE, letting the school's capacity choose the group --
//
// The panel used to ask for "grado y grupo" as free text, so a secretary typed
// "1° A" and nothing checked whether 1°A had a seat left. What a school actually
// knows is the level and the grade; which group the child lands in is a
// consequence of how full each one is, and that is arithmetic the system should
// be doing.

func TestEnrollIntoGradeTakesTheFirstGroupWithRoom(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "stu-1")
	catalog.gradeGroups["gl-1"] = []ports.GroupSeats{
		{ID: "grp-a", Name: "1° A", CycleID: "cyc-1", Capacity: 2},
		{ID: "grp-b", Name: "1° B", CycleID: "cyc-1", Capacity: 2},
	}

	placed, err := svc.EnrollIntoGrade(context.Background(), "stu-1", "gl-1")
	if err != nil {
		t.Fatalf("EnrollIntoGrade: %v", err)
	}

	if placed.GroupName != "1° A" {
		t.Errorf("placed in %q, want 1° A — a school fills A before B", placed.GroupName)
	}
	if len(enrolls.createSeen) != 1 || enrolls.createSeen[0].GroupID() != "grp-a" {
		t.Errorf("persisted enrollment group = %+v", enrolls.createSeen)
	}
}

// The rule the owner asked for in as many words: when A fills up, the next child
// goes to B.
func TestAFullGroupOverflowsIntoTheNext(t *testing.T) {
	svc, students, enrolls, catalog, _ := newEnrollService(t)
	catalog.gradeGroups["gl-1"] = []ports.GroupSeats{
		{ID: "grp-a", Name: "1° A", CycleID: "cyc-1", Capacity: 2},
		{ID: "grp-b", Name: "1° B", CycleID: "cyc-1", Capacity: 2},
	}

	var landed []string
	for i, id := range []string{"s1", "s2", "s3"} {
		seedStudent(t, students, id)
		placed, err := svc.EnrollIntoGrade(context.Background(), id, "gl-1")
		if err != nil {
			t.Fatalf("pupil %d: %v", i+1, err)
		}
		landed = append(landed, placed.GroupName)
	}

	want := []string{"1° A", "1° A", "1° B"}
	for i := range want {
		if landed[i] != want[i] {
			t.Fatalf("pupils landed in %v, want %v", landed, want)
		}
	}
	_ = enrolls
}

// A grade with every seat taken is the school's decision to make, not ours. It
// refuses and says how full it is, rather than opening a group on its own or
// quietly overfilling one.
func TestAGradeWithNoSeatsLeftIsRefused(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	catalog.gradeGroups["gl-1"] = []ports.GroupSeats{{ID: "grp-a", Name: "1° A", CycleID: "cyc-1", Capacity: 1}}
	seedStudent(t, students, "s1")
	if _, err := svc.EnrollIntoGrade(context.Background(), "s1", "gl-1"); err != nil {
		t.Fatalf("first pupil: %v", err)
	}

	seedStudent(t, students, "s2")
	_, err := svc.EnrollIntoGrade(context.Background(), "s2", "gl-1")

	if err == nil || glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "lugar") {
		t.Errorf("message = %q; it must tell the school its grade is full", err.Error())
	}
}

// A grade nobody has given groups to is a configuration gap, and saying so beats
// "no hay lugares" — the school has to go and create a group, not free a seat.
func TestAGradeWithNoGroupsSaysSo(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedStudent(t, students, "s1")
	catalog.gradeGroups["gl-1"] = nil

	_, err := svc.EnrollIntoGrade(context.Background(), "s1", "gl-1")

	if err == nil {
		t.Fatal("a grade with no groups cannot take a pupil")
	}
	if !strings.Contains(err.Error(), "grupo") {
		t.Errorf("message = %q; it must point at the missing group", err.Error())
	}
}

// Placing by grade must move the pupil's file exactly as placing by group does —
// the two paths cannot disagree about what enrolment means.
func TestEnrollIntoGradeRecordsTheGroupWithoutEnrolling(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	seedPendingApplicant(t, students, "stu-9")
	catalog.gradeGroups["gl-1"] = []ports.GroupSeats{{ID: "grp-b", Name: "1° B", CycleID: "cyc-1", Capacity: 5}}

	if _, err := svc.EnrollIntoGrade(context.Background(), "stu-9", "gl-1"); err != nil {
		t.Fatalf("EnrollIntoGrade: %v", err)
	}

	if len(students.updated) == 0 {
		t.Fatal("the file was never persisted")
	}
	saved := students.updated[len(students.updated)-1]
	// Placed, but still en trámite: inscrito comes with the payment.
	if saved.Status() != domain.AdmissionPending {
		t.Errorf("persisted admission status = %v, want pending", saved.Status())
	}
	// The record follows the placement: "grado y grupo" used to be free text a
	// secretary typed, and it drifted the moment they guessed a group that was
	// full or one that had never been created.
	if saved.GradeGroup() != "1° B" {
		t.Errorf("persisted grade group = %q, want 1° B — the record must say where they landed", saved.GradeGroup())
	}
}

// --- a level being deleted (ADR 0021) --------------------------------------

// Review finding 4: while a level's deletion runs, no path may place a pupil in
// one of its groups — the saga already archived (or is about to archive) its
// enrollments, and a new one would be left pointing at a deleted group. The
// refusal holds for a legacy pupil with no level too.
func TestPlacementsIntoALevelBeingDeletedAreRefused(t *testing.T) {
	cases := []struct {
		name   string
		legacy bool
		run    func(svc *usecase.EnrollmentService, e domain.Enrollment) error
	}{
		{"enroll", false, func(svc *usecase.EnrollmentService, _ domain.Enrollment) error {
			_, err := svc.Enroll(context.Background(), "stu-2", "grp-2", "")
			return err
		}},
		{"enroll a legacy pupil", true, func(svc *usecase.EnrollmentService, _ domain.Enrollment) error {
			_, err := svc.Enroll(context.Background(), "stu-2", "grp-2", "")
			return err
		}},
		{"enroll into grade", false, func(svc *usecase.EnrollmentService, _ domain.Enrollment) error {
			_, err := svc.EnrollIntoGrade(context.Background(), "stu-2", "gl-k")
			return err
		}},
		{"transfer", false, func(svc *usecase.EnrollmentService, e domain.Enrollment) error {
			_, err := svc.Transfer(context.Background(), e.ID(), "grp-3", "x")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, students, enrolls, catalog, pub := newEnrollService(t)
			e := enrolledStudent(t, svc, students, catalog)
			catalog.gradeLevels["gl-doomed"] = primariaLevel
			catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-98", GradeLevelID: "gl-k"}
			catalog.groups["grp-3"] = groupRef{ID: "grp-3", CycleID: "cyc-99", GradeLevelID: "gl-doomed", Capacity: 30}
			catalog.gradeGroups["gl-k"] = []ports.GroupSeats{{ID: "grp-2", Name: "1° A", CycleID: "cyc-98", Capacity: 30}}
			seedStudent(t, students, "stu-2")
			if tc.legacy {
				students.byID["stu-2"] = domain.Rehydrate(domain.NewApplicantParams{
					ID: "stu-2", TenantID: tenant, FirstName: "Beto", LastName: "Legado",
				}, domain.AdmissionAccepted)
			} else {
				students.byID["stu-2"] = domain.Rehydrate(domain.NewApplicantParams{
					ID: "stu-2", TenantID: tenant, FirstName: "Beto", LastName: "K",
					Level: domain.RestoreLevel(kinderLevel, "Kinder"),
				}, domain.AdmissionAccepted)
			}
			catalog.deleting = map[string]bool{kinderLevel: true, primariaLevel: tc.name == "transfer"}
			created, published := len(enrolls.createSeen), len(pub.enrolled)

			err := tc.run(svc, e)
			if got := levelarchive.CodeOf(err); got != levelarchive.CodeLevelBeingDeleted {
				t.Fatalf("code = %q (err %v), want %s", got, err, levelarchive.CodeLevelBeingDeleted)
			}
			if glerrors.KindOf(err) != glerrors.KindConflict {
				t.Errorf("kind = %v, want conflict (409)", glerrors.KindOf(err))
			}
			if len(enrolls.createSeen) != created || len(pub.enrolled) != published || len(enrolls.transfers) != 0 {
				t.Error("nothing may be written or published into a level being deleted")
			}
		})
	}
}
