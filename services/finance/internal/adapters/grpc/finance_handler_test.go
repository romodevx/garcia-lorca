package grpcadapter_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The handler is translate-only, so these tests drive a REAL FinanceService over
// behavioural doubles and assert on the wire: the derived paid/remaining fields
// the domain deliberately does not store, the due-date wire format, and the
// domain Kind → Connect code mapping.

const (
	testTenant  = "t1"
	testStudent = "stu-1"
	testCycle   = "cyc-1"
	testDueDate = "2026-09-05"
)

// fixedNow is the injected clock: nothing here reads the wall clock.
var fixedNow = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)

// --- cross-service readers ---------------------------------------------------

type fakeStudents struct{ byID map[string]ports.StudentRef }

func (f *fakeStudents) ByID(_ context.Context, id string) (ports.StudentRef, error) {
	s, ok := f.byID[id]
	if !ok {
		return ports.StudentRef{}, glerrors.ErrNotFound
	}
	return s, nil
}

type fakeCycles struct{ byID map[string]ports.CycleRef }

func (f *fakeCycles) ByID(_ context.Context, id string) (ports.CycleRef, error) {
	c, ok := f.byID[id]
	if !ok {
		return ports.CycleRef{}, glerrors.ErrNotFound
	}
	return c, nil
}

type fakeEnrollments struct {
	byCycle map[string][]ports.EnrollmentRef
}

func (f *fakeEnrollments) ListByCycle(_ context.Context, cycleID, groupID string) ([]ports.EnrollmentRef, error) {
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

// --- repository ---------------------------------------------------------------

// fakeRepo is behavioural, not a stub: it reproduces the duplicate keys that
// yield KindConflict and runs the caller's derive callback over its own payment
// sums, so the overpay rule is exercised for real.
type fakeRepo struct {
	concepts map[string]domain.ChargeConcept
	charges  map[string]domain.Charge
	payments map[string][]domain.Payment // keyed by charge id
	plans    []domain.TuitionPlan
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		concepts: map[string]domain.ChargeConcept{},
		charges:  map[string]domain.Charge{},
		payments: map[string][]domain.Payment{},
	}
}

func chargeKey(c domain.Charge) string {
	return c.StudentID() + "|" + c.CycleID() + "|" + c.ConceptID()
}

// The deposit path is exercised at the use-case layer; this handler fake only
// has to satisfy the port so the wiring compiles.

func (f *fakeRepo) DrainOutbox(_ context.Context, _ int, _ func(ports.OutboxEvent) error) (int, error) {
	return 0, nil
}

func (f *fakeRepo) RegisterDeposit(_ context.Context, d domain.Deposit,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
	_ func(record ports.DepositRecord) []ports.OutboxEvent,
) (ports.DepositRecord, bool, error) {
	write, err := plan(nil)
	if err != nil {
		return ports.DepositRecord{}, false, err
	}
	return ports.DepositRecord{Deposit: d, UnallocatedCents: write.UnallocatedCents}, false, nil
}

func (f *fakeRepo) UnallocatedCents(_ context.Context, _ string) (int64, error) { return 0, nil }

func (f *fakeRepo) IncomeLines(_ context.Context, _, _ time.Time) ([]ports.IncomeLine, error) {
	return nil, nil
}

func (f *fakeRepo) GetDeposit(_ context.Context, _ string) (domain.Deposit, error) {
	return domain.Deposit{}, glerrors.ErrNotFound
}

func (f *fakeRepo) ListDeposits(_ context.Context, _ string) ([]domain.Deposit, error) {
	return nil, nil
}

func (f *fakeRepo) ListDepositsOn(_ context.Context, _ time.Time) ([]domain.Deposit, error) {
	return nil, nil
}

func (f *fakeRepo) VoidDeposit(_ context.Context, _ domain.Deposit) error { return nil }

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

func (f *fakeRepo) CancelChargeIfUnpaid(_ context.Context, id string) (ports.ChargeWithPaid, error) {
	c, ok := f.charges[id]
	if !ok {
		return ports.ChargeWithPaid{}, glerrors.ErrNotFound
	}
	var paid int64
	for _, p := range f.payments[id] {
		paid += p.Amount().Int64()
	}
	cancelled, err := c.CancelIfUnpaid(paid)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	f.charges[id] = cancelled
	return ports.ChargeWithPaid{Charge: cancelled, PaidCents: paid}, nil
}

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

