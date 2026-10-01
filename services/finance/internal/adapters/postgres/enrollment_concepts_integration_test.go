//go:build integration

// Integration tests for M18: the inscription broken into priced lines, and the
// re-pricing of the ones nobody has paid yet.
//
// Run: go test -tags=integration ./internal/adapters/postgres/
// Requires Docker.
package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

func ymd(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// flyerConcepts is the school's printed price list: a ladder that doubles
// between January and April, and two lines that do not move.
func flyerConcepts() []domain.EnrollmentConcept {
	return []domain.EnrollmentConcept{
		{Name: "Inscripción", Tiers: []domain.PriceTier{
			{EffectiveFrom: ymd(2027, time.January, 1), AmountCents: 220000},
			{EffectiveFrom: ymd(2027, time.March, 1), AmountCents: 330000},
			{EffectiveFrom: ymd(2027, time.April, 1), AmountCents: 440000},
		}},
		{Name: "Gastos adm y seguro", Tiers: []domain.PriceTier{
			{EffectiveFrom: ymd(2027, time.January, 1), AmountCents: 470000},
		}},
		{Name: "Cuota Asociación Padres de Familia", Tiers: []domain.PriceTier{
			{EffectiveFrom: ymd(2027, time.January, 1), AmountCents: 85000},
		}},
	}
}

// referenced tags a charge with the reference the billing run would give it.
func referenced(t *testing.T, c domain.Charge, ref string) domain.Charge {
	t.Helper()
	out, err := c.WithExternalRef(ref)
	if err != nil {
		t.Fatalf("external ref: %v", err)
	}
	return out
}

func newPlanWithConcepts(t *testing.T, id, tenant, cycle, level string, concepts []domain.EnrollmentConcept) domain.TuitionPlan {
	t.Helper()
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: id, TenantID: tenant, CycleID: cycle, LevelID: level,
		MonthlyCents: 320000, Months: 11, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentConcepts: concepts,
	})
	if err != nil {
		t.Fatalf("new tuition plan: %v", err)
	}
	return p
}

// The whole ladder survives the database. A price list that came back with its
// steps flattened, reordered or shifted by a timezone would charge a different
// year than the one the director typed.
func TestEnrollmentConceptsSurviveTheRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	want := flyerConcepts()

	if _, err := repo.UpsertTuitionPlan(ctx, newPlanWithConcepts(t,
		"a8000000-0000-0000-0000-000000000001", tenantA, "a8000000-0000-0000-0000-0000000000c1", "a8000000-0000-0000-0000-0000000000e1", want)); err != nil {
		t.Fatalf("upsert plan: %v", err)
	}

	plans, err := repo.ListTuitionPlans(ctx, "a8000000-0000-0000-0000-0000000000c1")
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("got %d plans, want 1", len(plans))
	}
	got := plans[0].EnrollmentConcepts()
	if len(got) != len(want) {
		t.Fatalf("got %d concepts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name {
			t.Errorf("concept %d name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		if len(got[i].Tiers) != len(want[i].Tiers) {
			t.Fatalf("concept %q: %d steps, want %d", want[i].Name, len(got[i].Tiers), len(want[i].Tiers))
		}
		for j := range want[i].Tiers {
			if !got[i].Tiers[j].EffectiveFrom.Equal(want[i].Tiers[j].EffectiveFrom) {
				t.Errorf("concept %q step %d starts %s, want %s", want[i].Name, j,
					got[i].Tiers[j].EffectiveFrom.Format(time.RFC3339),
					want[i].Tiers[j].EffectiveFrom.Format(time.RFC3339))
			}
			if got[i].Tiers[j].AmountCents != want[i].Tiers[j].AmountCents {
				t.Errorf("concept %q step %d = %d, want %d", want[i].Name, j,
					got[i].Tiers[j].AmountCents, want[i].Tiers[j].AmountCents)
			}
		}
	}

	// The flyer's own totals, read back off the database rather than off a
	// struct built in memory.
	for _, tc := range []struct {
		on   time.Time
		want int64
	}{
		{ymd(2027, time.January, 15), 775000},
		{ymd(2027, time.March, 3), 885000},
		{ymd(2027, time.April, 20), 995000},
	} {
		var total int64
		for _, l := range plans[0].EnrollmentLinesOn(tc.on) {
			total += l.AmountCents
		}
		if total != tc.want {
			t.Errorf("total on %s = %d, want %d", tc.on.Format("2006-01-02"), total, tc.want)
		}
	}
}

