//go:build integration

package postgres_test

import (
	"context"
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
		splitOldestFirst(t, tenant, &ids, 500000, paidOn))
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
		splitOldestFirst(t, tenant, &ids, 320000, paidOn)); err != nil {
		t.Fatalf("first RegisterDeposit: %v", err)
	}

	second := newDeposit(t, "62000000-0000-0000-0000-000000000022", tenant, student, 320000, paidOn, "dep-key-2")
	record, replayed, err := repo.RegisterDeposit(context.Background(), second,
		splitOldestFirst(t, tenant, &ids, 320000, paidOn))
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
		splitOldestFirst(t, tenant, &ids, 320000, paidOn)); err != nil {
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
		splitOldestFirst(t, tenant, &ids, 500000, paidOn)); err != nil {
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
		splitOldestFirst(t, tenantA, &ids, 320000, paidOn)); err != nil {
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
