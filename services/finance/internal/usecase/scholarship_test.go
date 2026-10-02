package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T18 (ADR 0022): becas computed by the ledger. A $3,000 month due on the
// 10th, $500 inscription already paid; each test sets the level discount and
// the collection policy it is about.

type becaFixture struct {
	*fixture
	becas    *usecase.ScholarshipService
	srepo    *fakeScholarshipRepo
	activity *fakeActivityRepo
}

const monthlyFee = 300000

func newBecaFixture(t *testing.T, levelDiscount int64, policy domain.CollectionPolicy) *becaFixture {
	t.Helper()
	f := billingFixture(t)
	f.students.byID[otherStudent] = ports.StudentRef{ID: otherStudent, Status: "active"}
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: monthlyFee, MonthlyDiscountCents: levelDiscount,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true, Policy: policy,
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	runOn(t, f, "2026-09-01")
	settleInscription(t, f)
	f.clock = time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)

	srepo := newFakeScholarshipRepo(f.repo)
	activities := newFakeActivityRepo()
	f.svc.WithScholarships(srepo)
	becas := usecase.NewScholarshipService(srepo, f.svc, f.repo, activities, f.students, testTenant,
		seqID("beca"), func() time.Time { return f.clock })
	return &becaFixture{fixture: f, becas: becas, srepo: srepo, activity: activities}
}

var (
	noPolicy    = domain.CollectionPolicy{}
	dailyPolicy = domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true}
	earlyPolicy = domain.CollectionPolicy{
		DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true,
		EarlyPaymentCents: 10000, EarlyPaymentDays: 10,
	}
)

type becaSpec struct {
	kind    string
	value   int64
	targets []string
}

func (b *becaFixture) give(t *testing.T, name string, s becaSpec, students ...string) domain.Scholarship {
	t.Helper()
	if len(s.targets) == 0 {
		s.targets = []string{domain.TargetTuition}
	}
	created, err := b.becas.CreateScholarship(context.Background(), usecase.ScholarshipInput{
		Name: name, Kind: s.kind, Value: s.value, Targets: s.targets, Active: true,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if len(students) == 0 {
		students = []string{testStudent}
	}
	if _, err := b.becas.Assign(context.Background(), created.ID(), students); err != nil {
		t.Fatalf("assign %s: %v", name, err)
	}
	return created
}

func (b *becaFixture) quote(t *testing.T, on string) usecase.Expected {
	t.Helper()
	got, err := b.svc.ExpectedToday(context.Background(), testStudent, mustDay(t, on))
	if err != nil {
		t.Fatalf("ExpectedToday(%s): %v", on, err)
	}
	return got
}

func mustDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad date %q", s)
	}
	return d
}

// month is the tuition charge due in that month, as stored.
func (b *becaFixture) month(t *testing.T, m time.Month) ports.ChargeWithPaid {
	t.Helper()
	tuition := b.repo.conceptIDOfKind(domain.KindTuition)
	for _, c := range b.repo.charges {
		if c.ConceptID() == tuition && c.DueDate().Month() == m {
			return b.repo.withPaid(c)
		}
	}
	t.Fatalf("no tuition charge in %s", m)
	return ports.ChargeWithPaid{}
}

func lineOf(t *testing.T, e usecase.Expected, chargeID string) usecase.ExpectedLine {
	t.Helper()
	for _, l := range e.Lines {
		if l.ChargeID == chargeID {
			return l
		}
	}
	t.Fatalf("no line for %s in %+v", chargeID, e.Lines)
	return usecase.ExpectedLine{}
}

