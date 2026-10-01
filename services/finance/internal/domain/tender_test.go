package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func aTender(t *testing.T, method string, cents int64, reference, cardType string) domain.Tender {
	t.Helper()
	tender, err := domain.NewTender(method, cents, reference, cardType)
	if err != nil {
		t.Fatalf("NewTender(%s, %d): %v", method, cents, err)
	}
	return tender
}

// T8: a credit note is a way the school is paid, typed by hand by the cashier.
// It is not money: IsMoney says so, and every report that adds money asks it.
func TestACreditNoteIsATenderButNotMoney(t *testing.T) {
	note := aTender(t, domain.MethodCreditNote, 50000, "  NC-12  ", "")
	if note.Method() != domain.MethodCreditNote || note.Amount().Int64() != 50000 {
		t.Errorf("tender wrong: %+v", note)
	}
	if note.Reference() != "NC-12" {
		t.Errorf("reference = %q, want it trimmed", note.Reference())
	}
	if note.IsMoney() {
		t.Error("a credit note is not money in the drawer")
	}
	if !aTender(t, domain.MethodCash, 100, "", "").IsMoney() {
		t.Error("cash is money")
	}
}

func TestATenderIsValidated(t *testing.T) {
	cases := []struct {
		name           string
		method         string
		cents          int64
		cardType       string
		wantKindOfFail glerrors.Kind
	}{
		{"zero amount", domain.MethodCash, 0, "", glerrors.KindInvalid},
		{"negative amount", domain.MethodCash, -1, "", glerrors.KindInvalid},
		{"unknown method", "vale", 100, "", glerrors.KindInvalid},
		{"mixed is a summary, never a tender", domain.MethodMixed, 100, "", glerrors.KindInvalid},
		{"card type on cash", domain.MethodCash, 100, domain.CardDebit, glerrors.KindInvalid},
		{"unknown card type", domain.MethodCard, 100, "despensa", glerrors.KindInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := domain.NewTender(c.method, c.cents, "", c.cardType); glerrors.KindOf(err) != c.wantKindOfFail {
				t.Errorf("want %v, got %v", c.wantKindOfFail, err)
			}
		})
	}
	card := aTender(t, domain.MethodCard, 100, "", domain.CardCredit)
	if card.CardType() != domain.CardCredit {
		t.Errorf("card type = %q, want credit", card.CardType())
	}
}

// A deposit made the old way is one tender: its method, its amount, its
// reference, its card type. Readers never have to special-case it.
func TestALegacyDepositIsOneTender(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 320000, domain.MethodCard, "V-9",
		day(2026, time.October, 10), "", "")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	d, err = d.WithCardType(domain.CardDebit)
	if err != nil {
		t.Fatalf("WithCardType: %v", err)
	}
	got := d.Tenders()
	if len(got) != 1 {
		t.Fatalf("tenders = %d, want 1", len(got))
	}
	if got[0].Method() != domain.MethodCard || got[0].Amount().Int64() != 320000 ||
		got[0].Reference() != "V-9" || got[0].CardType() != domain.CardDebit {
		t.Errorf("tender = %+v", got[0])
	}
}

// The strict rule the school approved: the methods add up to the amount to pay,
// or nothing is registered.
func TestTendersMustAddUpToTheDeposit(t *testing.T) {
	d := aDeposit(t) // 670000 cash
	short := []domain.Tender{
		aTender(t, domain.MethodCreditNote, 200000, "", ""),
		aTender(t, domain.MethodCash, 400000, "", ""),
	}
	if _, err := d.WithTenders(short); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("tenders short of the amount: want KindInvalid, got %v", err)
	}
	if _, err := d.WithTenders(nil); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("no tenders: want KindInvalid, got %v", err)
	}
	five := make([]domain.Tender, 0, 5)
	for range 5 {
		five = append(five, aTender(t, domain.MethodCash, 134000, "", ""))
	}
	if _, err := d.WithTenders(five); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("more tenders than the counter offers: want KindInvalid, got %v", err)
	}
}

// Several methods make a "mixed" deposit: the summary column cannot name one
// method, and it carries no card type or reference of its own — those live on
// each tender.
func TestSeveralTendersMakeAMixedDeposit(t *testing.T) {
	d := aDeposit(t)
	mixed, err := d.WithTenders([]domain.Tender{
		aTender(t, domain.MethodCreditNote, 200000, "NC-1", ""),
		aTender(t, domain.MethodCard, 470000, "V-2", domain.CardCredit),
	})
	if err != nil {
		t.Fatalf("WithTenders: %v", err)
	}
	if mixed.Method() != domain.MethodMixed || mixed.CardType() != "" || mixed.Reference() != "" {
		t.Errorf("summary = %q/%q/%q, want mixed with no card type nor reference", mixed.Method(), mixed.CardType(), mixed.Reference())
	}
	if len(mixed.Tenders()) != 2 {
		t.Errorf("tenders = %d, want 2", len(mixed.Tenders()))
	}

	single, err := d.WithTenders([]domain.Tender{aTender(t, domain.MethodCard, 670000, "V-3", domain.CardDebit)})
	if err != nil {
		t.Fatalf("WithTenders single: %v", err)
	}
	if single.Method() != domain.MethodCard || single.CardType() != domain.CardDebit || single.Reference() != "V-3" {
		t.Errorf("single summary = %q/%q/%q", single.Method(), single.CardType(), single.Reference())
	}
}

// A deposit's allocations are cut where one tender ends and the next begins, in
// the order the tenders were given, so every payment row carries exactly one
// method and a report can tell the credit note from the cash per concept.
func TestSplitByTenderCutsAllocationsAtTenderBoundaries(t *testing.T) {
	tenders := []domain.Tender{
		aTender(t, domain.MethodCreditNote, 200000, "NC-1", ""),
		aTender(t, domain.MethodCash, 470000, "", ""),
	}
	allocations := []domain.Allocation{
		{ChargeID: "sep", AmountCents: 300000},
		{ChargeID: "sep-fee", AmountCents: 10000, Surcharge: true},
		{ChargeID: "oct", AmountCents: 300000},
	}
	got := domain.SplitByTender(allocations, tenders)
	want := []struct {
		charge string
		method string
		cents  int64
	}{
		{"sep", domain.MethodCreditNote, 200000},
		{"sep", domain.MethodCash, 100000},
		{"sep-fee", domain.MethodCash, 10000},
		{"oct", domain.MethodCash, 300000},
	}
	if len(got) != len(want) {
		t.Fatalf("slices = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ChargeID != w.charge || got[i].Tender.Method() != w.method || got[i].AmountCents != w.cents {
			t.Errorf("slice %d = %s/%s/%d, want %s/%s/%d", i,
				got[i].ChargeID, got[i].Tender.Method(), got[i].AmountCents, w.charge, w.method, w.cents)
		}
	}
	if !got[2].Surcharge {
		t.Error("the surcharge slice lost its surcharge mark")
	}
}
