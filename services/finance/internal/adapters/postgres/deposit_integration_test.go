//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

const (
	cycleDeposit = "70000000-0000-0000-0000-000000000006"
	cycleVoid    = "70000000-0000-0000-0000-000000000007"
	cycleCredit  = "70000000-0000-0000-0000-000000000008"
)

func newDeposit(t *testing.T, id, tenant, student string, amount int64, at time.Time, key string) domain.Deposit {
	t.Helper()
	d, err := domain.NewDeposit(id, tenant, student, amount, domain.MethodCash, "", at, key, "cal-test")
	if err != nil {
		t.Fatalf("new deposit: %v", err)
	}
	return d
}

// splitOldestFirst is the production rule in the shape the repository wants: fill
// each locked charge in the order it was handed over, and report what is left.
func splitOldestFirst(t *testing.T, tenant string, ids *int, amount int64, at time.Time) func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
	t.Helper()
	return func(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
		remaining := amount
		var payments []domain.Payment
		for _, c := range open {
			owed := c.Charge.Effective().Int64() - c.PaidCents
			if owed <= 0 || remaining <= 0 {
				continue
			}
			take := owed
			if take > remaining {
				take = remaining
			}
			*ids++
			payments = append(payments, newPayment(t, uuidOf(*ids), tenant, c.Charge.ID(), take, at))
			remaining -= take
		}
		return ports.DepositWrite{Allocations: payments, UnallocatedCents: remaining}, nil
	}
}

// uuidOf mints a distinct, valid uuid per allocation without a random source.
func uuidOf(n int) string {
	const digits = "0123456789abcdef"
	return "90000000-0000-0000-0000-0000000000" + string([]byte{digits[n/16%16], digits[n%16]})
}

