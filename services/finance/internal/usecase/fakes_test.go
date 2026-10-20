package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// schoolZone is the zone production resolves from SCHOOL_TIMEZONE (ADR 0035):
// every fixture decides "today" on Morelia's calendar, as the service does.
var schoolZone = mustLoadZone("America/Mexico_City")

func mustLoadZone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return zone
}

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
	// err makes ListByCycle fail, as students does when it is unreachable.
	err error
}

func (f *fakeEnrollments) ListByCycle(_ context.Context, cycleID, groupID string) ([]ports.EnrollmentRef, error) {
	f.gotCycleID, f.gotGroupID = cycleID, groupID
	if f.err != nil {
		return nil, f.err
	}
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

// fakeCalendar answers with whatever days the test says the school is shut.
type fakeCalendar struct {
	days []time.Time
}

func (f *fakeCalendar) NonWorkingDays(_ context.Context, _ string) (ports.CalendarSnapshot, error) {
	return ports.CalendarSnapshot{Days: f.days, Version: "test"}, nil
}

// --- publisher ---------------------------------------------------------------

type spyPublisher struct {
	events  []ports.PaymentRegisteredEvent
	settled []ports.ChargeSettledEvent
	err     error
	// settledErr fails ONLY the settled publication, which is how a broker
	// outage is simulated without also breaking the receipt notification.
	settledErr   error
	settledCalls int
	// corrected are the PaymentCorrected publications (T9).
	corrected []ports.PaymentCorrectedEvent
}

func (s *spyPublisher) PublishPaymentCorrected(_ context.Context, e ports.PaymentCorrectedEvent) error {
	s.corrected = append(s.corrected, e)
	return s.err
}

func (s *spyPublisher) PublishChargeSettled(_ context.Context, e ports.ChargeSettledEvent) error {
	s.settledCalls++
	if s.settledErr != nil {
		return s.settledErr
	}
	s.settled = append(s.settled, e)
	return s.err
}

func (s *spyPublisher) PublishPaymentRegistered(_ context.Context, e ports.PaymentRegisteredEvent) error {
	s.events = append(s.events, e)
	return s.err
}

// --- repository --------------------------------------------------------------

// testActor is the signed-in caller the gateway would stamp on a write.
const testActor = "a0000000-0000-4000-8000-0000000000ac"

type fakeRepo struct {
	// cancelledBy records who cancelled each charge (C2-005).
	cancelledBy map[string]string
	// voidedSales is what ListVoidedSalePaymentsOn answers; voidSale* record
	// the last VoidSalePayments call (C2-015).
	voidedSales    []ports.VoidedSalePayment
	voidSaleCalls  int
	voidSaleReason string
	voidSaleActor  string
	voidSaleToday  time.Time
	concepts       map[string]domain.ChargeConcept
	charges        map[string]domain.Charge
	payments       map[string][]domain.Payment // keyed by charge id
	plans          map[string]domain.TuitionPlan
	deposits       []domain.Deposit
	// depositOfPayment stands in for payments.deposit_id: which deposit an
	// allocation came from, so voiding can find and release exactly those.
	depositOfPayment map[string]string
	// outbox mirrors the real table: an event queued in the same "transaction"
	// as the money, still there when publishing fails. A fake that dropped it
	// would prove the opposite of what these tests claim.
	outbox []ports.OutboxEvent
	// corrections are the deposit_corrections rows (T9).
	corrections []domain.DepositCorrection

	// Recorded call arguments, for the tests that assert on the call itself.
	overdueCycleID      string
	overdueToday        time.Time
	lastPageLimit       int
	lastDerivedStatus   string
	cancelIfUnpaidCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		cancelledBy: map[string]string{},
		concepts:    map[string]domain.ChargeConcept{},
		charges:     map[string]domain.Charge{},
		payments:    map[string][]domain.Payment{},
		plans:       map[string]domain.TuitionPlan{},

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

// withPaid pairs a charge with what was collected, and what of it was paid
// by its due date (T18) — as the repository's withPaidTotals does.
func (f *fakeRepo) withPaid(c domain.Charge) ports.ChargeWithPaid {
	// By the payment's DAY (paid_on), as the SQL does — never its instant.
	due := domain.CalendarDay(c.DueDate())
	var byDue int64
	for _, p := range f.payments[c.ID()] {
		if !p.PaidOn().After(due) {
			byDue += p.Amount().Int64()
		}
	}
	return ports.ChargeWithPaid{Charge: c, PaidCents: f.paidFor(c.ID()), PaidByDueCents: byDue}
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
func (f *fakeRepo) CancelChargeIfUnpaid(ctx context.Context, id string) (ports.ChargeWithPaid, error) {
	return f.CancelChargeIfUnpaidWithReason(ctx, id, "", "")
}

func (f *fakeRepo) CancelChargeIfUnpaidWithReason(_ context.Context, id, reason, actorUserID string) (ports.ChargeWithPaid, error) {
	f.cancelIfUnpaidCalls++
	c, ok := f.charges[id]
	if !ok {
		return ports.ChargeWithPaid{}, glerrors.ErrNotFound
	}
	paid := f.paidFor(id)
	cancelled, err := c.CancelIfUnpaidWithReason(paid, reason)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	f.charges[id] = cancelled
	f.cancelledBy[id] = actorUserID
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

// The fake moves what the real repository moves: unpaid charges whose date
// actually differs, named by external ref.
func (f *fakeRepo) RepriceUnpaid(_ context.Context, items []ports.ChargeReprice) (int, error) {
	repriced := 0
	for _, item := range items {
		for id, c := range f.charges {
			if c.ExternalRef() != item.ExternalRef || c.Amount().Int64() >= item.AmountCents ||
				c.Status() == domain.StatusCancelled || c.Discount().Int64() > 0 || f.paidFor(id) > 0 {
				continue
			}
			updated, err := c.WithAmount(item.AmountCents)
			if err != nil {
				return 0, err
			}
			f.charges[id] = updated
			repriced++
		}
	}
	return repriced, nil
}

func (f *fakeRepo) RescheduleUnpaid(_ context.Context, items []ports.ChargeReschedule) (int, error) {
	moved := 0
	for _, item := range items {
		for id, c := range f.charges {
			// The real guards (SQL): a charge still owing — untouched or part
			// paid — whose window actually differs. Settled and cancelled ones
			// are history.
			if c.ExternalRef() != item.ExternalRef || c.Status() == domain.StatusCancelled ||
				f.paidFor(id) >= c.Effective().Int64() ||
				(c.DueDate().Equal(item.DueOn) && c.StartsOn().Equal(item.StartsOn)) {
				continue
			}
			moved2, err := c.WithDueDate(item.DueOn).WithStartsOn(item.StartsOn)
			if err != nil {
				return 0, err
			}
			f.charges[id] = moved2
			moved++
		}
	}
	return moved, nil
}

func (f *fakeRepo) GetCharge(_ context.Context, id string) (ports.ChargeWithPaid, error) {
	c, ok := f.charges[id]
	if !ok {
		return ports.ChargeWithPaid{}, glerrors.ErrNotFound
	}
	return ports.ChargeWithPaid{Charge: c, PaidCents: f.paidFor(id)}, nil
}

// The fake reads what the real repository reads: generated inscription and fee
// lines of one cycle that are not cancelled.
func (f *fakeRepo) ListIssuedLines(_ context.Context, cycleID, studentID string) ([]ports.IssuedLine, error) {
	var out []ports.IssuedLine
	for _, c := range f.charges {
		ref := c.ExternalRef()
		if c.CycleID() != cycleID || (studentID != "" && c.StudentID() != studentID) ||
			c.Status() == domain.StatusCancelled {
			continue
		}
		if !strings.HasPrefix(ref, domain.EnrollmentRefPrefix) && !strings.HasPrefix(ref, domain.FeeRefPrefix) {
			continue
		}
		out = append(out, ports.IssuedLine{StudentID: c.StudentID(), ExternalRef: ref, Description: c.Description()})
	}
	return out, nil
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
		out = append(out, f.withPaid(c))
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
	ps []domain.Payment,
	derive func(charge domain.Charge, newPaidCents int64) (string, error),
	outbox func(charge ports.ChargeWithPaid) []ports.OutboxEvent,
) ([]domain.Payment, ports.ChargeWithPaid, bool, error) {
	first := ps[0]
	c, ok := f.charges[first.ChargeID()]
	if !ok {
		return nil, ports.ChargeWithPaid{}, false, glerrors.ErrNotFound
	}
	if existing := f.paymentSetByKey(first.IdempotencyKey()); len(existing) > 0 {
		replayedCharge := f.charges[existing[0].ChargeID()]
		paid := f.paidFor(existing[0].ChargeID())
		return existing, ports.ChargeWithPaid{Charge: replayedCharge, PaidCents: paid}, true, nil
	}
	newPaid := f.paidFor(first.ChargeID())
	for _, p := range ps {
		newPaid += p.Amount().Int64()
	}
	status, err := derive(c, newPaid)
	if err != nil {
		// The transaction aborts: neither the payment nor the status is stored.
		return nil, ports.ChargeWithPaid{}, false, err
	}
	f.lastDerivedStatus = status
	f.payments[first.ChargeID()] = append(f.payments[first.ChargeID()], ps...)
	// WithStatus re-derives from the same total, so it yields exactly the status
	// the callback returned — recorded above for the tests to assert on.
	updated := c.WithStatus(newPaid)
	f.charges[c.ID()] = updated
	result := ports.ChargeWithPaid{Charge: updated, PaidCents: newPaid}
	if outbox != nil {
		f.outbox = append(f.outbox, outbox(result)...)
	}
	return ps, result, false, nil
}

// paymentSetByKey is the rows stored under key and, for a combined payment,
// under "key#2", "key#3"…, in that order — as the repository reads them.
func (f *fakeRepo) paymentSetByKey(key string) []domain.Payment {
	first, found := f.byIdempotencyKey(key)
	if !found {
		return nil
	}
	out := []domain.Payment{first}
	for i := 2; ; i++ {
		next, more := f.byIdempotencyKey(fmt.Sprintf("%s#%d", key, i))
		if !more {
			return out
		}
		out = append(out, next)
	}
}

// RegisterDeposit mirrors the real repository closely enough to be worth
// trusting: it dedupes on the key, hands the planner the student's live charges
// with their collected totals, and refuses an allocation that would overpay —
// which is the veto the transaction exists to run.
func (f *fakeRepo) RegisterDeposit(
	_ context.Context,
	d domain.Deposit,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
	outbox func(record ports.DepositRecord) []ports.OutboxEvent,
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
		open = append(open, f.withPaid(c))
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
	// Re-prices first, exactly as the repository writes them: the discounts and
	// the allocations that follow are both worked out against the new amount, and
	// a fake that applied them later would veto a payment the real ledger accepts.
	for _, c := range write.Repriced {
		f.charges[c.ID()] = c
	}
	for _, c := range write.Discounted {
		f.charges[c.ID()] = c
	}
	// The scholarship part is written after the discount, carrying both.
	for _, c := range write.Scholarships {
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
	record := ports.DepositRecord{
		Deposit:          d,
		Surcharges:       write.Surcharges,
		Allocations:      write.Allocations,
		UnallocatedCents: write.UnallocatedCents,
		Settled:          settled,
	}
	if outbox != nil {
		f.outbox = append(f.outbox, outbox(record)...)
	}
	return record, false, nil
}

// DrainOutbox mirrors the real one: an event the publisher refuses STAYS. A
// fake that removed it regardless would make the retry test pass against a
// service that loses events.
func (f *fakeRepo) DrainOutbox(_ context.Context, limit int, publish func(ports.OutboxEvent) error) (int, error) {
	published := 0
	var kept []ports.OutboxEvent
	for i, e := range f.outbox {
		if i >= limit {
			kept = append(kept, f.outbox[i:]...)
			break
		}
		if err := publish(e); err != nil {
			kept = append(kept, e)
			continue
		}
		published++
	}
	f.outbox = kept
	return published, nil
}

// pendingOutbox is how many events are still waiting to go out.
func (f *fakeRepo) pendingOutbox() int { return len(f.outbox) }

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

func (f *fakeRepo) IncomeLines(_ context.Context, from, to time.Time) ([]ports.IncomeLine, error) {
	end := to.AddDate(0, 0, 1)
	type key struct {
		concept    string
		creditNote bool
	}
	byConcept := map[key]int64{}
	for chargeID, ps := range f.payments {
		charge, ok := f.charges[chargeID]
		if !ok || charge.Status() == domain.StatusCancelled {
			continue
		}
		for _, p := range ps {
			if p.PaidOn().Before(from) || !p.PaidOn().Before(end) {
				continue
			}
			byConcept[key{charge.ConceptID(), p.Method() == domain.MethodCreditNote}] += p.Amount().Int64()
		}
	}
	out := make([]ports.IncomeLine, 0, len(byConcept))
	for k, cents := range byConcept {
		out = append(out, ports.IncomeLine{
			ConceptID:   k.concept,
			Concept:     f.concepts[k.concept].Name(),
			AmountCents: cents,
			CreditNote:  k.creditNote,
		})
	}
	return out, nil
}

func (f *fakeRepo) DepositReceipt(ctx context.Context, id string) (ports.DepositReceipt, error) {
	d, err := f.GetDeposit(ctx, id)
	if err != nil {
		return ports.DepositReceipt{}, err
	}
	return ports.DepositReceipt{Deposit: d}, nil
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

func (f *fakeRepo) ListDepositsOn(_ context.Context, day time.Time) ([]domain.Deposit, error) {
	var out []domain.Deposit
	for _, d := range f.deposits {
		if d.PaidOn().Equal(domain.CalendarDay(day)) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRepo) VoidSalePayments(_ context.Context, _, reason, actorUserID string, today time.Time) ([]ports.VoidedSalePayment, error) {
	f.voidSaleCalls++
	f.voidSaleReason, f.voidSaleActor, f.voidSaleToday = reason, actorUserID, today
	return f.voidedSales, nil
}

func (f *fakeRepo) ListVoidedSalePaymentsOn(_ context.Context, day time.Time) ([]ports.VoidedSalePayment, error) {
	var out []ports.VoidedSalePayment
	for _, v := range f.voidedSales {
		if v.PaidOn.Equal(domain.CalendarDay(day)) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListLoosePaymentsOn(_ context.Context, day time.Time) ([]ports.LoosePayment, error) {
	var out []ports.LoosePayment
	for chargeID, ps := range f.payments {
		for _, p := range ps {
			if _, inDeposit := f.depositOfPayment[p.ID()]; inDeposit {
				continue
			}
			if p.PaidOn().Equal(domain.CalendarDay(day)) {
				out = append(out, ports.LoosePayment{Payment: p, Description: f.charges[chargeID].Description()})
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) VoidDeposit(_ context.Context, d domain.Deposit) error {
	for i, stored := range f.deposits {
		if stored.ID() != d.ID() {
			continue
		}
		if stored.Voided() || stored.Superseded() {
			return glerrors.New(glerrors.KindConflict, "this deposit is already cancelled or corrected")
		}
		f.deposits[i] = d
		return f.release(d, false)
	}
	return glerrors.ErrNotFound
}

// release mirrors the repository's releaseDeposit: the deposit's allocations
// go, the charges they touched are re-derived, the price-at-payment-date
// discount goes with it and — on a correction — the early-payment reward the
// rule gave it (when it was the last payment on the month).
func (f *fakeRepo) release(d domain.Deposit, resetEarned bool) error {
	{
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
			if c.DiscountReason() == domain.ReasonPriceAtPaymentDate ||
				(resetEarned && c.DiscountReason() == domain.ReasonEarlyPayment && len(f.payments[chargeID]) == 0) {
				cleared, err := c.WithDiscount(0, f.paidFor(chargeID), "")
				if err != nil {
					return err
				}
				c = cleared
			}
			// As the repository: a scholarship part stays only while what is
			// left still settles the charge (T18).
			if c.Scholarship() > 0 && f.paidFor(chargeID) < c.Effective().Int64() {
				cleared, err := c.WithScholarship("", 0, f.paidFor(chargeID))
				if err != nil {
					return err
				}
				c = cleared
			}
			f.charges[chargeID] = c.WithStatus(f.paidFor(chargeID))
		}
		// As the repository does: the daily surcharges this deposit wrote go
		// with it, unless something else paid them.
		for id, c := range f.charges {
			if !strings.HasPrefix(c.ExternalRef(), "dailyfee:") || !strings.HasSuffix(c.ExternalRef(), ":"+d.ID()) ||
				c.Status() == domain.StatusCancelled || f.paidFor(id) > 0 {
				continue
			}
			cancelled, err := c.CancelIfUnpaid(f.paidFor(id))
			if err != nil {
				return err
			}
			f.charges[id] = cancelled
		}
		return nil
	}
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
		// As the repository reads it: a deposit's slice names its deposit.
		for _, p := range ps {
			if d, sliced := f.depositOfPayment[p.ID()]; sliced {
				p = p.WithDepositID(d)
			}
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

// ListOverduePage pages over the overdue CANDIDATES (past due, not cancelled)
// and only then drops the settled ones, exactly as the real query does. A fake
// that filtered first could never return the empty-but-not-last page that hid
// debt from the billing run.
func (f *fakeRepo) ListOverduePage(_ context.Context, cycleID string, today time.Time, after ports.ChargeCursor, limit int) (ports.ChargePage, error) {
	f.overdueCycleID, f.overdueToday = cycleID, today
	f.lastPageLimit = limit
	page := pageOf(f.sortedCharges(func(c domain.Charge) bool {
		return (cycleID == "" || c.CycleID() == cycleID) &&
			c.Status() != domain.StatusCancelled && c.DueDate().Before(today)
	}), after, limit)
	owed := page.Charges[:0:0]
	for _, c := range page.Charges {
		if c.Charge.Effective().Int64()-c.PaidCents > 0 {
			owed = append(owed, c)
		}
	}
	page.Charges = owed
	return page, nil
}

// ListChargesPage is the keyset page of the charges matching the filter.
func (f *fakeRepo) ListChargesPage(_ context.Context, studentID, cycleID string, after ports.ChargeCursor, limit int) (ports.ChargePage, error) {
	f.lastPageLimit = limit
	return pageOf(f.sortedCharges(func(c domain.Charge) bool {
		return (studentID == "" || c.StudentID() == studentID) && (cycleID == "" || c.CycleID() == cycleID)
	}), after, limit), nil
}

// sortedCharges is every charge `keep` accepts, with its collected total, in
// (due_date, id) — the same total order the real query uses, so a cursor walk
// over this fake means what it means in production.
func (f *fakeRepo) sortedCharges(keep func(domain.Charge) bool) []ports.ChargeWithPaid {
	var out []ports.ChargeWithPaid
	for _, c := range f.charges {
		if keep(c) {
			out = append(out, f.withPaid(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return chargeBefore(out[i].Charge, out[j].Charge) })
	return out
}

func chargeBefore(a, b domain.Charge) bool {
	if !a.DueDate().Equal(b.DueDate()) {
		return a.DueDate().Before(b.DueDate())
	}
	return a.ID() < b.ID()
}

// pageOf cuts the page strictly after `after` and says where the next starts.
func pageOf(sorted []ports.ChargeWithPaid, after ports.ChargeCursor, limit int) ports.ChargePage {
	start := 0
	if !after.IsZero() {
		start = sort.Search(len(sorted), func(i int) bool {
			c := sorted[i].Charge
			return c.DueDate().After(after.DueDate) || (c.DueDate().Equal(after.DueDate) && c.ID() > after.ID)
		})
	}
	rest := sorted[start:]
	if len(rest) <= limit {
		return ports.ChargePage{Charges: rest}
	}
	last := rest[limit-1].Charge
	return ports.ChargePage{Charges: rest[:limit], Next: ports.ChargeCursor{DueDate: last.DueDate(), ID: last.ID()}}
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
	calendar    *fakeCalendar
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
		calendar:    &fakeCalendar{},
		publisher:   &spyPublisher{},
	}
	f.clock = fixedNow
	f.svc = usecase.NewFinanceService(f.repo, f.students, f.cycles, f.enrollments, f.catalog, f.calendar, f.publisher,
		testTenant, seqID("id"), usecase.NewSchoolClock(func() time.Time { return f.clock }, schoolZone))
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

// conceptOfKind is the id of the system concept of a kind, if the service has
// created it yet.
func (f *fakeRepo) conceptOfKind(kind string) (string, bool) {
	for _, c := range f.concepts {
		if c.Kind() == kind {
			return c.ID(), true
		}
	}
	return "", false
}

// CorrectDeposit mirrors the repository (T9): replay by the corrected key,
// then supersede the original, release it, split the correction through
// RegisterDeposit's own path, and record the history row.
func (f *fakeRepo) CorrectDeposit(
	ctx context.Context,
	w ports.DepositCorrectionWrite,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
	outbox func(record ports.CorrectionRecord) []ports.OutboxEvent,
) (ports.CorrectionRecord, bool, error) {
	if prior, found := f.depositByKey(w.Corrected.IdempotencyKey()); found {
		if prior.Corrects() != w.OriginalID {
			return ports.CorrectionRecord{}, false, glerrors.New(glerrors.KindConflict, "key belongs to another payment")
		}
		for _, c := range f.corrections {
			if c.CorrectedID() == prior.ID() {
				original, _ := f.GetDeposit(ctx, c.OriginalID())
				return ports.CorrectionRecord{Original: original, Corrected: ports.DepositRecord{Deposit: prior}, Correction: c}, true, nil
			}
		}
	}
	at := -1
	for i, d := range f.deposits {
		if d.ID() == w.OriginalID {
			at = i
		}
	}
	if at < 0 {
		return ports.CorrectionRecord{}, false, glerrors.ErrNotFound
	}
	original := f.deposits[at]
	superseded, err := original.Supersede(w.Corrected.ID(), w.At)
	if err != nil {
		return ports.CorrectionRecord{}, false, err
	}
	var before []domain.Payment
	for _, ps := range f.payments {
		for _, p := range ps {
			if f.depositOfPayment[p.ID()] == original.ID() {
				before = append(before, p)
			}
		}
	}
	if err := f.release(original, true); err != nil {
		return ports.CorrectionRecord{}, false, err
	}
	stored, _, err := f.RegisterDeposit(ctx, w.Corrected.WithCorrects(original.ID(), original.Folio()), plan, nil)
	if err != nil {
		return ports.CorrectionRecord{}, false, err
	}
	superseded = superseded.RehydrateSupersession(w.At, stored.Deposit.ID(), stored.Deposit.Folio())
	f.deposits[at] = superseded
	correction, err := domain.NewDepositCorrection(w.CorrectionID, testTenant, original.StudentID(), original.ID(),
		stored.Deposit.ID(), w.Reason, domain.SnapshotOf(original, before), domain.SnapshotOf(stored.Deposit, stored.Allocations), w.By, w.At)
	if err != nil {
		return ports.CorrectionRecord{}, false, err
	}
	f.corrections = append(f.corrections, correction)
	record := ports.CorrectionRecord{Original: superseded, Corrected: stored, Correction: correction}
	if outbox != nil {
		f.outbox = append(f.outbox, outbox(record)...)
	}
	return record, false, nil
}

func (f *fakeRepo) ListDepositCorrections(_ context.Context, studentID string) ([]domain.DepositCorrection, error) {
	var out []domain.DepositCorrection
	for _, c := range f.corrections {
		if c.StudentID() == studentID {
			out = append(out, c)
		}
	}
	return out, nil
}

// setFolio stamps a folio on a stored deposit (the fake allocates none).
func (f *fakeRepo) setFolio(id, folio string) {
	for i, d := range f.deposits {
		if d.ID() == id {
			f.deposits[i] = d.WithFolio(folio)
		}
	}
	for i, d := range f.deposits {
		if d.Corrects() == id {
			f.deposits[i] = d.WithCorrects(id, folio)
		}
	}
}