// Every plan written before M18 goes on meaning what it meant. The column
// default is '[]', so an old row reads as the single inscription it always was.
func TestAPlanWrittenBeforeM18StillReadsAsOneInscription(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)

	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "a8000000-0000-0000-0000-000000000002", TenantID: tenantA, CycleID: "a8000000-0000-0000-0000-0000000000c2", LevelID: "a8000000-0000-0000-0000-0000000000e2",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
	})
	if err != nil {
		t.Fatalf("new tuition plan: %v", err)
	}
	if _, err := repo.UpsertTuitionPlan(ctx, plan); err != nil {
		t.Fatalf("upsert plan: %v", err)
	}

	plans, err := repo.ListTuitionPlans(ctx, "a8000000-0000-0000-0000-0000000000c2")
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if n := len(plans[0].EnrollmentConcepts()); n != 0 {
		t.Fatalf("got %d concepts, want none", n)
	}
	lines := plans[0].EnrollmentLinesOn(ymd(2027, time.April, 1))
	if len(lines) != 1 || lines[0].AmountCents != 500000 {
		t.Fatalf("lines = %+v, want one line of 500000", lines)
	}
}

// --- re-pricing what nobody has paid ----------------------------------------

// repriceFixture writes one unpaid charge and one that has taken money, both
// under refs the run would use, and returns their ids.
func repriceFixture(t *testing.T, tenant, hex string) (unpaidID, partPaidID, unpaidRef, partPaidRef string) {
	t.Helper()
	ctx := context.Background()
	repo := appRepo(t, tenant)

	// Every identity here is a uuid column; hex distinguishes one fixture's rows
	// from another's inside the same database.
	id := func(tag string) string { return "a8000000-0000-0000-0000-0000000" + hex + tag }
	// The catalog holds one concept per NAME per school, so each fixture names
	// its own rather than fighting the constraint.
	concept := newConcept(t, id("c00"), tenant, "Inscripción "+hex, 220000)
	if _, err := repo.CreateConcept(ctx, concept); err != nil {
		t.Fatalf("create concept: %v", err)
	}

	cycle := id("ce0")
	student1, student2 := id("a01"), id("a02")
	unpaidID = id("f01")
	partPaidID = id("f02")
	unpaidRef = "enrollment:" + cycle + ":" + student1 + ":inscripcin"
	partPaidRef = "enrollment:" + cycle + ":" + student2 + ":inscripcin"
	unpaid := referenced(t,
		newCharge(t, unpaidID, tenant, student1, cycle, concept.ID(), 220000, 0, ymd(2027, time.January, 10)),
		unpaidRef)
	partPaid := referenced(t,
		newCharge(t, partPaidID, tenant, student2, cycle, concept.ID(), 220000, 0, ymd(2027, time.January, 10)),
		partPaidRef)

	if _, err := repo.CreateChargesBulk(ctx, []domain.Charge{unpaid, partPaid}); err != nil {
		t.Fatalf("create charges: %v", err)
	}
	if _, _, _, err := repo.RegisterPayment(ctx,
		one(newPayment(t, id("b01"), tenant, partPaidID, 100000, ymd(2027, time.January, 20))), derive, nil); err != nil {
		t.Fatalf("register payment: %v", err)
	}
	return unpaidID, partPaidID, unpaidRef, partPaidRef
}