// The whole point, against real SQL: one deposit, several charges, one
// transaction. The allocations are payments carrying the deposit id, so the
// collected total of each charge still comes from exactly one place.
func TestRegisterDepositSplitsAcrossChargesInOneTransaction(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000001"
	sep := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000011", "colegiatura sep",
		"61000000-0000-0000-0000-000000000011", student, cycleDeposit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	// October carries an external ref: a second charge for the same concept in
	// one cycle is exactly what the monthly run issues, and the ref is what the
	// per-cycle unique index gives way to.
	october := newCharge(t, "61000000-0000-0000-0000-000000000012", tenant, student, cycleDeposit,
		sep.ConceptID(), 320000, 0, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	october, err := october.WithExternalRef("tuition:deposit-test:2026-10")
	if err != nil {
		t.Fatalf("external ref: %v", err)
	}
	oct, err := appRepo(t, tenant).CreateCharge(context.Background(), october)
	if err != nil {
		t.Fatalf("seed october: %v", err)
	}

	repo := appRepo(t, tenant)
	ids := 0
	paidOn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	record, replayed, err := repo.RegisterDeposit(context.Background(),
		newDeposit(t, "62000000-0000-0000-0000-000000000011", tenant, student, 500000, paidOn, "dep-key-1"),
		splitOldestFirst(t, tenant, &ids, 500000, paidOn), nil)
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if replayed {
		t.Fatal("a first deposit reported itself as a replay")
	}
	if len(record.Allocations) != 2 {
		t.Fatalf("allocations = %d, want 2", len(record.Allocations))
	}

	// September is settled and October is part paid, in due-date order.
	after, err := repo.GetCharge(context.Background(), sep.ID())
	if err != nil {
		t.Fatalf("GetCharge(sep): %v", err)
	}
	if after.Charge.Status() != domain.StatusPaid || after.PaidCents != 320000 {
		t.Errorf("september = %q paid %d, want paid 320000", after.Charge.Status(), after.PaidCents)
	}
	afterOct, err := repo.GetCharge(context.Background(), oct.ID())
	if err != nil {
		t.Fatalf("GetCharge(oct): %v", err)
	}
	if afterOct.Charge.Status() != domain.StatusPartiallyPaid || afterOct.PaidCents != 180000 {
		t.Errorf("october = %q paid %d, want partially_paid 180000", afterOct.Charge.Status(), afterOct.PaidCents)
	}
	if len(record.Settled) != 1 || record.Settled[0].Charge.ID() != sep.ID() {
		t.Errorf("settled = %+v, want september alone", record.Settled)
	}
}

// The retry a lost response provokes must not collect twice — enforced against
// the real partial unique index, not just in the application.
func TestRegisterDepositIsIdempotent(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000002"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000021", "colegiatura idem",
		"61000000-0000-0000-0000-000000000021", student, cycleDeposit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	ids := 100
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-000000000021", tenant, student, 320000, paidOn, "dep-key-2")
	if _, _, err := repo.RegisterDeposit(context.Background(), deposit,
		splitOldestFirst(t, tenant, &ids, 320000, paidOn), nil); err != nil {
		t.Fatalf("first RegisterDeposit: %v", err)
	}

	second := newDeposit(t, "62000000-0000-0000-0000-000000000022", tenant, student, 320000, paidOn, "dep-key-2")
	record, replayed, err := repo.RegisterDeposit(context.Background(), second,
		splitOldestFirst(t, tenant, &ids, 320000, paidOn), nil)
	if err != nil {
		t.Fatalf("second RegisterDeposit: %v", err)
	}
	if !replayed || record.Deposit.ID() != deposit.ID() {
		t.Errorf("replay returned %s (replayed=%v), want the original %s", record.Deposit.ID(), replayed, deposit.ID())
	}
	if n := countPayments(t, tenant, charge.ID()); n != 1 {
		t.Errorf("payments against the charge = %d, want 1 — the money was collected twice", n)
	}
}

// A cashier who keys the wrong amount undoes it, and the debt comes back. The
// deposit row survives with its reason: a school that deletes money rows cannot
// explain its own ledger.
func TestVoidDepositReleasesTheDebtAndKeepsTheRow(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000003"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000031", "colegiatura void",
		"61000000-0000-0000-0000-000000000031", student, cycleVoid, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	ids := 200
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-000000000031", tenant, student, 320000, paidOn, "dep-key-3")
	if _, _, err := repo.RegisterDeposit(context.Background(), deposit,
		splitOldestFirst(t, tenant, &ids, 320000, paidOn), nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	voided, err := deposit.Void("importe capturado mal", paidOn.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.VoidDeposit(context.Background(), voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	after, err := repo.GetCharge(context.Background(), charge.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if after.Charge.Status() != domain.StatusPending || after.PaidCents != 0 {
		t.Errorf("charge = %q paid %d after the void, want pending 0", after.Charge.Status(), after.PaidCents)
	}
	stored, err := repo.GetDeposit(context.Background(), deposit.ID())
	if err != nil {
		t.Fatalf("GetDeposit: %v", err)
	}
	if !stored.Voided() || stored.VoidReason() != "importe capturado mal" {
		t.Errorf("stored deposit did not keep the cancellation: %+v", stored)
	}

	// Voiding twice is a second cashier on a stale screen: refused, not silent.
	if err := repo.VoidDeposit(context.Background(), voided); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second void: want KindConflict, got %v", err)
	}
}

// Money beyond the debt is the family's credit, and the statement has to be able
// to say so. This is the two-sum query, run against real SQL.
func TestUnallocatedCentsIsTheFamilysCredit(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000004"
	seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000041", "colegiatura credit",
		"61000000-0000-0000-0000-000000000041", student, cycleCredit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	ids := 300
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	if _, _, err := repo.RegisterDeposit(context.Background(),
		newDeposit(t, "62000000-0000-0000-0000-000000000041", tenant, student, 500000, paidOn, "dep-key-4"),
		splitOldestFirst(t, tenant, &ids, 500000, paidOn), nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	credit, err := repo.UnallocatedCents(context.Background(), student)
	if err != nil {
		t.Fatalf("UnallocatedCents: %v", err)
	}
	if credit != 180000 {
		t.Errorf("credit = %d, want 180000", credit)
	}
}

// RLS, same posture as every other table here: another school cannot see a
// family's deposits, and cannot cancel one either.
func TestRLSIsolatesDeposits(t *testing.T) {
	student := "80000000-0000-0000-0000-000000000005"
	seedConceptAndCharge(t, tenantA, "60000000-0000-0000-0000-000000000051", "colegiatura rls",
		"61000000-0000-0000-0000-000000000051", student, cycleDeposit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	ids := 400
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-000000000051", tenantA, student, 320000, paidOn, "dep-key-5")
	if _, _, err := appRepo(t, tenantA).RegisterDeposit(context.Background(), deposit,
		splitOldestFirst(t, tenantA, &ids, 320000, paidOn), nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	other := appRepo(t, tenantB)
	if _, err := other.GetDeposit(context.Background(), deposit.ID()); !isNotFound(err) {
		t.Errorf("tenant B read tenant A's deposit: %v", err)
	}
	rows, err := other.ListDeposits(context.Background(), student)
	if err != nil {
		t.Fatalf("ListDeposits as tenant B: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("tenant B listed %d of tenant A's deposits", len(rows))
	}
	voided, err := deposit.Void("no es suyo", paidOn)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := other.VoidDeposit(context.Background(), voided); err == nil {
		t.Error("tenant B cancelled tenant A's deposit")
	}
	if credit, cerr := other.UnallocatedCents(context.Background(), student); cerr != nil || credit != 0 {
		t.Errorf("tenant B saw a credit of %d (err %v)", credit, cerr)
	}
}

// The income query is a grouped join over three tables. It is the kind of thing
// that works against a fake and is wrong against SQL, so it is proven here.
func TestIncomeLinesGroupsCollectedMoneyByConcept(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000006"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000061", "colegiatura income",
		"61000000-0000-0000-0000-000000000061", student, cycleCredit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	paidOn := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if _, _, _, err := repo.RegisterPayment(context.Background(),
		newPayment(t, "91000000-0000-0000-0000-000000000061", tenant, charge.ID(), 200000, paidOn), derive, nil); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}

	lines, err := repo.IncomeLines(context.Background(),
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeLines: %v", err)
	}
	var got int64
	for _, l := range lines {
		if l.ConceptID == charge.ConceptID() {
			got = l.AmountCents
			if l.Concept != "colegiatura income" {
				t.Errorf("concept name = %q, want the catalog name", l.Concept)
			}
		}
	}
	if got != 200000 {
		t.Errorf("income for the concept = %d, want 200000", got)
	}

	// September is another month's income. Counting it twice is how a school
	// convinces itself it had a good October.
	before, err := repo.IncomeLines(context.Background(),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeLines(September): %v", err)
	}
	for _, l := range before {
		if l.ConceptID == charge.ConceptID() {
			t.Errorf("September counted %d of October's money", l.AmountCents)
		}
	}
}

// Re-pricing a level must change everything the request said, not the columns
// somebody remembered to list. The upsert used to update seven of thirteen: an
// insert stored the daily surcharge and the split schedule, an update dropped
// them on the floor, and the same request did two different things depending on
// whether that level had been priced before.
func TestRepricingALevelUpdatesEveryColumn(t *testing.T) {
	tenant := tenantA
	repo := appRepo(t, tenant)
	ctx := context.Background()
	const cycleID = "70000000-0000-0000-0000-000000000099"
	const levelID = "71000000-0000-0000-0000-000000000099"

	plain, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "72000000-0000-0000-0000-000000000001", TenantID: tenant, CycleID: cycleID, LevelID: levelID,
		EnrollmentCents: 500000, MonthlyCents: 320000, Months: 10,
		FirstBilledMonth: 9, DueDay: 10, Active: true,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	if _, err := repo.UpsertTuitionPlan(ctx, plain); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Same level, now on the daily rule with a split year.
	changed, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "72000000-0000-0000-0000-000000000002", TenantID: tenant, CycleID: cycleID, LevelID: levelID,
		EnrollmentCents: 500000, MonthlyCents: 320000, Months: 10,
		FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000,
			SurchargeCountsAllDays: true, EarlyPaymentCents: 10000, EarlyPaymentDays: 5,
		},
		Schedule: []domain.Installment{
			{Month: 9, AmountCents: 320000},
			{Month: 12, AmountCents: 160000},
			{Month: 3, AmountCents: 480000},
		},
		Fees: []domain.Fee{
			{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}},
		},
		EnrollmentDueOn:    time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC),
		EnrollmentStartsOn: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	stored, err := repo.UpsertTuitionPlan(ctx, changed)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if !stored.Policy().DailyMode || stored.Policy().LateFeeDailyCents != 1000 {
		t.Errorf("the collection rule did not survive the update: %+v", stored.Policy())
	}
	if stored.Policy().EarlyPaymentCents != 10000 || stored.Policy().EarlyPaymentDays != 5 {
		t.Errorf("the early-payment reward did not survive: %+v", stored.Policy())
	}
	if len(stored.Schedule()) != 3 || stored.Schedule()[1].AmountCents != 160000 {
		t.Errorf("the split schedule did not survive: %+v", stored.Schedule())
	}
	if len(stored.Fees()) != 1 || stored.Fees()[0].AmountCents != 160000 || len(stored.Fees()[0].Months) != 2 {
		t.Errorf("the level fees did not survive the update: %+v", stored.Fees())
	}
	if got := stored.EnrollmentDueOn().Format("2006-01-02"); got != "2026-06-30" {
		t.Errorf("the inscription deadline did not survive the update: %s", got)
	}
	if got := stored.EnrollmentStartsOn().Format("2006-01-02"); got != "2026-06-01" {
		t.Errorf("the inscription start did not survive the update (T5): %s", got)
	}
}

// The whole point of the outbox, against real SQL: the event is written in the
// SAME transaction as the money, so a broker that is down costs a delay and not
// a fact.
//
// Before this, a payment committed and then published best-effort. Lose that
// send — broker down, container dies in between — and students never hears
// ChargeSettled, so a family that paid keeps a pupil "en trámite" forever with
// nothing able to notice.
func TestTheOutboxSurvivesABrokenBroker(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000010"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000071", "inscripción outbox",
		"61000000-0000-0000-0000-000000000071", student, cycleDeposit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	ids := 500
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	_, _, err := repo.RegisterDeposit(context.Background(),
		newDeposit(t, "62000000-0000-0000-0000-000000000071", tenant, student, 320000, paidOn, "dep-key-outbox"),
		splitOldestFirst(t, tenant, &ids, 320000, paidOn),
		func(record ports.DepositRecord) []ports.OutboxEvent {
			events := []ports.OutboxEvent{{Type: ports.OutboxPaymentRegistered, Payload: []byte(`{"paymentId":"dep"}`)}}
			for range record.Settled {
				events = append(events, ports.OutboxEvent{
					Type: ports.OutboxChargeSettled, Payload: []byte(`{"chargeId":"` + charge.ID() + `"}`),
				})
			}
			return events
		})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	// The broker is down: every publication is refused.
	broken := func(ports.OutboxEvent) error { return errors.New("pub/sub is down") }
	published, err := repo.DrainOutbox(context.Background(), 50, broken)
	if err != nil {
		t.Fatalf("DrainOutbox with a broken broker: %v", err)
	}
	if published != 0 {
		t.Errorf("published %d events through a broken broker", published)
	}

	// And the events are still there, which is the whole claim.
	var seen []ports.OutboxEvent
	published, err = repo.DrainOutbox(context.Background(), 50, func(e ports.OutboxEvent) error {
		seen = append(seen, e)
		return nil
	})
	if err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
	if published != 2 || len(seen) != 2 {
		t.Fatalf("published %d events after the broker came back, want 2", published)
	}
	var settled int
	for _, e := range seen {
		if e.Type == ports.OutboxChargeSettled {
			settled++
		}
	}
	if settled != 1 {
		t.Errorf("%d ChargeSettled events survived, want 1", settled)
	}

	// A second drain has nothing left: published rows are not sent twice.
	published, err = repo.DrainOutbox(context.Background(), 50, func(ports.OutboxEvent) error { return nil })
	if err != nil {
		t.Fatalf("second DrainOutbox: %v", err)
	}
	if published != 0 {
		t.Errorf("re-published %d events that had already gone out", published)
	}
}

// A payment that fails leaves no event behind. A consumer acting on a payment
// that never happened is worse than a lost notification.
func TestAnAbortedPaymentQueuesNoEvent(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-000000000011"
	seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-000000000081", "inscripción abort",
		"61000000-0000-0000-0000-000000000081", student, cycleDeposit, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	repo := appRepo(t, tenant)
	paidOn := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	_, _, err := repo.RegisterDeposit(context.Background(),
		newDeposit(t, "62000000-0000-0000-0000-000000000081", tenant, student, 320000, paidOn, "dep-key-abort"),
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{}, errors.New("the split could not be worked out")
		},
		func(ports.DepositRecord) []ports.OutboxEvent {
			return []ports.OutboxEvent{{Type: ports.OutboxChargeSettled, Payload: []byte(`{}`)}}
		})
	if err == nil {
		t.Fatal("the deposit was accepted")
	}

	published, derr := repo.DrainOutbox(context.Background(), 50, func(ports.OutboxEvent) error { return nil })
	if derr != nil {
		t.Fatalf("DrainOutbox: %v", derr)
	}
	if published != 0 {
		t.Errorf("%d events queued for a payment that never happened", published)
	}
}

// Task 7: who took the money is stored with the deposit (account and name) and
// with each payment it split into; the cash-up reads it back with the moment
// the deposit was keyed.
func TestADepositKeepsWhoReceivedIt(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000a7"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000a7", "colegiatura recibe",
		"61000000-0000-0000-0000-0000000000a7", student, "70000000-0000-0000-0000-0000000000a7", 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	const cashier = "c0000000-0000-0000-0000-0000000000a7"
	paidOn := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-0000000000a7", tenant, student, 320000, paidOn, "").
		WithReceiver(domain.Receiver{UserID: cashier, Name: "Laura Caja"})
	payment := newPayment(t, "90000000-0000-0000-0000-0000000000a7", tenant, charge.ID(), 320000, paidOn).WithReceivedBy(cashier)

	repo := appRepo(t, tenant)
	if _, _, err := repo.RegisterDeposit(context.Background(), deposit,
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Allocations: []domain.Payment{payment}}, nil
		}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	day, err := repo.ListDepositsOn(context.Background(), paidOn)
	if err != nil {
		t.Fatalf("ListDepositsOn: %v", err)
	}
	var got domain.Deposit
	for _, d := range day {
		if d.ID() == deposit.ID() {
			got = d
		}
	}
	if got.ReceivedBy() != (domain.Receiver{UserID: cashier, Name: "Laura Caja"}) || got.CapturedAt().IsZero() {
		t.Errorf("stored deposit = %+v captured %v", got.ReceivedBy(), got.CapturedAt())
	}
	var receivedBy string
	if err := superDB.Raw(`SELECT coalesce(received_by::text, '') FROM payments WHERE id = ?`, payment.ID()).Scan(&receivedBy).Error; err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if receivedBy != cashier {
		t.Errorf("payment received_by = %q, want %q", receivedBy, cashier)
	}
}

// Task 7: the price-at-payment-date discount exists only because this deposit
// settled the line. Voiding the deposit takes the discount back with it —
// otherwise a voided deposit leaves the inscription permanently cheaper.
func TestVoidingADepositTakesBackItsPriceAtPaymentDateDiscount(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000b7"
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000b7", "inscripcion precio fecha",
		"61000000-0000-0000-0000-0000000000b7", student, "70000000-0000-0000-0000-0000000000b7", 330000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	discounted, err := charge.WithDiscount(110000, 0, domain.ReasonPriceAtPaymentDate)
	if err != nil {
		t.Fatalf("WithDiscount: %v", err)
	}
	paidOn := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-0000000000b7", tenant, student, 220000, paidOn, "")
	payment := newPayment(t, "90000000-0000-0000-0000-0000000000b7", tenant, charge.ID(), 220000, paidOn)

	repo := appRepo(t, tenant)
	if _, _, err := repo.RegisterDeposit(context.Background(), deposit,
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Discounted: []domain.Charge{discounted}, Allocations: []domain.Payment{payment}}, nil
		}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	voided, err := deposit.Void("capturado mal", paidOn.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.VoidDeposit(context.Background(), voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	after, err := repo.GetCharge(context.Background(), charge.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if after.Charge.Discount().Int64() != 0 || after.Charge.DiscountReason() != "" || after.Charge.Status() != domain.StatusPending {
		t.Errorf("after the void: discount %d (%q), %s; want no discount, pending",
			after.Charge.Discount().Int64(), after.Charge.DiscountReason(), after.Charge.Status())
	}
}

// Task 9: a deposit that settles a month writes its daily surcharge, paid or
// not. That row exists only because of the deposit, so voiding it cancels the
// row when nothing else paid it — or a void and a re-key dated earlier would
// leave the family owing the days of the wrong date.
func TestVoidingADepositCancelsTheSurchargeItWrote(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000b8"
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	month := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000b8", "colegiatura recargo void",
		"61000000-0000-0000-0000-0000000000b8", student, "70000000-0000-0000-0000-0000000000b8", 320000, 0, due)
	paidOn := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-0000000000b8", tenant, student, 320000, paidOn, "")
	fee := newCharge(t, "63000000-0000-0000-0000-0000000000b8", tenant, student,
		"70000000-0000-0000-0000-0000000000b8", "60000000-0000-0000-0000-0000000000b8", 40000, 0, due)
	fee, err := fee.WithExternalRef("dailyfee:" + month.ID() + ":" + deposit.ID())
	if err != nil {
		t.Fatalf("WithExternalRef: %v", err)
	}
	payment := newPayment(t, "90000000-0000-0000-0000-0000000000b8", tenant, month.ID(), 320000, paidOn)

	repo := appRepo(t, tenant)
	if _, _, err := repo.RegisterDeposit(context.Background(), deposit,
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Surcharges: []domain.Charge{fee}, Allocations: []domain.Payment{payment}}, nil
		}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	voided, err := deposit.Void("fecha equivocada", paidOn)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.VoidDeposit(context.Background(), voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	after, err := repo.GetCharge(context.Background(), fee.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if after.Charge.Status() != domain.StatusCancelled {
		t.Errorf("the voided deposit's surcharge is %s, want cancelled", after.Charge.Status())
	}
	reopened, err := repo.GetCharge(context.Background(), month.ID())
	if err != nil {
		t.Fatalf("GetCharge month: %v", err)
	}
	if reopened.Charge.Status() != domain.StatusPending {
		t.Errorf("month after the void is %s, want pending", reopened.Charge.Status())
	}
}

// Task 12: every deposit gets a folio the receipt prints — P-<year>-<six
// digits>, one run per school and year, allocated in the deposit's own
// transaction. The year is the capture's, as created_at records it.
func TestDepositsGetAFolioPerSchool(t *testing.T) {
	ctx := context.Background()
	empty := func([]ports.ChargeWithPaid) (ports.DepositWrite, error) { return ports.DepositWrite{}, nil }
	paidOn := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

	repoA := appRepo(t, tenantA)
	first, _, err := repoA.RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000000c1", tenantA,
		"80000000-0000-0000-0000-0000000000c1", 100, paidOn, ""), empty, nil)
	if err != nil {
		t.Fatalf("first deposit: %v", err)
	}
	second, _, err := repoA.RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000000c2", tenantA,
		"80000000-0000-0000-0000-0000000000c1", 100, paidOn, ""), empty, nil)
	if err != nil {
		t.Fatalf("second deposit: %v", err)
	}
	f1, f2 := first.Deposit.Folio(), second.Deposit.Folio()
	if !regexp.MustCompile(`^P-[0-9]{4}-[0-9]{6}$`).MatchString(f1) {
		t.Fatalf("folio %q is not P-YYYY-NNNNNN", f1)
	}
	if seqOf(t, f2) != seqOf(t, f1)+1 || f1[:6] != f2[:6] {
		t.Errorf("folios %q then %q; want the next number of the same year", f1, f2)
	}
	stored, err := repoA.GetDeposit(ctx, first.Deposit.ID())
	if err != nil {
		t.Fatalf("GetDeposit: %v", err)
	}
	if stored.Folio() != f1 {
		t.Errorf("read back %q, want %q", stored.Folio(), f1)
	}
	if year := fmt.Sprintf("P-%04d-", stored.CapturedAt().UTC().Year()); f1[:7] != year {
		t.Errorf("folio %q names another year than its capture (%s)", f1, stored.CapturedAt())
	}

	// Another school keeps its own run: its first deposit is number 1.
	other, _, err := appRepo(t, tenantB).RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000000c3", tenantB,
		"80000000-0000-0000-0000-0000000000c3", 100, paidOn, ""), empty, nil)
	if err != nil {
		t.Fatalf("tenant B deposit: %v", err)
	}
	if seqOf(t, other.Deposit.Folio()) != 1 {
		t.Errorf("tenant B's first folio = %q, want number 1 of its own run", other.Deposit.Folio())
	}
}

