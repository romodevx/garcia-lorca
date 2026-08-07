package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

var paidAt = time.Date(2026, 9, 3, 10, 30, 0, 0, time.UTC)

func TestNewPaymentAccessors(t *testing.T) {
	p, err := domain.NewPayment("pay-1", "t1", "ch-1", 200000, domain.MethodTransfer, "SPEI-8891", paidAt)
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	if p.ID() != "pay-1" || p.TenantID() != "t1" || p.ChargeID() != "ch-1" ||
		p.Amount().Int64() != 200000 || p.Method() != domain.MethodTransfer ||
		p.Reference() != "SPEI-8891" || !p.PaidAt().Equal(paidAt) {
		t.Errorf("accessors mismatch: %+v", p)
	}
}

func TestNewPaymentMethods(t *testing.T) {
	for _, m := range []string{domain.MethodCash, domain.MethodCard, domain.MethodTransfer} {
		if _, err := domain.NewPayment("pay-1", "t1", "ch-1", 100, m, "", paidAt); err != nil {
			t.Errorf("method %q: want accepted, got %v", m, err)
		}
	}
}

func TestNewPaymentEmptyReferenceAllowed(t *testing.T) {
	// Cash at the front desk carries no reference; it is optional by design.
	p, err := domain.NewPayment("pay-1", "t1", "ch-1", 100, domain.MethodCash, "", paidAt)
	if err != nil {
		t.Fatalf("NewPayment with empty reference: %v", err)
	}
	if p.Reference() != "" {
		t.Errorf("Reference() = %q, want empty", p.Reference())
	}
}

func TestNewPaymentValidation(t *testing.T) {
	cases := []struct {
		name                   string
		id, tenantID, chargeID string
		amount                 int64
		method                 string
		at                     time.Time
	}{
		{"empty id", "", "t1", "ch-1", 100, domain.MethodCash, paidAt},
		{"blank id", "  ", "t1", "ch-1", 100, domain.MethodCash, paidAt},
		{"empty tenant", "pay-1", "", "ch-1", 100, domain.MethodCash, paidAt},
		{"empty charge", "pay-1", "t1", "", 100, domain.MethodCash, paidAt},
		{"zero amount", "pay-1", "t1", "ch-1", 0, domain.MethodCash, paidAt},
		{"negative amount", "pay-1", "t1", "ch-1", -100, domain.MethodCash, paidAt},
		{"unknown method", "pay-1", "t1", "ch-1", 100, "crypto", paidAt},
		{"empty method", "pay-1", "t1", "ch-1", 100, "", paidAt},
		{"zero paid at", "pay-1", "t1", "ch-1", 100, domain.MethodCash, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewPayment(tc.id, tc.tenantID, tc.chargeID, tc.amount, tc.method, "ref", tc.at)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}