// The rule of ADR 0011, at the only place that can enforce it: the first peso
// freezes the price. An unpaid inscription follows the ladder; one that has
// taken a single partial payment keeps the price the family was quoted, because
// the receipt in their hand must not contradict the statement.
func TestRepricingMovesTheUnpaidAndLeavesTheOneThatTookMoney(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	unpaidID, partPaidID, unpaidRef, partPaidRef := repriceFixture(t, tenantA, "a1")

	moved, err := repo.RepriceUnpaid(ctx, []ports.ChargeReprice{
		{ExternalRef: unpaidRef, AmountCents: 330000},
		{ExternalRef: partPaidRef, AmountCents: 330000},
	})
	if err != nil {
		t.Fatalf("reprice: %v", err)
	}
	if moved != 1 {
		t.Fatalf("repriced %d charges, want only the unpaid one", moved)
	}

	unpaid, err := repo.GetCharge(ctx, unpaidID)
	if err != nil {
		t.Fatalf("get unpaid: %v", err)
	}
	if got := unpaid.Charge.Amount().Int64(); got != 330000 {
		t.Errorf("unpaid inscription = %d, want March's 330000", got)
	}
	partPaid, err := repo.GetCharge(ctx, partPaidID)
	if err != nil {
		t.Fatalf("get part-paid: %v", err)
	}
	if got := partPaid.Charge.Amount().Int64(); got != 220000 {
		t.Errorf("part-paid inscription = %d, want the frozen 220000", got)
	}
}

// Re-running the same re-price changes nothing and reports nothing. The billing
// run calls this on every pass, and a count that grew each night would make the
// report meaningless.
func TestRepricingTwiceReportsNothingTheSecondTime(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	_, _, unpaidRef, _ := repriceFixture(t, tenantA, "b2")

	items := []ports.ChargeReprice{
		{ExternalRef: unpaidRef, AmountCents: 330000},
	}
	if _, err := repo.RepriceUnpaid(ctx, items); err != nil {
		t.Fatalf("first reprice: %v", err)
	}
	again, err := repo.RepriceUnpaid(ctx, items)
	if err != nil {
		t.Fatalf("second reprice: %v", err)
	}
	if again != 0 {
		t.Errorf("second run repriced %d, want 0", again)
	}
}

// A cancelled inscription is history. Re-pricing one would resurrect a debt the
// school withdrew.
func TestRepricingSkipsACancelledInscription(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)

	concept := newConcept(t, "a8000000-0000-0000-0000-0000000000d3", tenantA, "Inscripción cancelada", 220000)
	if _, err := repo.CreateConcept(ctx, concept); err != nil {
		t.Fatalf("create concept: %v", err)
	}
	charge := referenced(t,
		newCharge(t, "a8000000-0000-0000-0000-0000000000f3", tenantA, "a8000000-0000-0000-0000-0000000000a3", "a8000000-0000-0000-0000-0000000000c3", concept.ID(), 220000, 0, ymd(2027, time.January, 10)),
		"enrollment:cyc-m18-c:stu-3:inscripcin")
	if _, err := repo.CreateChargesBulk(ctx, []domain.Charge{charge}); err != nil {
		t.Fatalf("create charge: %v", err)
	}
	if _, err := repo.CancelChargeIfUnpaid(ctx, "a8000000-0000-0000-0000-0000000000f3"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	moved, err := repo.RepriceUnpaid(ctx, []ports.ChargeReprice{
		{ExternalRef: "enrollment:cyc-m18-c:stu-3:inscripcin", AmountCents: 330000},
	})
	if err != nil {
		t.Fatalf("reprice: %v", err)
	}
	if moved != 0 {
		t.Errorf("repriced %d cancelled charges, want 0", moved)
	}
}

// RLS, for the one operation in this slice that WRITES money. A school holding
// the external ref of another school's charge must not be able to move its
// price — the refs are predictable strings, so this is not a theoretical reach.
func TestRepricingCannotReachAnotherSchoolsCharge(t *testing.T) {
	ctx := context.Background()
	unpaidID, _, unpaidRef, _ := repriceFixture(t, tenantB, "c3")

	intruder := appRepo(t, tenantA)
	moved, err := intruder.RepriceUnpaid(ctx, []ports.ChargeReprice{
		{ExternalRef: unpaidRef, AmountCents: 990000},
	})
	if err != nil {
		t.Fatalf("reprice: %v", err)
	}
	if moved != 0 {
		t.Fatalf("tenant A repriced %d of tenant B's charges", moved)
	}

	victim := appRepo(t, tenantB)
	charge, err := victim.GetCharge(ctx, unpaidID)
	if err != nil {
		t.Fatalf("get victim charge: %v", err)
	}
	if got := charge.Charge.Amount().Int64(); got != 220000 {
		t.Fatalf("tenant B's inscription = %d, want the untouched 220000", got)
	}
}