func seqOf(t *testing.T, folio string) int {
	t.Helper()
	n, err := strconv.Atoi(folio[len(folio)-6:])
	if err != nil {
		t.Fatalf("folio %q: %v", folio, err)
	}
	return n
}

// Task 12: a deposit read back for its receipt — what it put on each charge, the
// late fee marked as such, the discount beside its month, and the credit left.
func TestDepositReceiptReadsWhatTheMoneyPaid(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000d1"
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	month := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000d1", "colegiatura recibo",
		"61000000-0000-0000-0000-0000000000d1", student, "70000000-0000-0000-0000-0000000000d1", 320000, 10000, due)
	fee := newCharge(t, "63000000-0000-0000-0000-0000000000d1", tenant, student,
		"70000000-0000-0000-0000-0000000000d1", "60000000-0000-0000-0000-0000000000d1", 30000, 0, due)
	paidOn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	deposit := newDeposit(t, "62000000-0000-0000-0000-0000000000d1", tenant, student, 350000, paidOn, "")
	fee, err := fee.WithExternalRef("dailyfee:" + month.ID() + ":" + deposit.ID())
	if err != nil {
		t.Fatalf("WithExternalRef: %v", err)
	}
	repo := appRepo(t, tenant)
	if _, _, err := repo.RegisterDeposit(ctx, deposit, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{
			Surcharges: []domain.Charge{fee},
			Allocations: []domain.Payment{
				newPayment(t, "90000000-0000-0000-0000-0000000000d1", tenant, month.ID(), 310000, paidOn),
				newPayment(t, "90000000-0000-0000-0000-0000000000d2", tenant, fee.ID(), 30000, paidOn),
			},
			UnallocatedCents: 10000,
		}, nil
	}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	got, err := repo.DepositReceipt(ctx, deposit.ID())
	if err != nil {
		t.Fatalf("DepositReceipt: %v", err)
	}
	if got.Deposit.Folio() == "" || len(got.Lines) != 2 || got.UnallocatedCents != 10000 {
		t.Fatalf("receipt = folio %q, %d lines, credit %d; want a folio, 2 lines, 10000", got.Deposit.Folio(), len(got.Lines), got.UnallocatedCents)
	}
	byCharge := map[string]ports.ReceiptLine{}
	for _, l := range got.Lines {
		byCharge[l.ChargeID] = l
	}
	if l := byCharge[month.ID()]; l.AmountCents != 310000 || l.Surcharge || l.DiscountCents != 10000 || l.Description == "" {
		t.Errorf("month line = %+v; want 310000, not a surcharge, 10000 discount, its description", l)
	}
	if l := byCharge[fee.ID()]; l.AmountCents != 30000 || !l.Surcharge {
		t.Errorf("fee line = %+v; want 30000 marked as a surcharge", l)
	}
}

