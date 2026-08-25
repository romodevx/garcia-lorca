package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// Test doubles for the four seams the finance use case depends on, plus the
// small helpers the table tests share. They are deliberately behavioural rather
// than stubs: the repository reproduces the duplicate keys that yield
// KindConflict and runs the caller's derive callback over its own payment sums,
// so the overpay and cancelled-charge rules are exercised for real.

const (
	testTenant  = "t1"
	testStudent = "stu-1"
	testCycle   = "cyc-1"
	testDueDate = "2026-09-05"
)

// fixedNow is the injected clock: every test that stamps a payment or asks for
// overdue charges reads this instant, never the wall clock.
var fixedNow = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)

// Compile-time proof that the fakes still satisfy the ports.
var (
	_ ports.FinanceRepository = (*fakeRepo)(nil)
	_ ports.StudentReader     = (*fakeStudents)(nil)
	_ ports.CycleReader       = (*fakeCycles)(nil)
	_ ports.EnrollmentReader  = (*fakeEnrollments)(nil)
	_ ports.PaymentPublisher  = (*spyPublisher)(nil)
)

// --- cross-service readers ---------------------------------------------------

type fakeStudents struct {
	byID map[string]ports.StudentRef
}

func (f *fakeStudents) ByID(_ context.Context, id string) (ports.StudentRef, error) {
	s, ok := f.byID[id]
	if !ok {
		return ports.StudentRef{}, glerrors.ErrNotFound
	}
	return s, nil
}

type fakeCycles struct {
	byID map[string]ports.CycleRef
}

func (f *fakeCycles) ByID(_ context.Context, id string) (ports.CycleRef, error) {
	c, ok := f.byID[id]
	if !ok {
		return ports.CycleRef{}, glerrors.ErrNotFound
	}
	return c, nil
}

// fakeEnrollments records the filter it was called with so the bulk-generation
// test can prove the group narrowing reaches the students service.
type fakeEnrollments struct {
	byCycle    map[string][]ports.EnrollmentRef
	gotCycleID string
	gotGroupID string
}

