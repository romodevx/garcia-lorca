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

func (f *fakeEnrollments) ListByStudent(_ context.Context, studentID string) ([]ports.EnrollmentRef, error) {
	var out []ports.EnrollmentRef
	for _, rows := range f.byCycle {
		for _, e := range rows {
			if e.StudentID == studentID {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// --- publisher ---------------------------------------------------------------

type spyPublisher struct {
	events  []ports.PaymentRegisteredEvent
	settled []ports.ChargeSettledEvent
	err     error
}

func (s *spyPublisher) PublishChargeSettled(_ context.Context, e ports.ChargeSettledEvent) error {
	s.settled = append(s.settled, e)
	return s.err
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
	plans    map[string]domain.TuitionPlan
	deposits []domain.Deposit
	// depositOfPayment stands in for payments.deposit_id: which deposit an
	// allocation came from, so voiding can find and release exactly those.
	depositOfPayment map[string]string

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
		plans:    map[string]domain.TuitionPlan{},

		depositOfPayment: map[string]string{},
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

// RegisterDeposit mirrors the real repository closely enough to be worth
// trusting: it dedupes on the key, hands the planner the student's live charges
// with their collected totals, and refuses an allocation that would overpay —
// which is the veto the transaction exists to run.
func (f *fakeRepo) RegisterDeposit(
	_ context.Context,
	d domain.Deposit,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
) (ports.DepositRecord, bool, error) {
	if original, found := f.depositByKey(d.IdempotencyKey()); found {
		var allocations []domain.Payment
		var allocated int64
		for _, ps := range f.payments {
			for _, p := range ps {
				if f.depositOfPayment[p.ID()] == original.ID() {
					allocations = append(allocations, p)
					allocated += p.Amount().Int64()
				}
			}
		}
		sort.Slice(allocations, func(i, j int) bool { return allocations[i].ID() < allocations[j].ID() })
		return ports.DepositRecord{
			Deposit:          original,
			Allocations:      allocations,
			UnallocatedCents: original.Amount().Int64() - allocated,
		}, true, nil
	}

	var open []ports.ChargeWithPaid
	for _, c := range f.charges {
		if c.StudentID() != d.StudentID() || c.Status() == domain.StatusCancelled {
			continue
		}
		open = append(open, ports.ChargeWithPaid{Charge: c, PaidCents: f.paidFor(c.ID())})
	}
	sort.Slice(open, func(i, j int) bool {
		if !open[i].Charge.DueDate().Equal(open[j].Charge.DueDate()) {
			return open[i].Charge.DueDate().Before(open[j].Charge.DueDate())
		}
		return open[i].Charge.ID() < open[j].Charge.ID()
	})

	write, err := plan(open)
	if err != nil {
		return ports.DepositRecord{}, false, err
	}

	f.deposits = append(f.deposits, d)
	for _, c := range write.Discounted {
		f.charges[c.ID()] = c
	}
	for _, fee := range write.Surcharges {
		f.charges[fee.ID()] = fee
	}
	var settled []ports.ChargeWithPaid
	for _, p := range write.Allocations {
		c, ok := f.charges[p.ChargeID()]
		if !ok {
			return ports.DepositRecord{}, false, glerrors.New(glerrors.KindInternal,
				"deposit allocated to a charge that is not in the transaction")
		}
		newPaid := f.paidFor(p.ChargeID()) + p.Amount().Int64()
		if newPaid > c.Effective().Int64() {
			return ports.DepositRecord{}, false, glerrors.New(glerrors.KindInternal,
				"deposit allocated more than a charge owed")
		}
		f.payments[p.ChargeID()] = append(f.payments[p.ChargeID()], p)
		f.depositOfPayment[p.ID()] = d.ID()
		updated := c.WithStatus(newPaid)
		f.charges[c.ID()] = updated
		if updated.Status() == domain.StatusPaid {
			settled = append(settled, ports.ChargeWithPaid{Charge: updated, PaidCents: newPaid})
		}
	}
	return ports.DepositRecord{
		Deposit:          d,
		Surcharges:       write.Surcharges,
		Allocations:      write.Allocations,
		UnallocatedCents: write.UnallocatedCents,
		Settled:          settled,
	}, false, nil
}

func (f *fakeRepo) depositByKey(key string) (domain.Deposit, bool) {
	if key == "" {
		return domain.Deposit{}, false
	}
	for _, d := range f.deposits {
		if d.IdempotencyKey() == key && !d.Voided() {
			return d, true
		}
	}
	return domain.Deposit{}, false
}

func (f *fakeRepo) UnallocatedCents(_ context.Context, studentID string) (int64, error) {
	var deposited int64
	live := map[string]bool{}
	for _, d := range f.deposits {
		if d.StudentID() != studentID || d.Voided() {
			continue
		}
		deposited += d.Amount().Int64()
		live[d.ID()] = true
	}
	var allocated int64
	for _, ps := range f.payments {
		for _, p := range ps {
			if live[f.depositOfPayment[p.ID()]] {
				allocated += p.Amount().Int64()
			}
		}
	}
	return deposited - allocated, nil
}

func (f *fakeRepo) GetDeposit(_ context.Context, id string) (domain.Deposit, error) {
	for _, d := range f.deposits {
		if d.ID() == id {
			return d, nil
		}
	}
	return domain.Deposit{}, glerrors.ErrNotFound
}

func (f *fakeRepo) ListDeposits(_ context.Context, studentID string) ([]domain.Deposit, error) {
	out := make([]domain.Deposit, 0, len(f.deposits))
	for _, d := range f.deposits {
		if studentID != "" && d.StudentID() != studentID {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeRepo) VoidDeposit(_ context.Context, d domain.Deposit) error {
	for i, stored := range f.deposits {
		if stored.ID() != d.ID() {
			continue
		}
		if stored.Voided() {
			return glerrors.New(glerrors.KindConflict, "this deposit is already cancelled")
		}
		f.deposits[i] = d
		touched := map[string]bool{}
		for chargeID, ps := range f.payments {
			var kept []domain.Payment
			for _, p := range ps {
				if f.depositOfPayment[p.ID()] == d.ID() {
					touched[chargeID] = true
					continue
				}
				kept = append(kept, p)
			}
			f.payments[chargeID] = kept
		}
		for chargeID := range touched {
			c := f.charges[chargeID]
			f.charges[chargeID] = c.WithStatus(f.paidFor(chargeID))
		}
		return nil
	}
	return glerrors.ErrNotFound
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

// overdueCap mirrors the real repository's maxListLimit. The fake used to return
// everything, which made it MORE capable than production and hid a money bug:
// past this many overdue charges the billing run silently stopped accruing
// surcharges. A fake that cannot express the production bound cannot catch it.
const overdueCap = 500

func (f *fakeRepo) ListOverdue(_ context.Context, cycleID string, today time.Time) ([]ports.ChargeWithPaid, error) {
	all, err := f.listOverdueFrom(cycleID, today)
	if err != nil {
		return nil, err
	}
	if len(all) > overdueCap {
		return all[:overdueCap], nil
	}
	return all, nil
}

// ListOverduePage is the paged read the billing run uses: it walks the whole set
// in (due_date, id) order instead of stopping at a cap.
func (f *fakeRepo) ListOverduePage(_ context.Context, cycleID string, today time.Time, after ports.OverdueCursor, limit int) ([]ports.ChargeWithPaid, error) {
	all, err := f.listOverdueFrom(cycleID, today)
	if err != nil {
		return nil, err
	}
	var out []ports.ChargeWithPaid
	for _, c := range all {
		if !after.IsZero() && !afterCursor(c, after) {
			continue
		}
		out = append(out, c)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func afterCursor(c ports.ChargeWithPaid, after ports.OverdueCursor) bool {
	if c.Charge.DueDate().After(after.DueDate) {
		return true
	}
	return c.Charge.DueDate().Equal(after.DueDate) && c.Charge.ID() > after.ID
}

func (f *fakeRepo) listOverdueFrom(cycleID string, today time.Time) ([]ports.ChargeWithPaid, error) {
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
	// (due_date, id) — the same total order the real query uses, so a cursor
	// walk over this fake means what it means in production.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Charge.DueDate().Equal(out[j].Charge.DueDate()) {
			return out[i].Charge.DueDate().Before(out[j].Charge.DueDate())
		}
		return out[i].Charge.ID() < out[j].Charge.ID()
	})
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
	catalog     *fakeCatalog
	publisher   *spyPublisher
	// clock is what the service reads for "now". It starts at fixedNow and a
	// test may move it forward — the collection rules are all functions of a
	// date, so a test that cannot say what day it is cannot exercise them.
	clock time.Time
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
		catalog:     &fakeCatalog{},
		publisher:   &spyPublisher{},
	}
	f.clock = fixedNow
	f.svc = usecase.NewFinanceService(f.repo, f.students, f.cycles, f.enrollments, f.catalog, nil, f.publisher,
		testTenant, seqID("id"), func() time.Time { return f.clock })
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

// --- tuition doubles (M15) ---------------------------------------------------

func (f *fakeRepo) EnsureConceptOfKind(_ context.Context, id, kind, name string) (domain.ChargeConcept, error) {
	for _, c := range f.concepts {
		if c.Kind() == kind {
			return c, nil
		}
	}
	recurrence := domain.RecurrenceOnce
	if kind == domain.KindTuition {
		recurrence = domain.RecurrenceMonthly
	}
	c, err := domain.NewChargeConcept(id, testTenant, name, 0, recurrence, true)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	c, err = c.WithKind(kind)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	f.concepts[c.ID()] = c
	return c, nil
}

// conceptIDOfKind is a test convenience: which concept ended up being the school's
// tuition, inscription or surcharge.
func (f *fakeRepo) conceptIDOfKind(kind string) string {
	for _, c := range f.concepts {
		if c.Kind() == kind {
			return c.ID()
		}
	}
	return ""
}

func (f *fakeRepo) planKey(cycleID, levelID string) string { return cycleID + "|" + levelID }

func (f *fakeRepo) UpsertTuitionPlan(_ context.Context, p domain.TuitionPlan) (domain.TuitionPlan, error) {
	f.plans[f.planKey(p.CycleID(), p.LevelID())] = p
	return p, nil
}

func (f *fakeRepo) ListTuitionPlans(_ context.Context, cycleID string) ([]domain.TuitionPlan, error) {
	out := make([]domain.TuitionPlan, 0, len(f.plans))
	for _, p := range f.plans {
		if p.CycleID() == cycleID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LevelID() < out[j].LevelID() })
	return out, nil
}

// fakeCatalog stands in for the config service: the levels a school has, and
// which level each group of a cycle belongs to.
type fakeCatalog struct {
	levels      []ports.LevelRef
	groupLevels []ports.GroupLevel
}

func (f *fakeCatalog) ListLevels(_ context.Context) ([]ports.LevelRef, error) { return f.levels, nil }

func (f *fakeCatalog) GroupLevels(_ context.Context, _ string) ([]ports.GroupLevel, error) {
	return f.groupLevels, nil
}

// clearPlans hands back an empty tuition table, for the tests about a school that
// has not priced a level.
func clearPlans() map[string]domain.TuitionPlan { return map[string]domain.TuitionPlan{} }