// RegisterPayment mirrors the adapter's transaction: it replays a payment
// already posted under the request's idempotency key, otherwise runs derive over
// the total the new payment would bring the charge to, and aborts on its error.
func (f *fakeRepo) RegisterPayment(
	_ context.Context,
	p domain.Payment,
	derive func(charge domain.Charge, newPaidCents int64) (string, error),
	_ func(charge ports.ChargeWithPaid) []ports.OutboxEvent,
) (domain.Payment, ports.ChargeWithPaid, bool, error) {
	charge, ok := f.charges[p.ChargeID()]
	if !ok {
		return domain.Payment{}, ports.ChargeWithPaid{}, false, glerrors.ErrNotFound
	}
	if original, found := f.byIdempotencyKey(p.IdempotencyKey()); found {
		return original, ports.ChargeWithPaid{Charge: charge, PaidCents: f.paidFor(p.ChargeID())}, true, nil
	}
	newPaid := f.paidFor(p.ChargeID()) + p.Amount().Int64()
	status, err := derive(charge, newPaid)
	if err != nil {
		return domain.Payment{}, ports.ChargeWithPaid{}, false, err
	}
	f.payments[p.ChargeID()] = append(f.payments[p.ChargeID()], p)
	updated := charge.WithStatus(newPaid)
	if updated.Status() != status {
		return domain.Payment{}, ports.ChargeWithPaid{}, false, fmt.Errorf("derive said %q but the charge derived %q", status, updated.Status())
	}
	f.charges[updated.ID()] = updated
	return p, ports.ChargeWithPaid{Charge: updated, PaidCents: newPaid}, false, nil
}

// byIdempotencyKey finds a payment already posted under key; an empty key never
// matches, because it means the caller opted out of deduplication.
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
		if c, ok := f.charges[chargeID]; !ok || c.StudentID() != studentID {
			continue
		}
		out = append(out, ps...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (f *fakeRepo) ListOverdue(_ context.Context, cycleID string, today time.Time) ([]ports.ChargeWithPaid, error) {
	var out []ports.ChargeWithPaid
	for _, c := range f.charges {
		if cycleID != "" && c.CycleID() != cycleID {
			continue
		}
		paid := f.paidFor(c.ID())
		if c.Status() == domain.StatusCancelled || !c.DueDate().Before(today) || c.Effective().Int64()-paid <= 0 {
			continue
		}
		out = append(out, ports.ChargeWithPaid{Charge: c, PaidCents: paid})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Charge.ID() < out[j].Charge.ID() })
	return out, nil
}

// ListOverduePage is the uncapped keyset walk the billing run uses. This fake
// holds a handful of charges, so one page always covers them; the walk semantics
// are exercised properly in the use-case tests.
func (f *fakeRepo) ListOverduePage(ctx context.Context, cycleID string, today time.Time, after ports.OverdueCursor, limit int) ([]ports.ChargeWithPaid, error) {
	if !after.IsZero() {
		return nil, nil // one page is enough here; a second walk is always empty
	}
	all, err := f.ListOverdue(ctx, cycleID, today)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(all) > limit {
		return all[:limit], nil
	}
	return all, nil
}

// --- fixture -------------------------------------------------------------------

type fixture struct {
	h           *grpcadapter.FinanceHandler
	repo        *fakeRepo
	enrollments *fakeEnrollments
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := newFakeRepo()
	students := &fakeStudents{byID: map[string]ports.StudentRef{testStudent: {ID: testStudent, Status: "active"}}}
	cycles := &fakeCycles{byID: map[string]ports.CycleRef{testCycle: {ID: testCycle}}}
	enrollments := &fakeEnrollments{byCycle: map[string][]ports.EnrollmentRef{}}

	seq := 0
	newID := func() string { seq++; return fmt.Sprintf("id-%d", seq) }
	svc := usecase.NewFinanceService(repo, students, cycles, enrollments, &fakeCatalog{}, nil, nil,
		testTenant, newID, func() time.Time { return fixedNow })
	return &fixture{
		h:           grpcadapter.NewFinanceHandler(svc, usecase.NewHealthService("finance")),
		repo:        repo,
		enrollments: enrollments,
	}
}

