package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T7: every field of a ledger line crosses the wire. Same reason as the tuition
// round trip — a field the adapter forgets is a zero nobody notices.
func TestAStatementSurvivesTheWireWhole(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	concept := f.seedConcept(t, "Colegiatura", 100000)
	charge := f.seedCharge(t, concept.GetId(), 100000)
	if _, err := f.h.ApplyDiscount(ctx, connect.NewRequest(&financev1.ApplyDiscountRequest{
		ChargeId: charge.GetId(), DiscountCents: 10000, Reason: domain.ReasonLevelDiscount,
	})); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}
	later, err := domain.NewCharge("ch-later", testTenant, testStudent, testCycle, concept.GetId(),
		"Inscripción 2027", 50000, 0, fixedNow.AddDate(0, 3, 0))
	if err != nil {
		t.Fatalf("NewCharge: %v", err)
	}
	if later, err = later.WithStartsOn(fixedNow.AddDate(0, 2, 0)); err != nil {
		t.Fatalf("WithStartsOn: %v", err)
	}
	f.repo.charges[later.ID()] = later

	paidAt := time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)
	d, err := domain.NewDeposit("dep-1", testTenant, testStudent, 30000, domain.MethodCard, "voucher-9", paidAt, "", "cal")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	if d, err = d.WithCardType("debit"); err != nil {
		t.Fatalf("WithCardType: %v", err)
	}
	if d, err = d.WithFolio("P-2026-000007").Void("tecleado dos veces", paidAt); err != nil {
		t.Fatalf("Void: %v", err)
	}
	f.repo.deposits = []domain.Deposit{d}

	res, err := f.h.GetStatement(ctx, connect.NewRequest(&financev1.GetStatementRequest{StudentId: testStudent}))
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	byKey := map[string]*financev1.StatementMovement{}
	for _, m := range res.Msg.GetMovements() {
		byKey[m.GetKind()+"/"+m.GetChargeId()+m.GetDepositId()] = m
	}

	c := byKey["charge/"+charge.GetId()]
	if c == nil || c.GetAmountCents() != 100000 || c.GetDate() != charge.GetDueDate() || c.GetDescription() == "" || c.GetBalanceCents() != 100000 {
		t.Errorf("charge line = %+v", c)
	}
	if dl := byKey["discount/"+charge.GetId()]; dl == nil || dl.GetAmountCents() != 10000 || dl.GetBalanceCents() != 90000 {
		t.Errorf("discount line = %+v", dl)
	}
	if nl := byKey["charge/ch-later"]; nl == nil || !nl.GetNotYetDue() {
		t.Errorf("unopened line = %+v", nl)
	}
	p := byKey["payment/dep-1"]
	if p == nil || p.GetMethod() != domain.MethodCard || p.GetCardType() != "debit" || p.GetReference() != "voucher-9" ||
		p.GetFolio() != "P-2026-000007" || !p.GetVoided() || p.GetVoidReason() != "tecleado dos veces" ||
		p.GetDate() != "2026-09-06" || p.GetAmountCents() != 30000 {
		t.Errorf("deposit line = %+v", p)
	}
	tot := res.Msg.GetTotals()
	if tot.GetChargedCents() != 100000 || tot.GetDiscountCents() != 10000 || tot.GetPaidCents() != 0 ||
		tot.GetBalanceCents() != 90000 || tot.GetNotYetDueCents() != 50000 {
		t.Errorf("totals = %+v", tot)
	}
	if res.Msg.GetAsOf() == "" {
		t.Error("as_of is empty")
	}

	family, err := f.h.GetStatement(ctx, connect.NewRequest(&financev1.GetStatementRequest{StudentId: testStudent, FamilyView: true}))
	if err != nil {
		t.Fatalf("GetStatement(family): %v", err)
	}
	for _, m := range family.Msg.GetMovements() {
		if m.GetNotYetDue() || m.GetVoided() {
			t.Errorf("the family is shown %+v", m)
		}
	}
}