// Every combination the owner's order names, quoted for September: base price
// → level discount → scholarship → pronto pago → (late: no scholarship, normal
// surcharge).
func TestTheCounterQuotesTheScholarship(t *testing.T) {
	half := becaSpec{kind: domain.ScholarshipPercent, value: 5000}
	cases := []struct {
		name          string
		level         int64
		policy        domain.CollectionPolicy
		becas         []becaSpec
		on            string
		wantAmount    int64 // what the counter asks for
		wantBeca      int64 // the scholarship's part on September's (due) line
		wantEarly     int64
		wantSurcharge int64
	}{
		{"percent", 0, noPolicy, []becaSpec{half}, "2026-09-03", 150000, 150000, 0, 0},
		{"percent on the due day", 0, dailyPolicy, []becaSpec{half}, "2026-09-10", 150000, 150000, 0, 0},
		{"percent after the level discount", 20000, noPolicy, []becaSpec{half}, "2026-09-03", 140000, 140000, 0, 0},
		{"percent then pronto pago", 0, earlyPolicy, []becaSpec{half}, "2026-09-03", 140000, 150000, 10000, 0},
		{"level discount keeps pronto pago out, as before T18", 20000, earlyPolicy, []becaSpec{half}, "2026-09-03", 140000, 140000, 0, 0},
		{"fixed amount off", 0, noPolicy, []becaSpec{{kind: domain.ScholarshipFixedOff, value: 50000}}, "2026-09-03", 250000, 50000, 0, 0},
		{"fixed amount off after the level discount", 20000, noPolicy, []becaSpec{{kind: domain.ScholarshipFixedOff, value: 50000}}, "2026-09-03", 230000, 50000, 0, 0},
		{"fixed amount to pay", 0, noPolicy, []becaSpec{{kind: domain.ScholarshipFixedPrice, value: 100000}}, "2026-09-03", 100000, 200000, 0, 0},
		{"fixed amount to pay above the price never raises it", 20000, noPolicy, []becaSpec{{kind: domain.ScholarshipFixedPrice, value: 290000}}, "2026-09-03", 280000, 0, 0, 0},
		{"fixed price then pronto pago", 0, earlyPolicy, []becaSpec{{kind: domain.ScholarshipFixedPrice, value: 100000}}, "2026-09-03", 90000, 200000, 10000, 0},
		{"two becas do not stack: the best one", 0, noPolicy, []becaSpec{half, {kind: domain.ScholarshipFixedPrice, value: 100000}}, "2026-09-03", 100000, 200000, 0, 0},
		{"late: no scholarship and the normal surcharge", 0, dailyPolicy, []becaSpec{half}, "2026-09-20", 310000, 0, 0, 10000},
		{"late after the level discount", 20000, dailyPolicy, []becaSpec{half}, "2026-09-20", 290000, 0, 0, 10000},
		{"late with pronto pago: neither", 0, earlyPolicy, []becaSpec{half}, "2026-09-20", 310000, 0, 0, 10000},
		// Settled by the scholarship alone: nothing is asked, so no line.
		{"a free month is never late", 0, dailyPolicy, []becaSpec{{kind: domain.ScholarshipFixedPrice, value: 0}}, "2026-09-20", 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBecaFixture(t, c.level, c.policy)
			b.clock = mustDay(t, c.on).Add(12 * time.Hour)
			for i, s := range c.becas {
				b.give(t, "Beca "+string(rune('A'+i)), s)
			}
			got := b.quote(t, c.on)
			if got.AmountCents != c.wantAmount || got.SurchargeCents != c.wantSurcharge || got.DiscountCents != c.wantEarly {
				t.Fatalf("asks %d (surcharge %d, early %d); want %d (%d, %d)",
					got.AmountCents, got.SurchargeCents, got.DiscountCents, c.wantAmount, c.wantSurcharge, c.wantEarly)
			}
			if got.ScholarshipCents != c.wantBeca {
				t.Fatalf("scholarship total %d, want %d", got.ScholarshipCents, c.wantBeca)
			}
			if c.wantAmount > 0 {
				line := lineOf(t, got, b.month(t, time.September).Charge.ID())
				if line.ScholarshipCents != c.wantBeca || line.TotalCents != c.wantAmount {
					t.Fatalf("September line %+v; want beca %d total %d", line, c.wantBeca, c.wantAmount)
				}
			}
		})
	}
}

// Paying in time writes the scholarship on the charge: from then on it is
// history, and the receipt and the statement show it.
func TestADepositInTimeWritesTheScholarship(t *testing.T) {
	b := newBecaFixture(t, 20000, earlyPolicy)
	beca := b.give(t, "Hermanos", becaSpec{kind: domain.ScholarshipPercent, value: 5000})

	got := deposit(t, b.fixture, 140000, "2026-09-03")

	if got.UnallocatedCents != 0 {
		t.Fatalf("unallocated %d: $2,800 at 50 %% is $1,400 (no pronto pago on a level-discounted month)", got.UnallocatedCents)
	}
	sep := b.month(t, time.September)
	if sep.Charge.Status() != domain.StatusPaid || sep.Charge.ScholarshipID() != beca.ID() || sep.Charge.Scholarship() != 140000 {
		t.Fatalf("September %s beca %q %d", sep.Charge.Status(), sep.Charge.ScholarshipID(), sep.Charge.Scholarship())
	}
	// Taking the scholarship away afterwards never touches what was paid.
	if _, err := b.becas.Unassign(context.Background(), beca.ID(), []string{testStudent}); err != nil {
		t.Fatal(err)
	}
	if after := b.month(t, time.September); after.Charge.Scholarship() != 140000 || after.Charge.Status() != domain.StatusPaid {
		t.Fatal("a paid charge never changes")
	}
}