// The domain refuses a plan carrying both shapes of inscription, so the mapper
// can never send one. The CHECK exists anyway, for the paths that do not go
// through the mapper — a migration, a fix applied by hand at 11pm, a future
// service. An unverified constraint is a claim, so this one is exercised as the
// owner, bypassing RLS and the domain alike.
func TestTheDatabaseAlsoRefusesBothShapesOfInscription(t *testing.T) {
	ctx := context.Background()
	planID := "a8000000-0000-0000-0000-000000000003"
	if _, err := appRepo(t, tenantA).UpsertTuitionPlan(ctx, newPlanWithConcepts(t,
		planID, tenantA, "a8000000-0000-0000-0000-0000000000c4",
		"a8000000-0000-0000-0000-0000000000e4", flyerConcepts())); err != nil {
		t.Fatalf("upsert plan: %v", err)
	}

	err := superDB.Exec(
		"UPDATE tuition_plans SET enrollment_cents = 500000 WHERE id = ?", planID).Error
	if err == nil {
		t.Fatal("the database accepted a plan priced two ways at once")
	}
	if !strings.Contains(err.Error(), "tuition_plans_one_inscription_shape_ck") {
		t.Fatalf("refused by %v, want the one-inscription-shape check", err)
	}
}

// Pricing a level that was ALREADY priced. The upsert's assignment list has to
// carry every mutable column, and the one added by 0015 is the newest chance to
// forget one — which is how the daily surcharge and the split schedule looked
// saved and were not.
//
// Both directions, because they fail differently: forgetting the column on the
// way IN stores an inscription of zero (enrollment_cents is assigned to 0, the
// concepts stay '[]', and the shape CHECK is happy with that); forgetting it on
// the way BACK OUT leaves concepts behind a non-zero amount and every later save
// of that level dies on the CHECK with an opaque internal error.
func TestRepricingALevelThatWasAlreadyPricedKeepsBothShapesStraight(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	cycle := "a8000000-0000-0000-0000-0000000000c5"
	level := "a8000000-0000-0000-0000-0000000000e5"

	single, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "a8000000-0000-0000-0000-000000000005", TenantID: tenantA, CycleID: cycle, LevelID: level,
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
	})
	if err != nil {
		t.Fatalf("new plan: %v", err)
	}
	if _, err := repo.UpsertTuitionPlan(ctx, single); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Now the school breaks that one amount into the flyer's three lines.
	stored, err := repo.UpsertTuitionPlan(ctx, newPlanWithConcepts(t,
		"a8000000-0000-0000-0000-000000000006", tenantA, cycle, level, flyerConcepts()))
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if n := len(stored.EnrollmentConcepts()); n != 3 {
		t.Fatalf("after re-pricing an existing level: %d concepts, want 3", n)
	}
	var total int64
	for _, l := range stored.EnrollmentLinesOn(ymd(2027, time.March, 3)) {
		total += l.AmountCents
	}
	if total != 885000 {
		t.Fatalf("inscription = %d, want March's 885000 — an inscription of 0 means the column was not assigned", total)
	}

	// And back again: the school decides one amount was simpler after all.
	back, err := repo.UpsertTuitionPlan(ctx, single)
	if err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	if n := len(back.EnrollmentConcepts()); n != 0 {
		t.Fatalf("after going back to one amount: %d concepts, want none", n)
	}
	if got := back.EnrollmentCents(); got != 500000 {
		t.Errorf("inscription = %d, want 500000", got)
	}
}