// Review of task 12: a charge's discount is realized by the payment that
// settles it, so only that deposit's receipt shows it. Paid in two deposits, the
// family must not read the discount twice. A voided deposit has no lines and no
// credit.
func TestDepositReceiptShowsTheDiscountOnTheSettlingDepositOnly(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000e1"
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000e1", "colegiatura en dos",
		"61000000-0000-0000-0000-0000000000e1", student, "70000000-0000-0000-0000-0000000000e1", 320000, 10000, due)
	paidOn := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	repo := appRepo(t, tenant)
	pay := func(depositID, paymentID string, amount int64) domain.Deposit {
		d := newDeposit(t, depositID, tenant, student, amount, paidOn, "")
		if _, _, err := repo.RegisterDeposit(ctx, d, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Allocations: []domain.Payment{newPayment(t, paymentID, tenant, charge.ID(), amount, paidOn)}}, nil
		}, nil); err != nil {
			t.Fatalf("RegisterDeposit %s: %v", depositID, err)
		}
		return d
	}
	first := pay("62000000-0000-0000-0000-0000000000e1", "90000000-0000-0000-0000-0000000000e1", 200000)
	second := pay("62000000-0000-0000-0000-0000000000e2", "90000000-0000-0000-0000-0000000000e2", 110000)

	a, err := repo.DepositReceipt(ctx, first.ID())
	if err != nil {
		t.Fatalf("receipt A: %v", err)
	}
	b, err := repo.DepositReceipt(ctx, second.ID())
	if err != nil {
		t.Fatalf("receipt B: %v", err)
	}
	if len(a.Lines) != 1 || a.Lines[0].DiscountCents != 0 {
		t.Errorf("the part payment's line = %+v, want no discount", a.Lines)
	}
	if len(b.Lines) != 1 || b.Lines[0].DiscountCents != 10000 {
		t.Errorf("the settling payment's line = %+v, want the 10000 discount", b.Lines)
	}

	voided, err := first.Void("capturado mal", paidOn)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.VoidDeposit(ctx, voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}
	gone, err := repo.DepositReceipt(ctx, first.ID())
	if err != nil {
		t.Fatalf("voided receipt: %v", err)
	}
	if !gone.Deposit.Voided() || len(gone.Lines) != 0 || gone.UnallocatedCents != 0 {
		t.Errorf("voided receipt = voided %v, %d lines, credit %d; want voided, none, 0", gone.Deposit.Voided(), len(gone.Lines), gone.UnallocatedCents)
	}
}