func TestPartPaymentsInTimeKeepTheScholarship(t *testing.T) {
	b := newBecaFixture(t, 0, dailyPolicy)
	b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})

	deposit(t, b.fixture, 100000, "2026-09-03")
	if sep := b.month(t, time.September); sep.Charge.Scholarship() != 0 || sep.Charge.Status() != domain.StatusPartiallyPaid {
		t.Fatal("a part payment writes no scholarship: it is kept only by settling in time")
	}
	b.clock = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if got := b.quote(t, "2026-09-08"); got.AmountCents != 50000 {
		t.Fatalf("after $1,000 the rest is $500 at the scholarship price, got %d", got.AmountCents)
	}
	deposit(t, b.fixture, 50000, "2026-09-08")
	if sep := b.month(t, time.September); sep.Charge.Status() != domain.StatusPaid || sep.Charge.Scholarship() != 150000 {
		t.Fatalf("settled in time: %s beca %d", sep.Charge.Status(), sep.Charge.Scholarship())
	}
}

// Part paid in time, the rest late: the scholarship is lost, the month is owed
// at full price with the surcharge — the $1,000 already paid counts.
func TestPartPaidThenLateLosesTheScholarship(t *testing.T) {
	b := newBecaFixture(t, 0, dailyPolicy)
	b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})
	deposit(t, b.fixture, 100000, "2026-09-03")
	b.clock = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	got := b.quote(t, "2026-09-20")

	if got.AmountCents != 200000+10000 || got.ScholarshipCents != 0 {
		t.Fatalf("asks %d (beca %d); want $2,000 left + $100 surcharge, no scholarship", got.AmountCents, got.ScholarshipCents)
	}
}

// The ADR's example: a $3,000 month with $1,000 paid BEFORE the 50 %
// scholarship existed. The scholarship price is $1,500 in all; the $1,000
// counts towards it, so $500 is left. Nothing is ever refunded: with $2,000
// paid, nothing is left and the scholarship takes only the $1,000 unpaid.
func TestTheScholarshipAppliesToWhatIsLeftOfAPartPayment(t *testing.T) {
	cases := []struct {
		name     string
		paid     int64
		wantOwed int64
		wantBeca int64
		wantPart int64 // the scholarship's part on the priced charge
	}{
		{"$1,000 paid", 100000, 50000, 150000, 150000},
		// Settled: nothing is asked, so no line carries the part.
		{"$2,000 paid: nothing refunded", 200000, 0, 0, 100000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBecaFixture(t, 0, noPolicy)
			b.clock = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
			deposit(t, b.fixture, c.paid, "2026-09-02")
			b.clock = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
			b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})

			got := b.quote(t, "2026-09-03")

			if got.AmountCents != c.wantOwed || got.ScholarshipCents != c.wantBeca {
				t.Fatalf("asks %d beca %d; want %d beca %d", got.AmountCents, got.ScholarshipCents, c.wantOwed, c.wantBeca)
			}
			account, err := b.svc.GetAccount(context.Background(), testStudent)
			requireNoError(t, err)
			sep := b.month(t, time.September).Charge.ID()
			for _, ch := range account.Charges {
				if ch.Charge.ID() == sep && ch.Charge.Scholarship().Int64() != c.wantPart {
					t.Fatalf("September's part %d, want %d", ch.Charge.Scholarship(), c.wantPart)
				}
			}
		})
	}
}

func TestAMonthPaidBeforeTheScholarshipNeverChanges(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	deposit(t, b.fixture, monthlyFee, "2026-09-03")
	b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})

	st := statementOf(t, b.fixture, testStudent)

	sep := b.month(t, time.September).Charge.ID()
	for _, m := range movementsOf(st, domain.MovementScholarship) {
		if m.ChargeID == sep {
			t.Fatal("a month paid in full before the scholarship gets no scholarship line")
		}
	}
	if b.month(t, time.September).Charge.Scholarship() != 0 {
		t.Fatal("paid charges never change")
	}
	// October, still pending, takes it at once.
	oct := b.month(t, time.October).Charge.ID()
	var found bool
	for _, m := range movementsOf(st, domain.MovementScholarship) {
		if m.ChargeID == oct && m.AmountCents == 150000 && strings.HasPrefix(m.Description, "Beca Media · ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("October's scholarship is not itemized: %+v", movementsOf(st, domain.MovementScholarship))
	}
}

