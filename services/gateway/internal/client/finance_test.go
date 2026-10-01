package client

import (
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// The mapping this pins was silently missing twice: a field existed on the
// struct, carried a JSON tag, and was never copied to the proto. The screen
// saved a plan, the API answered zeros, and everything compiled and linted.
func TestTuitionPlanRequestCarriesEveryField(t *testing.T) {
	sent := tuitionPlanRequest(TuitionPlanInput{
		CycleID: "c1", LevelID: "lv1", EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		MonthlyDiscountCents: 50000,
		Schedule:             []Installment{{Month: 9, AmountCents: 320000}},
		Fees:                 []Fee{{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}}},
		EnrollmentDueOn:      "2026-06-30",
		EnrollmentStartsOn:   "2026-06-01",
		Policy: CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true,
			EarlyPaymentCents: 10000, EarlyPaymentDays: 5,
		},
	})

	if sent.GetMonthlyDiscountCents() != 50000 {
		t.Errorf("the level discount was not sent: %d", sent.GetMonthlyDiscountCents())
	}
	if len(sent.GetSchedule()) != 1 || sent.GetSchedule()[0].GetAmountCents() != 320000 {
		t.Errorf("the payment plan was not sent: %+v", sent.GetSchedule())
	}
	if !sent.GetPolicy().GetDailyMode() || sent.GetPolicy().GetLateFeeDailyCents() != 1000 {
		t.Errorf("the collection rule was not sent: %+v", sent.GetPolicy())
	}
	if sent.GetPolicy().GetEarlyPaymentDays() != 5 || sent.GetPolicy().GetEarlyPaymentCents() != 10000 {
		t.Errorf("the early-payment reward was not sent: %+v", sent.GetPolicy())
	}
	if sent.GetEnrollmentCents() != 500000 || sent.GetLateFeeCents() != 15000 {
		t.Errorf("the plain amounts were not sent: %+v", sent)
	}
	if len(sent.GetFees()) != 1 {
		t.Fatalf("the level fees were not sent: %+v", sent.GetFees())
	}
	fee := sent.GetFees()[0]
	if fee.GetName() != "Cuota de Julio" || fee.GetAmountCents() != 160000 {
		t.Errorf("the fee was sent as %q for %d", fee.GetName(), fee.GetAmountCents())
	}
	if len(fee.GetMonths()) != 2 || fee.GetMonths()[0] != 9 || fee.GetMonths()[1] != 10 {
		t.Errorf("the fee months were sent as %v, want [9 10]", fee.GetMonths())
	}
	if sent.GetEnrollmentDueOn() != "2026-06-30" {
		t.Errorf("the inscription deadline was not sent: %q", sent.GetEnrollmentDueOn())
	}
	if sent.GetEnrollmentStartsOn() != "2026-06-01" {
		t.Errorf("the inscription start was not sent (T5): %q", sent.GetEnrollmentStartsOn())
	}
}

// And back: the same fields have to survive the answer, or the screen reopens
// showing a plan the school did not save.
func TestTuitionPlanReadsEveryFieldBack(t *testing.T) {
	got := toTuitionPlan(&financev1.TuitionPlan{
		Id: "tp-1", CycleId: "c1", LevelId: "lv1",
		MonthlyCents: 320000, MonthlyDiscountCents: 50000,
		Schedule: []*financev1.Installment{{Month: 12, AmountCents: 160000}},
		Policy:   &financev1.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000},
		Fees: []*financev1.Fee{
			{Name: "Cuota de Julio", AmountCents: 160000, Months: []int32{9, 10}},
		},
		EnrollmentDueOn:    "2026-06-30",
		EnrollmentStartsOn: "2026-06-01",
	})

	if got.MonthlyDiscountCents != 50000 {
		t.Errorf("the level discount was not read back: %d", got.MonthlyDiscountCents)
	}
	if len(got.Schedule) != 1 || got.Schedule[0].Month != 12 {
		t.Errorf("the payment plan was not read back: %+v", got.Schedule)
	}
	if !got.Policy.DailyMode || got.Policy.LateFeeDailyCents != 1000 {
		t.Errorf("the collection rule was not read back: %+v", got.Policy)
	}
	if len(got.Fees) != 1 || got.Fees[0].Name != "Cuota de Julio" || len(got.Fees[0].Months) != 2 {
		t.Errorf("the level fees were not read back: %+v", got.Fees)
	}
	if got.EnrollmentDueOn != "2026-06-30" {
		t.Errorf("the inscription deadline was not read back: %q", got.EnrollmentDueOn)
	}
	if got.EnrollmentStartsOn != "2026-06-01" {
		t.Errorf("the inscription start was not read back (T5): %q", got.EnrollmentStartsOn)
	}
}