// Review of task 12: a retried deposit returns the folio it was given, and the
// counter does not move.
func TestAReplayedDepositKeepsItsFolio(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	empty := func([]ports.ChargeWithPaid) (ports.DepositWrite, error) { return ports.DepositWrite{}, nil }
	paidOn := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	d := newDeposit(t, "62000000-0000-0000-0000-0000000000e3", tenantA, "80000000-0000-0000-0000-0000000000e3", 100, paidOn, "replay-folio-key")

	first, _, err := repo.RegisterDeposit(ctx, d, empty, nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	retry := newDeposit(t, "62000000-0000-0000-0000-0000000000e4", tenantA, "80000000-0000-0000-0000-0000000000e3", 100, paidOn, "replay-folio-key")
	again, replayed, err := repo.RegisterDeposit(ctx, retry, empty, nil)
	if err != nil || !replayed {
		t.Fatalf("retry: replayed %v, err %v", replayed, err)
	}
	if again.Deposit.Folio() != first.Deposit.Folio() {
		t.Errorf("retry folio %q, want the original %q", again.Deposit.Folio(), first.Deposit.Folio())
	}
	next, _, err := repo.RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000000e5", tenantA,
		"80000000-0000-0000-0000-0000000000e3", 100, paidOn, ""), empty, nil)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if seqOf(t, next.Deposit.Folio()) != seqOf(t, first.Deposit.Folio())+1 {
		t.Errorf("after a replay the next folio is %q; want the number right after %q", next.Deposit.Folio(), first.Deposit.Folio())
	}
}