// Removing it, or switching it off, gives the pending months their price
// back; what it already settled — with money, or by itself (a free month) by
// its due date — keeps it.
func TestTakingTheScholarshipAway(t *testing.T) {
	for _, how := range []string{"unassign", "deactivate"} {
		t.Run(how, func(t *testing.T) {
			b := newBecaFixture(t, 0, dailyPolicy)
			free := b.give(t, "Completa", becaSpec{kind: domain.ScholarshipFixedPrice, value: 0})
			b.clock = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

			switch how {
			case "unassign":
				n, err := b.becas.Unassign(context.Background(), free.ID(), []string{testStudent, otherStudent})
				if err != nil || n != 1 {
					t.Fatalf("unassign: %d %v", n, err)
				}
			case "deactivate":
				if _, err := b.becas.UpdateScholarship(context.Background(), free.ID(), usecase.ScholarshipInput{
					Name: "Completa", Kind: domain.ScholarshipFixedPrice, Value: 0,
					Targets: []string{domain.TargetTuition}, Active: false,
				}); err != nil {
					t.Fatal(err)
				}
			}

			for _, m := range []time.Month{time.September, time.October} {
				if c := b.month(t, m).Charge; c.Scholarship() != monthlyFee || c.Effective() != 0 {
					t.Fatalf("%s was settled by the scholarship by its due date and keeps it: beca %d", m, c.Scholarship())
				}
			}
			if c := b.month(t, time.November).Charge; c.Scholarship() != 0 {
				t.Fatal("a month not yet due goes back to its price")
			}
			got := b.quote(t, "2026-10-15")
			if got.AmountCents != 0 {
				t.Fatalf("nothing is due on 15 Oct: September and October were free, got %d", got.AmountCents)
			}
			if nov := b.quote(t, "2026-11-01"); nov.AmountCents != monthlyFee {
				t.Fatalf("November at full price, got %d", nov.AmountCents)
			}
		})
	}
}

// T9: a correction re-dates the money and the rule decides again.
func TestACorrectionReevaluatesTheScholarship(t *testing.T) {
	b := newBecaFixture(t, 0, dailyPolicy)
	b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})
	paid := deposit(t, b.fixture, 150000, "2026-09-03")
	if b.month(t, time.September).Charge.Scholarship() != 150000 {
		t.Fatal("settled in time")
	}
	b.clock = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	correct(t, b.fixture, paid.Deposit.ID(), 150000, "2026-09-20", "corr-beca")

	sep := b.month(t, time.September)
	if sep.Charge.Scholarship() != 0 || sep.Charge.Status() != domain.StatusPartiallyPaid {
		t.Fatalf("paid on the 20th the scholarship is lost: %s beca %d", sep.Charge.Status(), sep.Charge.Scholarship())
	}
	got := b.quote(t, "2026-09-25")
	// $1,500 left of the full price and fifteen days of surcharge at $10.
	if got.AmountCents != 150000+15000 || got.ScholarshipCents != 0 {
		t.Fatalf("asks %d (beca %d), want 165000 and no scholarship", got.AmountCents, got.ScholarshipCents)
	}
}

// The overdue list and the monthly late fee read the same rule: a month the
// scholarship settled is not a debt, and earns no fee.
func TestOverdueAndLateFeesSkipWhatTheScholarshipSettled(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	// billingFixture's plan charged $150 a month late; keep it.
	if _, err := b.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: monthlyFee,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	b.give(t, "Completa", becaSpec{kind: domain.ScholarshipFixedPrice, value: 0})
	b.clock = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

	got := runOn(t, b.fixture, "2026-11-15")

	if got.LateFeesCreated != 0 {
		t.Fatalf("%d late fees on months a full scholarship settled", got.LateFeesCreated)
	}
	overdue, err := b.svc.ListOverdue(context.Background(), testCycle)
	requireNoError(t, err)
	if len(overdue) != 0 {
		t.Fatalf("overdue lists %d charges the scholarship settled", len(overdue))
	}
}

