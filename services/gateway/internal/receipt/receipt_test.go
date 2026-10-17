package receipt_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

type fakeSources struct {
	deposit  client.DepositReceipt
	order    client.UniformOrder
	student  client.StudentResult
	account  client.AccountStatement
	profile  client.SchoolProfile
	found    bool
	orderErr error
}

func (f fakeSources) DepositReceipt(_ context.Context, _ string) (client.DepositReceipt, error) {
	return f.deposit, nil
}

func (f fakeSources) GetOrder(_ context.Context, _ string, _ []string) (client.UniformOrder, error) {
	return f.order, f.orderErr
}

func (f fakeSources) GetStudent(_ context.Context, _ string) (client.StudentResult, error) {
	return f.student, nil
}

func (f fakeSources) GetAccount(_ context.Context, _ string) (client.AccountStatement, error) {
	return f.account, nil
}

func (f fakeSources) GetSchoolProfile(_ context.Context) (client.SchoolProfile, bool, error) {
	return f.profile, f.found, nil
}

func builder(f fakeSources) *receipt.Builder {
	return receipt.NewBuilder(f, f, f, f, f)
}

var school = client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", RFC: "EGL150101AB1", Legend: "Gracias por su pago."}

// A payment's receipt: the school, the folio, the pupil, what each concept got,
// the late fee and discount apart, the credit, the total handed over, how and
// who took it.
func TestAPaymentReceipt(t *testing.T) {
	f := fakeSources{
		profile: school, found: true,
		student: client.StudentResult{FirstName: "Camila", LastName: "Zamora Sánchez", GradeGroup: "1° A"},
		deposit: client.DepositReceipt{
			Deposit: client.Deposit{
				ID: "dep-1", StudentID: "stu-1", AmountCents: 350000, Method: "card", CardType: "debit",
				Reference: "1234", PaidAt: "2026-10-10T00:00:00Z", Folio: "P-2026-000042", ReceivedByName: "Laura Caja",
			},
			Lines: []client.ReceiptLine{
				{Description: "Colegiatura septiembre 2026", AmountCents: 310000, DiscountCents: 10000},
				{Description: "Recargo · Colegiatura septiembre 2026", AmountCents: 30000, Surcharge: true},
			},
			UnallocatedCents: 10000,
		},
	}

	got, err := builder(f).Payment(context.Background(), "dep-1")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if got.Folio != "P-2026-000042" || got.School.Name != "COLEGIO GARCÍA LORCA" || got.Kind != receipt.KindPayment {
		t.Errorf("head = %q %q %q", got.Folio, got.School.Name, got.Kind)
	}
	if got.Buyer != "Zamora Sánchez Camila" || got.BuyerDetail != "1° A" {
		t.Errorf("buyer = %q (%q)", got.Buyer, got.BuyerDetail)
	}
	if got.PaidOn != "2026-10-10" || got.Method != "Tarjeta de débito" || got.Reference != "1234" || got.ReceivedBy != "Laura Caja" {
		t.Errorf("payment = %q %q %q %q", got.PaidOn, got.Method, got.Reference, got.ReceivedBy)
	}
	if len(got.Lines) != 2 || got.SurchargeCents != 30000 || got.DiscountCents != 10000 || got.CreditCents != 10000 || got.TotalCents != 350000 {
		t.Errorf("amounts = %d lines, surcharge %d, discount %d, credit %d, total %d",
			len(got.Lines), got.SurchargeCents, got.DiscountCents, got.CreditCents, got.TotalCents)
	}
}

// A voided deposit still prints, and says so.
func TestAVoidedPaymentSaysSo(t *testing.T) {
	f := fakeSources{deposit: client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-2", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-10T00:00:00Z",
		Folio: "P-2026-000043", VoidedAt: "2026-10-11T00:00:00Z", VoidReason: "capturado mal",
	}}}
	got, err := builder(f).Payment(context.Background(), "dep-2")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if !got.Voided || got.VoidReason != "capturado mal" {
		t.Errorf("voided = %v %q", got.Voided, got.VoidReason)
	}
	if got.CreditCents != 0 || len(got.Lines) != 0 {
		t.Errorf("a voided receipt = credit %d, %d lines; want none", got.CreditCents, len(got.Lines))
	}
}

// ISSUE-T14-107 · the family's receipt is the office's, for a deposit of THAT
// pupil only: another pupil's deposit is "no such receipt", never a hint that
// it exists.
func TestAFamilyReceiptIsOnlyOfItsOwnPupil(t *testing.T) {
	f := fakeSources{profile: school, found: true,
		student: client.StudentResult{FirstName: "Alumna", LastName: "Dos"},
		deposit: client.DepositReceipt{Deposit: client.Deposit{
			ID: "dep-1", StudentID: "stu-1", AmountCents: 300000, Method: "cash", PaidAt: "2026-10-17T00:00:00Z", Folio: "P-2026-000002",
		}}}

	got, err := builder(f).FamilyPayment(context.Background(), "stu-1", "dep-1")
	if err != nil || got.Folio != "P-2026-000002" || got.TotalCents != 300000 {
		t.Fatalf("own pupil: %+v, %v", got, err)
	}
	_, err = builder(f).FamilyPayment(context.Background(), "stu-2", "dep-1")
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another pupil's deposit: %v, want NotFound", err)
	}
}