// T5: a charge says the day it opens, and the family read asks finance for the
// family's view rather than filtering at the edge.
func TestAChargeCarriesTheDayItOpens(t *testing.T) {
	got := toCharge(&financev1.Charge{Id: "ch-1", DueDate: "2026-06-30", StartsOn: "2026-06-01"})
	if got.StartsOn != "2026-06-01" {
		t.Errorf("starts_on was not read: %q", got.StartsOn)
	}
	if !accountRequest("stu-1", true).GetFamilyView() || accountRequest("stu-1", false).GetFamilyView() {
		t.Error("family_view must be set for the family read and only for it")
	}
}

// The run now issues fee charges too, and the panel reports what it did. A
// count that never leaves the gateway is a count the director cannot see.
func TestABillingRunReportsTheFeesItIssued(t *testing.T) {
	got := toBillingRun(&financev1.RunBillingResponse{
		TuitionCreated: 10, EnrollmentCreated: 1, LateFeesCreated: 2, FeesCreated: 4,
		EnrollmentMoved: 7, Skipped: 3,
	})

	if got.FeesCreated != 4 {
		t.Errorf("fees created came back as %d, want 4", got.FeesCreated)
	}
	if got.EnrollmentMoved != 7 {
		t.Errorf("moved inscriptions came back as %d, want 7", got.EnrollmentMoved)
	}
}

// QA 2026-09-25, ISSUE-010: the cash-up the office reads names every payment
// taken outside a deposit, with what it paid for — not only the deposits.
func TestCashUpCarriesLoosePayments(t *testing.T) {
	cut := toCashUp(&financev1.DailyCashUpResponse{
		CashCents: 135000, TotalCents: 135000,
		Payments: []*financev1.CashUpPayment{{
			Payment:     &financev1.Payment{Id: "pay-1", ChargeId: "chg-1", AmountCents: 135000, Method: "cash", Reference: "r"},
			Description: "Uniformes · U-2026-000001",
		}},
	})
	if len(cut.Payments) != 1 {
		t.Fatalf("carried %d payments, want 1", len(cut.Payments))
	}
	want := CashUpPayment{ID: "pay-1", ChargeID: "chg-1", AmountCents: 135000, Method: "cash", Reference: "r", Description: "Uniformes · U-2026-000001"}
	if cut.Payments[0] != want {
		t.Errorf("payment = %+v, want %+v", cut.Payments[0], want)
	}
	if cut.TotalCents != 135000 {
		t.Errorf("total = %d", cut.TotalCents)
	}
}

// T8, both directions of the wire: the tenders go out as sent and come back on
// the deposit, and the credit-note line of the cash-up is read.
func TestTendersCrossTheWireBothWays(t *testing.T) {
	sent := []Tender{
		{Method: "credit_note", AmountCents: 200000, Reference: "NC-7"},
		{Method: "card", AmountCents: 150000, CardType: "credit"},
	}
	back := fromProtoTenders(toProtoTenders(sent))
	if len(back) != 2 || back[0] != sent[0] || back[1] != sent[1] {
		t.Errorf("round trip = %+v, want %+v", back, sent)
	}

	d := toDeposit(&financev1.Deposit{Id: "dep-1", Method: "mixed", Tenders: toProtoTenders(sent)})
	if d.Method != "mixed" || len(d.Tenders) != 2 || d.Tenders[0] != sent[0] {
		t.Errorf("deposit = %+v", d)
	}

	cut := toCashUp(&financev1.DailyCashUpResponse{CashCents: 1, TotalCents: 1, CreditNoteCents: 200000})
	if cut.CreditNoteCents != 200000 || cut.TotalCents != 1 {
		t.Errorf("cash-up credit notes = %d, total = %d", cut.CreditNoteCents, cut.TotalCents)
	}
}
