package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
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
	byID       map[string]domain.Enrollment
	uniq       map[string]bool // tenant|student|cycle
	transfers  []domain.EnrollmentTransfer
	createErr  error
	createSeen []domain.Enrollment
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

func (r *fakeEnrollRepo) TransferEnrollment(_ context.Context, enrollmentID, toGroupID string, transfer domain.EnrollmentTransfer) error {
	cur, ok := r.byID[enrollmentID]
	if !ok {
		return glerrors.ErrNotFound
	}
	// Mirror the single-tx contract: record history AND move the group.
	r.transfers = append(r.transfers, transfer)
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

// fakeCatalog is an in-memory CatalogValidator. A missing group/term returns
// glerrors.ErrNotFound, matching the real Connect client's NotFound mapping.
type fakeCatalog struct {
	groups map[string]groupRef
	terms  map[string]termRef
}

func newCatalog() *fakeCatalog {
	return &fakeCatalog{groups: map[string]groupRef{}, terms: map[string]termRef{}}
}

func (c *fakeCatalog) GroupByID(_ context.Context, id string) (groupRef, error) {
	if g, ok := c.groups[id]; ok {
		return g, nil
	}
	return groupRef{}, glerrors.ErrNotFound
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
		Level: domain.LevelPrimaria,
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
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99"}

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

func TestTransferNonActiveIsInvalid(t *testing.T) {
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)
	catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99"}
	if _, err := svc.Withdraw(context.Background(), e.ID()); err != nil {
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
	svc, students, _, catalog, _ := newEnrollService(t)
	e := enrolledStudent(t, svc, students, catalog)

	updated, err := svc.Withdraw(context.Background(), e.ID())
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if updated.Status() != domain.EnrollmentStatusWithdrawn {
		t.Errorf("status = %v, want withdrawn", updated.Status())
	}

	_, err = svc.Withdraw(context.Background(), e.ID())
	if glerrors.KindOf(err) != glerrors.KindInvalid {
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