// What a scholarship can name, beyond tuition: the inscription, a concept of
// the catalog, an activity.
func TestScholarshipTargets(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	ctx := context.Background()
	books := b.seedConcept(t, "Libros", 80000)
	charge, err := b.svc.CreateCharge(ctx, usecase.CreateChargeInput{
		StudentID: testStudent, CycleID: testCycle, ConceptID: books.ID(), DueDate: "2026-09-25",
	})
	requireNoError(t, err)
	b.give(t, "Libros a mitad", becaSpec{kind: domain.ScholarshipPercent, value: 5000, targets: []string{domain.ConceptTarget(books.ID())}})

	got := b.quote(t, "2026-09-03")

	line := lineOf(t, got, charge.Charge.ID())
	if line.ScholarshipCents != 40000 || line.TotalCents != 40000 {
		t.Fatalf("the concept's line %+v", line)
	}
	sep := lineOf(t, got, b.month(t, time.September).Charge.ID())
	if sep.ScholarshipCents != 0 {
		t.Fatal("a scholarship on a concept leaves the tuition alone")
	}

	targets, err := b.becas.ListTargets(ctx)
	requireNoError(t, err)
	keys := map[string]string{}
	for _, tg := range targets {
		keys[tg.Key] = tg.Group
	}
	if keys[domain.TargetEnrollment] != "enrollment" || keys[domain.TargetTuition] != "tuition" || keys[domain.ConceptTarget(books.ID())] != "concept" {
		t.Fatalf("targets %+v", targets)
	}
	for k := range keys {
		if strings.HasPrefix(k, "concept:") && k != domain.ConceptTarget(books.ID()) && !strings.Contains(k, b.repo.conceptIDOfKind(domain.KindFee)) {
			t.Fatalf("only the school's own concepts and the fees are offered, got %s", k)
		}
	}
}

func TestAssigningWarnsAboutOverlaps(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	ctx := context.Background()
	first := b.give(t, "Hermanos", becaSpec{kind: domain.ScholarshipPercent, value: 2000})
	second, err := b.becas.CreateScholarship(ctx, usecase.ScholarshipInput{
		Name: "Excelencia", Kind: domain.ScholarshipPercent, Value: 5000,
		Targets: []string{domain.TargetTuition, domain.TargetEnrollment}, Active: true,
	})
	requireNoError(t, err)

	holders, err := b.becas.Assign(ctx, second.ID(), []string{testStudent, otherStudent})
	requireNoError(t, err)

	if len(holders) != 2 {
		t.Fatalf("assigned %d", len(holders))
	}
	for _, h := range holders {
		want := 0
		if h.StudentID == testStudent {
			want = 1
		}
		if len(h.Overlaps) != want || (want == 1 && h.Overlaps[0].ID != first.ID()) {
			t.Fatalf("%s overlaps %+v", h.StudentID, h.Overlaps)
		}
	}
	again, err := b.becas.Assign(ctx, second.ID(), []string{testStudent})
	requireNoError(t, err)
	if len(again) != 0 {
		t.Fatal("assigning twice holds once")
	}
	listed, err := b.becas.ListHolders(ctx, second.ID())
	requireNoError(t, err)
	if len(listed) != 2 {
		t.Fatalf("holders %d", len(listed))
	}
	views, err := b.becas.ListScholarships(ctx, false)
	requireNoError(t, err)
	for _, v := range views {
		if v.Scholarship.ID() == second.ID() && v.AssignedCount != 2 {
			t.Fatalf("count %d", v.AssignedCount)
		}
	}
}

func TestScholarshipServiceRefusals(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	ctx := context.Background()
	in := usecase.ScholarshipInput{Name: "Hermanos", Kind: domain.ScholarshipPercent, Value: 5000, Targets: []string{"tuition"}, Active: true}
	created, err := b.becas.CreateScholarship(ctx, in)
	requireNoError(t, err)

	_, err = b.becas.CreateScholarship(ctx, usecase.ScholarshipInput{Name: "hermános", Kind: domain.ScholarshipPercent, Value: 1, Targets: []string{"tuition"}})
	requireKind(t, err, glerrors.KindConflict)

	_, err = b.becas.CreateScholarship(ctx, usecase.ScholarshipInput{Name: "Mala", Kind: domain.ScholarshipPercent, Value: 10001, Targets: []string{"tuition"}})
	requireKind(t, err, glerrors.KindInvalid)

	_, err = b.becas.Assign(ctx, created.ID(), []string{"stu-unknown"})
	requireKind(t, err, glerrors.KindInvalid)

	in.Active = false
	_, err = b.becas.UpdateScholarship(ctx, created.ID(), in)
	requireNoError(t, err)
	_, err = b.becas.Assign(ctx, created.ID(), []string{testStudent})
	requireKind(t, err, glerrors.KindConflict)

	_, err = b.becas.Assign(ctx, "nope", []string{testStudent})
	requireKind(t, err, glerrors.KindNotFound)
}