// Review of task 12: deposits taken at the same time get distinct numbers with
// no gap.
func TestConcurrentDepositsGetDistinctConsecutiveFolios(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantB)
	empty := func([]ports.ChargeWithPaid) (ports.DepositWrite, error) { return ports.DepositWrite{}, nil }
	paidOn := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	const n = 8
	// Built here, on the test's goroutine: t.Fatalf is not for the workers.
	deposits := make([]domain.Deposit, n)
	for i := range deposits {
		deposits[i] = newDeposit(t, fmt.Sprintf("62000000-0000-0000-0000-0000000001%02d", i), tenantB, "80000000-0000-0000-0000-0000000000f1", 100, paidOn, "")
	}
	folios := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, _, err := repo.RegisterDeposit(ctx, deposits[i], empty, nil)
			if err != nil {
				errs <- err
				return
			}
			folios <- got.Deposit.Folio()
		}(i)
	}
	wg.Wait()
	close(folios)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent deposit: %v", err)
	}
	seqs := []int{}
	for f := range folios {
		seqs = append(seqs, seqOf(t, f))
	}
	sort.Ints(seqs)
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("folio numbers %v are not distinct and consecutive", seqs)
		}
	}
}

// Review of task 12: a counter behind its folios is a fault, not "this payment
// already exists" — that answer would send the cashier away with money taken.
func TestAFolioClashIsNotADuplicatePayment(t *testing.T) {
	ctx := context.Background()
	const tenant = "33333333-3333-3333-3333-333333333333"
	repo := appRepo(t, tenant)
	empty := func([]ports.ChargeWithPaid) (ports.DepositWrite, error) { return ports.DepositWrite{}, nil }
	paidOn := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	if _, _, err := repo.RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000002a1", tenant,
		"80000000-0000-0000-0000-0000000002a1", 100, paidOn, ""), empty, nil); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := superDB.Exec(`UPDATE deposit_folio_counters SET next_seq = 1 WHERE tenant_id = ?`, tenant).Error; err != nil {
		t.Fatalf("rolling the counter back: %v", err)
	}

	_, _, err := repo.RegisterDeposit(ctx, newDeposit(t, "62000000-0000-0000-0000-0000000002a2", tenant,
		"80000000-0000-0000-0000-0000000002a1", 100, paidOn, ""), empty, nil)
	if kind := glerrors.KindOf(err); kind != glerrors.KindInternal {
		t.Errorf("a folio clash answered %v (%v); want an internal fault, not a conflict", kind, err)
	}
}