// A voided or superseded deposit is not on the family's statement, so the
// family has no receipt of it either: NotFound, like a missing one.
func TestAFamilyReceiptOfAVoidedOrSupersededDepositIsNotFound(t *testing.T) {
	for name, d := range map[string]client.Deposit{
		"voided": {ID: "dep-2", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-10T00:00:00Z",
			Folio: "P-2026-000043", VoidedAt: "2026-10-11T00:00:00Z", VoidReason: "capturado mal"},
		"superseded": {ID: "dep-3", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-10T00:00:00Z",
			Folio: "P-2026-000044", SupersededBy: "dep-4", SupersededByFolio: "P-2026-000045"},
	} {
		f := fakeSources{deposit: client.DepositReceipt{Deposit: d}}
		_, err := builder(f).FamilyPayment(context.Background(), "stu-1", d.ID)
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("%s: %v, want NotFound", name, err)
		}
	}
}

// A uniforms sale's receipt: the order's folio and lines (garment, size,
// quantity), its total, and how it was paid, read from finance by the delivery's
// charge.
func TestASaleReceipt(t *testing.T) {
	f := fakeSources{
		profile: school, found: true,
		order: client.UniformOrder{
			ID: "ord-1", Folio: "U-2026-000007", Source: "counter", StudentID: "stu-1", StudentName: "Zoe Villalobos",
			TotalCents: 96000, CreatedAt: "2026-09-25T06:36:26Z",
			Lines: []client.UniformOrderLine{
				{ItemName: "Playera polo blanca", SizeLabel: "8", Qty: 2, UnitPriceCents: 32000},
				{ItemName: "Playera polo blanca", SizeLabel: "10", Qty: 1, UnitPriceCents: 32000},
			},
			Fulfillments: []client.UniformFulfillment{{FinanceChargeID: "ch-9"}},
		},
		account: client.AccountStatement{Payments: []client.Payment{{ChargeID: "ch-9", Method: "transfer", Reference: "SPEI 55"}}},
	}

	got, err := builder(f).Sale(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("Sale: %v", err)
	}
	if got.Kind != receipt.KindSale || got.Folio != "U-2026-000007" || got.Buyer != "Zoe Villalobos" || got.PaidOn != "2026-09-25" {
		t.Errorf("head = %q %q %q %q", got.Kind, got.Folio, got.Buyer, got.PaidOn)
	}
	if len(got.Lines) != 2 || got.Lines[0].Description != "Playera polo blanca · talla 8 × 2" || got.Lines[0].AmountCents != 64000 {
		t.Errorf("lines = %+v", got.Lines)
	}
	if got.TotalCents != 96000 || got.Method != "Transferencia" || got.Reference != "SPEI 55" {
		t.Errorf("total %d, method %q, reference %q", got.TotalCents, got.Method, got.Reference)
	}
}

// A school that has not filled its details still gets a receipt.
func TestAReceiptWithoutTheSchoolsDetails(t *testing.T) {
	f := fakeSources{deposit: client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-3", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-10T00:00:00Z", Folio: "P-2026-000044",
	}}}
	got, err := builder(f).Payment(context.Background(), "dep-3")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if got.School.Name != "" || got.Method != "Efectivo" {
		t.Errorf("school %q, method %q", got.School.Name, got.Method)
	}
}

func TestASaleThatCannotBeReadIsAnError(t *testing.T) {
	f := fakeSources{orderErr: errors.New("boom")}
	if _, err := builder(f).Sale(context.Background(), "ord-x"); err == nil {
		t.Error("want the order's error")
	}
}

// Review of task 12: a "Comprobante de venta" proves a counter sale. A family
// order — unpaid, waiting, cancelled — is not one, and gets no such receipt.
func TestOnlyACounterSaleHasASaleReceipt(t *testing.T) {
	f := fakeSources{order: client.UniformOrder{ID: "ord-3", Folio: "U-2026-000010", Source: "family", TotalCents: 32000}}
	_, err := builder(f).Sale(context.Background(), "ord-3")
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a family order's sale receipt: %v, want NotFound", err)
	}
}

// Review of task 12: a discount on a surcharge line counts in the total of
// discounts too — the line and the totals say the same.
func TestEveryLinesDiscountCounts(t *testing.T) {
	f := fakeSources{deposit: client.DepositReceipt{
		Deposit: client.Deposit{ID: "dep-4", StudentID: "stu-1", AmountCents: 20000, Method: "cash", PaidAt: "2026-10-10T00:00:00Z", Folio: "P-2026-000045"},
		Lines:   []client.ReceiptLine{{Description: "Recargo", AmountCents: 20000, Surcharge: true, DiscountCents: 5000}},
	}}
	got, err := builder(f).Payment(context.Background(), "dep-4")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if got.DiscountCents != 5000 {
		t.Errorf("discounts = %d, want the surcharge line's 5000", got.DiscountCents)
	}
}

// T18: a month paid under a scholarship prints it, with its name, and the
// receipt's totals carry it apart from the discounts.
func TestAScholarshipIsItemized(t *testing.T) {
	f := fakeSources{deposit: client.DepositReceipt{
		Deposit: client.Deposit{ID: "dep-5", StudentID: "stu-1", AmountCents: 140000, Method: "cash", PaidAt: "2026-09-03T00:00:00Z", Folio: "P-2026-000046"},
		Lines: []client.ReceiptLine{{Description: "Colegiatura septiembre 2026", AmountCents: 140000, DiscountCents: 20000,
			ScholarshipCents: 140000, ScholarshipName: "Hermanos"}},
	}}
	got, err := builder(f).Payment(context.Background(), "dep-5")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if l := got.Lines[0]; l.ScholarshipCents != 140000 || l.ScholarshipName != "Hermanos" || l.DiscountCents != 20000 {
		t.Fatalf("line = %+v", l)
	}
	if got.ScholarshipCents != 140000 || got.DiscountCents != 20000 {
		t.Errorf("totals: scholarships %d discounts %d", got.ScholarshipCents, got.DiscountCents)
	}
}