// seedConcept registers a catalog entry through the handler.
func (f *fixture) seedConcept(t *testing.T, name string, amount int64) *financev1.ChargeConcept {
	t.Helper()
	res, err := f.h.CreateConcept(context.Background(), connect.NewRequest(&financev1.CreateConceptRequest{
		Name: name, DefaultAmountCents: amount, Recurrence: domain.RecurrenceMonthly,
	}))
	if err != nil {
		t.Fatalf("seed concept %q: %v", name, err)
	}
	return res.Msg.GetConcept()
}

// seedCharge issues a charge for the default student through the handler.
func (f *fixture) seedCharge(t *testing.T, conceptID string, amount int64) *financev1.Charge {
	t.Helper()
	res, err := f.h.CreateCharge(context.Background(), connect.NewRequest(&financev1.CreateChargeRequest{
		StudentId: testStudent, CycleId: testCycle, ConceptId: conceptID,
		AmountCentsOverride: amount, DueDate: testDueDate,
	}))
	if err != nil {
		t.Fatalf("seed charge: %v", err)
	}
	return res.Msg.GetCharge()
}

// --- tests ----------------------------------------------------------------------

func TestHealth(t *testing.T) {
	f := newFixture(t)
	res, err := f.h.Health(context.Background(), connect.NewRequest(&financev1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

func TestCreateConceptMapsEveryField(t *testing.T) {
	f := newFixture(t)
	got := f.seedConcept(t, "Colegiatura", 250000)

	if got.GetId() == "" || got.GetTenantId() != testTenant {
		t.Errorf("identity = %+v", got)
	}
	if got.GetName() != "Colegiatura" || got.GetDefaultAmountCents() != 250000 {
		t.Errorf("name/amount = %+v", got)
	}
	if got.GetRecurrence() != domain.RecurrenceMonthly || !got.GetActive() {
		t.Errorf("recurrence/active = %+v", got)
	}
}

func TestCreateConceptDuplicateNameIsAlreadyExists(t *testing.T) {
	f := newFixture(t)
	f.seedConcept(t, "Colegiatura", 250000)

	_, err := f.h.CreateConcept(context.Background(), connect.NewRequest(&financev1.CreateConceptRequest{
		Name: "Colegiatura", DefaultAmountCents: 250000, Recurrence: domain.RecurrenceMonthly,
	}))
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("want AlreadyExists, got %v", connect.CodeOf(err))
	}
}

func TestListConceptsHidesInactiveUnlessAsked(t *testing.T) {
	f := newFixture(t)
	active := f.seedConcept(t, "Colegiatura", 250000)
	retired := f.seedConcept(t, "Inscripción", 500000)
	if _, err := f.h.UpdateConcept(context.Background(), connect.NewRequest(&financev1.UpdateConceptRequest{
		Id: retired.GetId(), Name: retired.GetName(), DefaultAmountCents: retired.GetDefaultAmountCents(),
		Recurrence: retired.GetRecurrence(), Active: false,
	})); err != nil {
		t.Fatalf("retire concept: %v", err)
	}

	visible, err := f.h.ListConcepts(context.Background(), connect.NewRequest(&financev1.ListConceptsRequest{}))
	if err != nil {
		t.Fatalf("ListConcepts: %v", err)
	}
	if len(visible.Msg.GetConcepts()) != 1 || visible.Msg.GetConcepts()[0].GetId() != active.GetId() {
		t.Errorf("default listing = %+v, want only the active concept", visible.Msg.GetConcepts())
	}

	all, err := f.h.ListConcepts(context.Background(), connect.NewRequest(&financev1.ListConceptsRequest{
		IncludeInactive: true,
	}))
	if err != nil {
		t.Fatalf("ListConcepts(includeInactive): %v", err)
	}
	if len(all.Msg.GetConcepts()) != 2 {
		t.Errorf("include_inactive listing = %+v, want both", all.Msg.GetConcepts())
	}
}

func TestCreateChargeDerivesPaidAndRemainingAndFormatsDueDate(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)

	got := f.seedCharge(t, concept.GetId(), 100000)

	if got.GetStudentId() != testStudent || got.GetCycleId() != testCycle || got.GetConceptId() != concept.GetId() {
		t.Errorf("references = %+v", got)
	}
	// The description defaults to the concept's name when not overridden.
	if got.GetDescription() != "Colegiatura" {
		t.Errorf("description = %q, want the concept name", got.GetDescription())
	}
	if got.GetAmountCents() != 100000 || got.GetDiscountCents() != 0 {
		t.Errorf("amount/discount = %+v", got)
	}
	// A fresh charge has collected nothing and owes everything.
	if got.GetPaidCents() != 0 || got.GetRemainingCents() != 100000 {
		t.Errorf("paid/remaining = %d/%d, want 0/100000", got.GetPaidCents(), got.GetRemainingCents())
	}
	if got.GetStatus() != domain.StatusPending {
		t.Errorf("status = %q, want pending", got.GetStatus())
	}
	// The due date crosses the wire as a plain calendar day, never an instant.
	if got.GetDueDate() != testDueDate {
		t.Errorf("due_date = %q, want %q", got.GetDueDate(), testDueDate)
	}
}

func TestCreateChargeUnknownStudentIsInvalidArgument(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)

	_, err := f.h.CreateCharge(context.Background(), connect.NewRequest(&financev1.CreateChargeRequest{
		StudentId: "nope", CycleId: testCycle, ConceptId: concept.GetId(), DueDate: testDueDate,
	}))
	// A dangling cross-service reference is bad input, never a NotFound leak
	// from the students context.
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestCreateChargeBadDueDateIsInvalidArgument(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)

	_, err := f.h.CreateCharge(context.Background(), connect.NewRequest(&financev1.CreateChargeRequest{
		StudentId: testStudent, CycleId: testCycle, ConceptId: concept.GetId(), DueDate: "05/09/2026",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGenerateChargesReportsCreatedThenSkippedOnRerun(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	// Two active enrollments plus one withdrawn: only the active rows are billed.
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{
		{ID: "enr-1", StudentID: "stu-1", GroupID: "grp-1", CycleID: testCycle, Status: "active"},
		{ID: "enr-2", StudentID: "stu-2", GroupID: "grp-1", CycleID: testCycle, Status: "active"},
		{ID: "enr-3", StudentID: "stu-3", GroupID: "grp-1", CycleID: testCycle, Status: "withdrawn"},
	}
	req := func() *connect.Request[financev1.GenerateChargesRequest] {
		return connect.NewRequest(&financev1.GenerateChargesRequest{
			ConceptId: concept.GetId(), CycleId: testCycle, DueDate: testDueDate,
		})
	}

	first, err := f.h.GenerateCharges(context.Background(), req())
	if err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	if first.Msg.GetCreatedCount() != 2 || first.Msg.GetSkippedCount() != 0 {
		t.Errorf("first run = %d created / %d skipped, want 2/0 (the withdrawn row is not billed)",
			first.Msg.GetCreatedCount(), first.Msg.GetSkippedCount())
	}

	// Re-running a generation is idempotent: everyone already owes the concept.
	second, err := f.h.GenerateCharges(context.Background(), req())
	if err != nil {
		t.Fatalf("GenerateCharges (rerun): %v", err)
	}
	if second.Msg.GetCreatedCount() != 0 || second.Msg.GetSkippedCount() != 2 {
		t.Errorf("rerun = %d created / %d skipped, want 0/2",
			second.Msg.GetCreatedCount(), second.Msg.GetSkippedCount())
	}
}

func TestRegisterPaymentReturnsPaymentAndRecomputedCharge(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)

	res, err := f.h.RegisterPayment(context.Background(), connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 40000, Method: domain.MethodCash, Reference: "folio-9",
	}))
	if err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}

	payment := res.Msg.GetPayment()
	if payment.GetChargeId() != charge.GetId() || payment.GetTenantId() != testTenant {
		t.Errorf("payment references = %+v", payment)
	}
	if payment.GetAmountCents() != 40000 || payment.GetMethod() != domain.MethodCash || payment.GetReference() != "folio-9" {
		t.Errorf("payment fields = %+v", payment)
	}
	if !payment.GetPaidAt().AsTime().Equal(fixedNow) {
		t.Errorf("paid_at = %v, want the injected clock %v", payment.GetPaidAt().AsTime(), fixedNow)
	}

	got := res.Msg.GetCharge()
	if got.GetPaidCents() != 40000 || got.GetRemainingCents() != 60000 {
		t.Errorf("paid/remaining = %d/%d, want 40000/60000", got.GetPaidCents(), got.GetRemainingCents())
	}
	if got.GetStatus() != domain.StatusPartiallyPaid {
		t.Errorf("status = %q, want partially_paid", got.GetStatus())
	}
}