// QA 2026-09-25, ISSUE-010: the cash-up reads the day's payments taken outside
// a deposit (a counter sale's), with what each paid for, and never a slice of a
// deposit, which the cut already counts through the deposit itself.
func TestListLoosePaymentsOnReadsOnlyPaymentsOutsideADeposit(t *testing.T) {
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000c7"
	loose := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000c7", "uniformes suelto",
		"61000000-0000-0000-0000-0000000000c7", student, "70000000-0000-0000-0000-0000000000c7", 135000, 0,
		time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	sliced := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000c8", "colegiatura suelto",
		"61000000-0000-0000-0000-0000000000c8", student, "70000000-0000-0000-0000-0000000000c7", 250000, 0,
		time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	paidOn := time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)

	repo := appRepo(t, tenant)
	if _, _, _, err := repo.RegisterPayment(context.Background(),
		newPayment(t, "91000000-0000-0000-0000-0000000000c7", tenant, loose.ID(), 135000, paidOn), derive, nil); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	dep := newDeposit(t, "62000000-0000-0000-0000-0000000000c8", tenant, student, 250000, paidOn, "")
	slice := newPayment(t, "91000000-0000-0000-0000-0000000000c8", tenant, sliced.ID(), 250000, paidOn)
	if _, _, err := repo.RegisterDeposit(context.Background(), dep,
		func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Allocations: []domain.Payment{slice}}, nil
		}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	got, err := repo.ListLoosePaymentsOn(context.Background(), time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListLoosePaymentsOn: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d loose payments, want 1 (the deposit's slice is not one)", len(got))
	}
	if got[0].Payment.ID() != "91000000-0000-0000-0000-0000000000c7" || got[0].Payment.Amount().Int64() != 135000 {
		t.Errorf("loose payment = %s %d", got[0].Payment.ID(), got[0].Payment.Amount().Int64())
	}
	if got[0].Description != loose.Description() {
		t.Errorf("description = %q, want the charge's %q", got[0].Description, loose.Description())
	}
}
