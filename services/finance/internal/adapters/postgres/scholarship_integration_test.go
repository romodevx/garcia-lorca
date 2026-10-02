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

// T18 against real SQL: scholarships, their holders, the part written on a
// charge settled under one, the receipt that prints it, and RLS. Every id here
// starts with "5c" so no other test of the shared container collides.

const (
	scHalf     = "5c000000-0000-0000-0000-000000000001"
	scFree     = "5c000000-0000-0000-0000-000000000002"
	scOther    = "5c000000-0000-0000-0000-000000000003"
	scStudent1 = "5c000000-0000-0000-0000-00000000005a"
	scStudent2 = "5c000000-0000-0000-0000-00000000005b"
	scCycle    = "5c000000-0000-0000-0000-0000000000c1"
)

func mustScholarship(t *testing.T, id, tenant, name, kind string, value int64, active bool, targets ...string) domain.Scholarship {
	t.Helper()
	s, err := domain.NewScholarship(domain.ScholarshipSpec{
		ID: id, TenantID: tenant, Name: name, Kind: kind, Value: value, Targets: targets, Active: active,
	})
	if err != nil {
		t.Fatalf("NewScholarship: %v", err)
	}
	return s
}

func holding(t *testing.T, id, tenant, scholarship, student string) domain.ScholarshipHolding {
	t.Helper()
	h, err := domain.NewScholarshipHolding(id, tenant, scholarship, student, time.Date(2031, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestScholarshipsRoundTripUpdateTwiceAndRLS(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateScholarship(ctx, mustScholarship(t, scHalf, tenantA, "Hermanos", domain.ScholarshipPercent, 5000, true,
		domain.TargetTuition, domain.TargetEnrollment)); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Edit twice on the same key (checklist #1), every field.
	edited := mustScholarship(t, scHalf, tenantA, "Hermanos 2", domain.ScholarshipFixedPrice, 120000, false,
		domain.ActivityTarget("5c000000-0000-0000-0000-0000000000a1"))
	for i := 0; i < 2; i++ {
		if _, err := repo.UpdateScholarship(ctx, edited); err != nil {
			t.Fatalf("update #%d: %v", i+1, err)
		}
	}
	got, err := repo.GetScholarship(ctx, scHalf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "Hermanos 2" || got.Kind() != domain.ScholarshipFixedPrice || got.Value() != 120000 || got.Active() ||
		len(got.Targets()) != 1 || got.Targets()[0] != domain.ActivityTarget("5c000000-0000-0000-0000-0000000000a1") {
		t.Fatalf("update lost fields: %+v", got)
	}
	active, err := repo.ListScholarships(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range active {
		if s.ID() == scHalf {
			t.Fatal("an inactive scholarship is listed only when asked")
		}
	}

	_, err = repo.CreateScholarship(ctx, mustScholarship(t, scOther, tenantA, " hermanos 2", domain.ScholarshipPercent, 1, true, domain.TargetTuition))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("same name: want KindConflict, got %v", err)
	}
	if _, err := appRepo(t, tenantB).GetScholarship(ctx, scHalf); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("another school must not read it: %v", err)
	}
	if _, err := appRepo(t, tenantB).CreateScholarship(ctx, mustScholarship(t, scOther, tenantB, "Hermanos 2", domain.ScholarshipPercent, 1, true, domain.TargetTuition)); err != nil {
		t.Fatalf("another school may use the name: %v", err)
	}
}

func TestScholarshipHoldings(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateScholarship(ctx, mustScholarship(t, scFree, tenantA, "Completa", domain.ScholarshipFixedPrice, 0, true, domain.TargetTuition)); err != nil {
		t.Fatal(err)
	}
	first := []domain.ScholarshipHolding{
		holding(t, "5c000000-0000-0000-0000-0000000000b1", tenantA, scFree, scStudent1),
		holding(t, "5c000000-0000-0000-0000-0000000000b2", tenantA, scFree, scStudent2),
	}
	written, err := repo.Assign(ctx, first)
	if err != nil || len(written) != 2 {
		t.Fatalf("assign: %d %v", len(written), err)
	}
	again, err := repo.Assign(ctx, []domain.ScholarshipHolding{holding(t, "5c000000-0000-0000-0000-0000000000b3", tenantA, scFree, scStudent1)})
	if err != nil || len(again) != 0 {
		t.Fatalf("one open holding per pupil: %d %v", len(again), err)
	}
	held, err := repo.HeldBy(ctx, []string{scStudent1})
	if err != nil || len(held[scStudent1]) != 1 || held[scStudent1][0].ID() != scFree {
		t.Fatalf("held by: %+v %v", held, err)
	}
	counts, err := repo.CountHolders(ctx)
	if err != nil || counts[scFree] != 2 {
		t.Fatalf("count %d %v", counts[scFree], err)
	}
	if other, err := appRepo(t, tenantB).HeldBy(ctx, []string{scStudent1}); err != nil || len(other) != 0 {
		t.Fatalf("RLS: another school sees %+v %v", other, err)
	}

	n, err := repo.Unassign(ctx, scFree, []string{scStudent1, "not-a-uuid"}, time.Date(2031, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || n != 1 {
		t.Fatalf("unassign %d %v", n, err)
	}
	holders, err := repo.ListHolders(ctx, scFree)
	if err != nil || len(holders) != 1 || holders[0].StudentID != scStudent2 {
		t.Fatalf("holders %+v %v", holders, err)
	}
	if _, err := repo.Assign(ctx, []domain.ScholarshipHolding{holding(t, "5c000000-0000-0000-0000-0000000000b4", tenantA, scFree, scStudent1)}); err != nil {
		t.Fatalf("a pupil may hold it again after it ended: %v", err)
	}
}

// A deposit that settles a charge under a scholarship writes its part; the
// receipt prints it with the name; paid_by_due counts what came by the due
// date; voiding the deposit takes the part back.
func TestADepositWritesTheScholarshipAndTheReceiptPrintsIt(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "5c000000-0000-0000-0000-00000000005c"
	const becaID = "5c000000-0000-0000-0000-000000000004"
	repo := appRepo(t, tenant)
	if _, err := repo.CreateScholarship(ctx, mustScholarship(t, becaID, tenant, "Media beca", domain.ScholarshipPercent, 5000, true, domain.TargetTuition)); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2031, 9, 10, 0, 0, 0, 0, time.UTC)
	charge := seedConceptAndCharge(t, tenant, "5c000000-0000-0000-0000-0000000000d1", "colegiatura beca",
		"5c000000-0000-0000-0000-0000000000e1", student, scCycle, 300000, 0, due)

	paidOn := time.Date(2031, 9, 10, 23, 30, 0, 0, time.UTC) // the due day, late at night
	deposit := newDeposit(t, "5c000000-0000-0000-0000-0000000000f1", tenant, student, 150000, paidOn, "sc-dep-1")
	_, _, err := repo.RegisterDeposit(ctx, deposit, func(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
		var target ports.ChargeWithPaid
		for _, c := range open {
			if c.Charge.ID() == charge.ID() {
				target = c
			}
		}
		withBeca, err := target.Charge.WithScholarship(becaID, 150000, 0)
		if err != nil {
			return ports.DepositWrite{}, err
		}
		pay := newPayment(t, "5c000000-0000-0000-0000-0000000000f2", tenant, charge.ID(), 150000, paidOn)
		pay = pay.WithDepositID(deposit.ID())
		return ports.DepositWrite{Scholarships: []domain.Charge{withBeca}, Allocations: []domain.Payment{pay}}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	listed, err := repo.ListCharges(ctx, student, "")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list %v", err)
	}
	got := listed[0]
	if got.Charge.Status() != domain.StatusPaid || got.Charge.Scholarship() != 150000 || got.Charge.ScholarshipID() != becaID {
		t.Fatalf("charge %s beca %q %d", got.Charge.Status(), got.Charge.ScholarshipID(), got.Charge.Scholarship())
	}
	if got.PaidByDueCents != 150000 {
		t.Fatalf("paid by due %d: the due day counts whole", got.PaidByDueCents)
	}
	receipt, err := repo.DepositReceipt(ctx, deposit.ID())
	if err != nil || len(receipt.Lines) != 1 {
		t.Fatalf("receipt %+v %v", receipt, err)
	}
	if l := receipt.Lines[0]; l.ScholarshipCents != 150000 || l.ScholarshipName != "Media beca" {
		t.Fatalf("receipt line %+v", l)
	}

	voided, err := deposit.Void("capturado dos veces", paidOn.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.VoidDeposit(ctx, voided); err != nil {
		t.Fatalf("void: %v", err)
	}
	after, err := repo.GetCharge(ctx, charge.ID())
	if err != nil {
		t.Fatal(err)
	}
	if after.Charge.Scholarship() != 0 || after.Charge.ScholarshipID() != "" || after.Charge.Status() != domain.StatusPending {
		t.Fatalf("released: %s beca %q %d", after.Charge.Status(), after.Charge.ScholarshipID(), after.Charge.Scholarship())
	}
}

// What a scholarship settled by itself is frozen once, under the lock, and a
// second freeze is a no-op; the CHECK keeps discount + part within the amount.
func TestFreezeScholarships(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "5c000000-0000-0000-0000-00000000005d"
	const becaID = "5c000000-0000-0000-0000-000000000005"
	repo := appRepo(t, tenant)
	if _, err := repo.CreateScholarship(ctx, mustScholarship(t, becaID, tenant, "Gratis", domain.ScholarshipFixedPrice, 0, true, domain.TargetTuition)); err != nil {
		t.Fatal(err)
	}
	charge := seedConceptAndCharge(t, tenant, "5c000000-0000-0000-0000-0000000000d2", "colegiatura gratis",
		"5c000000-0000-0000-0000-0000000000e2", student, scCycle, 300000, 0, time.Date(2031, 9, 10, 0, 0, 0, 0, time.UTC))
	free, err := charge.WithScholarship(becaID, 300000, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{1, 0} {
		n, err := repo.FreezeScholarships(ctx, []domain.Charge{free})
		if err != nil || n != want {
			t.Fatalf("freeze #%d: %d %v", i+1, n, err)
		}
	}
	after, err := repo.GetCharge(ctx, charge.ID())
	if err != nil || after.Charge.Scholarship() != 300000 || after.Charge.Effective() != 0 {
		t.Fatalf("frozen %+v %v", after.Charge, err)
	}
	// The database refuses a part that, beside the discount, exceeds the amount.
	err = superDB.Exec(`UPDATE charges SET discount_cents = 1 WHERE id = ?`, charge.ID()).Error
	if err == nil {
		t.Fatal("CHECK (discount_cents + scholarship_cents <= amount_cents) must refuse")
	}
}