// Task 7: who took the money crosses the wire and reaches the ledger.
func TestRegisterPaymentRecordsWhoReceivedIt(t *testing.T) {
	f := newFixture(t)
	charge := f.seedCharge(t, f.seedConcept(t, "Colegiatura", 250000).GetId(), 100000)
	if _, err := f.h.RegisterPayment(context.Background(), connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 40000, Method: domain.MethodCash, ReceivedBy: "c-caja",
	})); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	stored := f.repo.payments[charge.GetId()]
	if len(stored) != 1 || stored[0].ReceivedBy() != "c-caja" {
		t.Errorf("stored payments = %+v, want one received by c-caja", stored)
	}
}

// The idempotency key has to survive the whole wire path: the proto field is
// only useful if the handler threads it into the use case. Sending the same
// request twice must yield the same payment and leave the charge where it was.
func TestRegisterPaymentIdempotencyKeyDedupesTheRetry(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)
	attempt := func() *connect.Request[financev1.RegisterPaymentRequest] {
		return connect.NewRequest(&financev1.RegisterPaymentRequest{
			ChargeId: charge.GetId(), AmountCents: 40000, Method: domain.MethodCash, IdempotencyKey: "attempt-1",
		})
	}

	first, err := f.h.RegisterPayment(context.Background(), attempt())
	if err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	second, err := f.h.RegisterPayment(context.Background(), attempt())
	if err != nil {
		t.Fatalf("RegisterPayment (retry): %v", err)
	}

	if second.Msg.GetPayment().GetId() != first.Msg.GetPayment().GetId() {
		t.Errorf("retry minted payment %q, want the original %q",
			second.Msg.GetPayment().GetId(), first.Msg.GetPayment().GetId())
	}
	got := second.Msg.GetCharge()
	if got.GetPaidCents() != 40000 || got.GetRemainingCents() != 60000 {
		t.Errorf("paid/remaining = %d/%d, want 40000/60000 — the retry must not collect again",
			got.GetPaidCents(), got.GetRemainingCents())
	}
}