func (f *fakeEnrollments) ListByCycle(_ context.Context, cycleID, groupID string) ([]ports.EnrollmentRef, error) {
	f.gotCycleID, f.gotGroupID = cycleID, groupID
	var out []ports.EnrollmentRef
	for _, e := range f.byCycle[cycleID] {
		if groupID != "" && e.GroupID != groupID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// --- publisher ---------------------------------------------------------------

type spyPublisher struct {
	events []ports.PaymentRegisteredEvent
	err    error
}

func (s *spyPublisher) PublishPaymentRegistered(_ context.Context, e ports.PaymentRegisteredEvent) error {
	s.events = append(s.events, e)
	return s.err
}

// --- repository --------------------------------------------------------------

type fakeRepo struct {
	concepts map[string]domain.ChargeConcept
	charges  map[string]domain.Charge
	payments map[string][]domain.Payment // keyed by charge id

	// Recorded call arguments, for the tests that assert on the call itself.
	overdueCycleID      string
	overdueToday        time.Time
	lastDerivedStatus   string
	cancelIfUnpaidCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		concepts: map[string]domain.ChargeConcept{},
		charges:  map[string]domain.Charge{},
		payments: map[string][]domain.Payment{},
	}
}

// chargeKey is the uniqueness key the ledger enforces: one charge per student,
// cycle and concept.
// chargeKey mirrors what the ledger actually enforces, which since migration
// 0003 is TWO partial unique indexes rather than one total key: a charge with no
// external ref is one per (student, cycle, concept), and a charge carrying one
// is identified by the ref alone. A fake that kept the old single key would let
// tests pass against a rule the database no longer has.
func chargeKey(c domain.Charge) string {
	if ref := c.ExternalRef(); ref != "" {
		return "ref|" + ref
	}
	return c.StudentID() + "|" + c.CycleID() + "|" + c.ConceptID()
}

// paidFor sums the payments stored against a charge.
func (f *fakeRepo) paidFor(chargeID string) int64 {
	var total int64
	for _, p := range f.payments[chargeID] {
		total += p.Amount().Int64()
	}
	return total
}

func (f *fakeRepo) hasChargeKey(key string) bool {
	for _, existing := range f.charges {
		if chargeKey(existing) == key {
			return true
		}
	}
	return false
}

func (f *fakeRepo) CreateConcept(_ context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error) {
	for _, existing := range f.concepts {
		if existing.Name() == c.Name() {
			return domain.ChargeConcept{}, glerrors.New(glerrors.KindConflict, "concept name already exists")
		}
	}
	f.concepts[c.ID()] = c
	return c, nil
}

func (f *fakeRepo) GetConcept(_ context.Context, id string) (domain.ChargeConcept, error) {
	c, ok := f.concepts[id]
	if !ok {
		return domain.ChargeConcept{}, glerrors.ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) ListConcepts(_ context.Context, includeInactive bool) ([]domain.ChargeConcept, error) {
	out := make([]domain.ChargeConcept, 0, len(f.concepts))
	for _, c := range f.concepts {
		if !includeInactive && !c.Active() {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (f *fakeRepo) UpdateConcept(_ context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error) {
	f.concepts[c.ID()] = c
	return c, nil
}

func (f *fakeRepo) CreateCharge(_ context.Context, c domain.Charge) (domain.Charge, error) {
	if f.hasChargeKey(chargeKey(c)) {
		return domain.Charge{}, glerrors.New(glerrors.KindConflict, "student already has this charge")
	}
	f.charges[c.ID()] = c
	return c, nil
}

// CancelChargeIfUnpaid is the fake of the guarded cancel. The real one decides
// under the charge's row lock; here it is enough to apply the same domain rule
// against the payments recorded for the charge.
func (f *fakeRepo) CancelChargeIfUnpaid(_ context.Context, id string) (ports.ChargeWithPaid, error) {
	f.cancelIfUnpaidCalls++
	c, ok := f.charges[id]
	if !ok {
		return ports.ChargeWithPaid{}, glerrors.ErrNotFound
	}
	paid := f.paidFor(id)
	cancelled, err := c.CancelIfUnpaid(paid)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	f.charges[id] = cancelled
	return ports.ChargeWithPaid{Charge: cancelled, PaidCents: paid}, nil
}

// CreateChargesBulk mirrors the adapter's ON CONFLICT DO NOTHING: rows whose
// (student, cycle, concept) key already exists are skipped, not rejected.
func (f *fakeRepo) CreateChargesBulk(_ context.Context, cs []domain.Charge) (int, error) {
	created := 0
	for _, c := range cs {
		if f.hasChargeKey(chargeKey(c)) {
			continue
		}
		f.charges[c.ID()] = c
		created++
	}
	return created, nil
}

func (f *fakeRepo) GetCharge(_ context.Context, id string) (ports.ChargeWithPaid, error) {
	c, ok := f.charges[id]
	if !ok {
		return ports.ChargeWithPaid{}, glerrors.ErrNotFound
	}
	return ports.ChargeWithPaid{Charge: c, PaidCents: f.paidFor(id)}, nil
}

func (f *fakeRepo) ListCharges(_ context.Context, studentID, cycleID string) ([]ports.ChargeWithPaid, error) {
	var out []ports.ChargeWithPaid
	for _, c := range f.charges {
		if studentID != "" && c.StudentID() != studentID {
			continue
		}
		if cycleID != "" && c.CycleID() != cycleID {
			continue
		}
		out = append(out, ports.ChargeWithPaid{Charge: c, PaidCents: f.paidFor(c.ID())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Charge.ID() < out[j].Charge.ID() })
	return out, nil
}

func (f *fakeRepo) SaveCharge(_ context.Context, c domain.Charge) (domain.Charge, error) {
	if _, ok := f.charges[c.ID()]; !ok {
		return domain.Charge{}, glerrors.ErrNotFound
	}
	f.charges[c.ID()] = c
	return c, nil
}

// RegisterPayment reproduces the atomic knot: dedupe the attempt by its
// idempotency key, then sum the stored payments, let the caller's derive
// callback decide (and veto), and store the payment and the charge's new status
// together. A key already on the ledger replays the original payment and writes
// nothing, exactly like the real transaction.
func (f *fakeRepo) RegisterPayment(
	_ context.Context,
	p domain.Payment,
	derive func(charge domain.Charge, newPaidCents int64) (string, error),
) (domain.Payment, ports.ChargeWithPaid, bool, error) {
	c, ok := f.charges[p.ChargeID()]
	if !ok {
		return domain.Payment{}, ports.ChargeWithPaid{}, false, glerrors.ErrNotFound
	}
	if existing, found := f.byIdempotencyKey(p.IdempotencyKey()); found {
		replayedCharge := f.charges[existing.ChargeID()]
		paid := f.paidFor(existing.ChargeID())
		return existing, ports.ChargeWithPaid{Charge: replayedCharge, PaidCents: paid}, true, nil
	}
	newPaid := f.paidFor(p.ChargeID()) + p.Amount().Int64()
	status, err := derive(c, newPaid)
	if err != nil {
		// The transaction aborts: neither the payment nor the status is stored.
		return domain.Payment{}, ports.ChargeWithPaid{}, false, err
	}
	f.lastDerivedStatus = status
	f.payments[p.ChargeID()] = append(f.payments[p.ChargeID()], p)
	// WithStatus re-derives from the same total, so it yields exactly the status
	// the callback returned — recorded above for the tests to assert on.
	updated := c.WithStatus(newPaid)
	f.charges[c.ID()] = updated
	return p, ports.ChargeWithPaid{Charge: updated, PaidCents: newPaid}, false, nil
}

// byIdempotencyKey finds a payment already posted under key. An empty key never
// matches: it means the caller opted out of deduplication.
func (f *fakeRepo) byIdempotencyKey(key string) (domain.Payment, bool) {
	if key == "" {
		return domain.Payment{}, false
	}
	for _, ps := range f.payments {
		for _, p := range ps {
			if p.IdempotencyKey() == key {
				return p, true
			}
		}
	}
	return domain.Payment{}, false
}

func (f *fakeRepo) ListPayments(_ context.Context, studentID string) ([]domain.Payment, error) {
	var out []domain.Payment
	for chargeID, ps := range f.payments {
		c, ok := f.charges[chargeID]
		if !ok || c.StudentID() != studentID {
			continue
		}
		out = append(out, ps...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (f *fakeRepo) ListOverdue(_ context.Context, cycleID string, today time.Time) ([]ports.ChargeWithPaid, error) {
	f.overdueCycleID, f.overdueToday = cycleID, today
	var out []ports.ChargeWithPaid
	for _, c := range f.charges {
		if cycleID != "" && c.CycleID() != cycleID {
			continue
		}
		if c.Status() == domain.StatusCancelled || !c.DueDate().Before(today) {
			continue
		}
		paid := f.paidFor(c.ID())
		if c.Effective().Int64()-paid <= 0 {
			continue
		}
		out = append(out, ports.ChargeWithPaid{Charge: c, PaidCents: paid})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Charge.ID() < out[j].Charge.ID() })
	return out, nil
}

// --- fixtures ----------------------------------------------------------------

// fixture bundles a service with the doubles behind it, so a test can both act
// and inspect.
type fixture struct {
	svc         *usecase.FinanceService
	repo        *fakeRepo
	students    *fakeStudents
	cycles      *fakeCycles
	enrollments *fakeEnrollments
	publisher   *spyPublisher
}

// newFixture wires a service over fresh doubles: one active student, one cycle,
// a deterministic id sequence and the fixed clock.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo:        newFakeRepo(),
		students:    &fakeStudents{byID: map[string]ports.StudentRef{testStudent: {ID: testStudent, Status: "active"}}},
		cycles:      &fakeCycles{byID: map[string]ports.CycleRef{testCycle: {ID: testCycle}}},
		enrollments: &fakeEnrollments{byCycle: map[string][]ports.EnrollmentRef{}},
		publisher:   &spyPublisher{},
	}
	f.svc = usecase.NewFinanceService(f.repo, f.students, f.cycles, f.enrollments, f.publisher,
		testTenant, seqID("id"), func() time.Time { return fixedNow })
	return f
}

// seqID mints deterministic ids: prefix-1, prefix-2, …
func seqID(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s-%d", prefix, n)
	}
}

// seedConcept creates a concept through the service and returns it.
func (f *fixture) seedConcept(t *testing.T, name string, amount int64) domain.ChargeConcept {
	t.Helper()
	c, err := f.svc.CreateConcept(context.Background(), name, amount, domain.RecurrenceMonthly)
	if err != nil {
		t.Fatalf("seed concept %q: %v", name, err)
	}
	return c
}

// seedRetiredConcept creates a concept and deactivates it — the catalog state a
// charge may no longer be issued from.
func (f *fixture) seedRetiredConcept(t *testing.T, name string, amount int64) domain.ChargeConcept {
	t.Helper()
	c := f.seedConcept(t, name, amount)
	retired, err := f.svc.UpdateConcept(context.Background(), c.ID(), name, amount, domain.RecurrenceMonthly, false)
	if err != nil {
		t.Fatalf("retire concept %q: %v", name, err)
	}
	return retired
}

// seedCharge creates a charge for the default student through the service.
func (f *fixture) seedCharge(t *testing.T, conceptID string, amount int64) domain.Charge {
	t.Helper()
	cwp, err := f.svc.CreateCharge(context.Background(), usecase.CreateChargeInput{
		StudentID:           testStudent,
		CycleID:             testCycle,
		ConceptID:           conceptID,
		DueDate:             testDueDate,
		AmountCentsOverride: amount,
	})
	if err != nil {
		t.Fatalf("seed charge: %v", err)
	}
	return cwp.Charge
}

// requireKind fails unless err carries the expected Kind.
func requireKind(t *testing.T, err error, want glerrors.Kind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error of kind %v, got nil", want)
	}
	if got := glerrors.KindOf(err); got != want {
		t.Fatalf("kind = %v, want %v (err: %v)", got, want, err)
	}
}

// requireNoError fails on any error.
func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