func TestRegisterPaymentOverpayIsInvalidArgument(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)

	_, err := f.h.RegisterPayment(context.Background(), connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 100001, Method: domain.MethodCash,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestApplyDiscountFloorsRemainingAtZero(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)
	ctx := context.Background()
	if _, err := f.h.RegisterPayment(ctx, connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 100000, Method: domain.MethodCash,
	})); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	// A later discount leaves the charge collecting more than it now owes.
	res, err := f.h.ApplyDiscount(ctx, connect.NewRequest(&financev1.ApplyDiscountRequest{
		ChargeId: charge.GetId(), DiscountCents: 20000, Reason: "ajuste",
	}))
	if err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	got := res.Msg.GetCharge()
	if got.GetDiscountCents() != 20000 || got.GetPaidCents() != 100000 {
		t.Errorf("discount/paid = %+v", got)
	}
	// An over-collected charge is a credit to reconcile, never a negative debt.
	if got.GetRemainingCents() != 0 {
		t.Errorf("remaining = %d, want 0 (floored)", got.GetRemainingCents())
	}
}

func TestCancelChargeThenPayingItIsInvalidArgument(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)
	ctx := context.Background()

	res, err := f.h.CancelCharge(ctx, connect.NewRequest(&financev1.CancelChargeRequest{ChargeId: charge.GetId()}))
	if err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}
	if res.Msg.GetCharge().GetStatus() != domain.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", res.Msg.GetCharge().GetStatus())
	}

	_, err = f.h.RegisterPayment(ctx, connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 1000, Method: domain.MethodCash,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument paying a cancelled charge, got %v", connect.CodeOf(err))
	}
}

func TestListChargesAppliesTheStudentFilter(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)

	res, err := f.h.ListCharges(context.Background(), connect.NewRequest(&financev1.ListChargesRequest{
		StudentId: testStudent, CycleId: testCycle,
	}))
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	if len(res.Msg.GetCharges()) != 1 || res.Msg.GetCharges()[0].GetId() != charge.GetId() {
		t.Fatalf("charges = %+v, want just %q", res.Msg.GetCharges(), charge.GetId())
	}

	none, err := f.h.ListCharges(context.Background(), connect.NewRequest(&financev1.ListChargesRequest{
		StudentId: "someone-else",
	}))
	if err != nil {
		t.Fatalf("ListCharges(other student): %v", err)
	}
	if len(none.Msg.GetCharges()) != 0 {
		t.Errorf("other student's charges = %+v, want none", none.Msg.GetCharges())
	}
}

func TestGetAccountReturnsChargesPaymentsAndBalance(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.GetId(), 100000)
	ctx := context.Background()
	if _, err := f.h.RegisterPayment(ctx, connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 40000, Method: domain.MethodTransfer, Reference: "spei-1",
	})); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	res, err := f.h.GetAccount(ctx, connect.NewRequest(&financev1.GetAccountRequest{StudentId: testStudent}))
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if len(res.Msg.GetCharges()) != 1 || res.Msg.GetCharges()[0].GetRemainingCents() != 60000 {
		t.Errorf("charges = %+v", res.Msg.GetCharges())
	}
	if len(res.Msg.GetPayments()) != 1 || res.Msg.GetPayments()[0].GetAmountCents() != 40000 {
		t.Errorf("payments = %+v", res.Msg.GetPayments())
	}
	if res.Msg.GetTotalBalanceCents() != 60000 {
		t.Errorf("total_balance = %d, want 60000", res.Msg.GetTotalBalanceCents())
	}
}

func TestListOverdueReturnsThePastDueCharge(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	overdue := f.seedCharge(t, concept.GetId(), 100000) // due 2026-09-05, before fixedNow

	res, err := f.h.ListOverdue(context.Background(), connect.NewRequest(&financev1.ListOverdueRequest{
		CycleId: testCycle,
	}))
	if err != nil {
		t.Fatalf("ListOverdue: %v", err)
	}
	if len(res.Msg.GetCharges()) != 1 || res.Msg.GetCharges()[0].GetId() != overdue.GetId() {
		t.Errorf("overdue = %+v, want just %q", res.Msg.GetCharges(), overdue.GetId())
	}
}

func TestExportContpaqiIsUnimplemented(t *testing.T) {
	f := newFixture(t)
	_, err := f.h.ExportContpaqi(context.Background(), connect.NewRequest(&financev1.ExportContpaqiRequest{
		TenantId: testTenant,
	}))
	// The contract reserves the RPC; answering Unimplemented is what stops a
	// caller mistaking an empty document for a real export.
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("want Unimplemented, got %v", connect.CodeOf(err))
	}
}

// --- tuition doubles (M15) ---------------------------------------------------
//
// The handler's own tests do not exercise billing — that lives in the use case,
// against a fixture that models a school. These exist so the handler compiles
// against the port, and answer with the least a caller could believe.

func (f *fakeRepo) EnsureConceptOfKind(_ context.Context, id, kind, name string) (domain.ChargeConcept, error) {
	c, err := domain.NewChargeConcept(id, testTenant, name, 0, domain.RecurrenceOnce, true)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return c.WithKind(kind)
}

// The fake keeps the plan it was handed. It used to return nil from the list,
// which meant any test reading a plan back was measuring the fake rather than
// the adapter — and the adapter is where fields have twice gone missing.
func (f *fakeRepo) RepriceUnpaid(_ context.Context, _ []ports.ChargeReprice) (int, error) {
	return 0, nil
}

func (f *fakeRepo) RescheduleUnpaid(_ context.Context, _ []ports.ChargeReschedule) (int, error) {
	return 0, nil
}

func (f *fakeRepo) UpsertTuitionPlan(_ context.Context, p domain.TuitionPlan) (domain.TuitionPlan, error) {
	for i, existing := range f.plans {
		if existing.CycleID() == p.CycleID() && existing.LevelID() == p.LevelID() {
			f.plans[i] = p
			return p, nil
		}
	}
	f.plans = append(f.plans, p)
	return p, nil
}

func (f *fakeRepo) ListTuitionPlans(_ context.Context, cycleID string) ([]domain.TuitionPlan, error) {
	var out []domain.TuitionPlan
	for _, p := range f.plans {
		if p.CycleID() == cycleID {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakeCatalog struct{}

func (fakeCatalog) ListLevels(_ context.Context) ([]ports.LevelRef, error) { return nil, nil }

func (fakeCatalog) GroupLevels(_ context.Context, _ string) ([]ports.GroupLevel, error) {
	return nil, nil
}
