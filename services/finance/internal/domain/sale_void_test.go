package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// C2-015: only a uniforms counter sale's own payments, taken today, are voided
// by a return. A deposit's slice is corrected through its deposit; an earlier
// day's money is in a cash-up that may be closed.
func TestCheckSaleVoid(t *testing.T) {
	today := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		ref      string
		payments []domain.SalePayment
		want     glerrors.Kind
	}{
		{"today's sale", "uniforms:f1", []domain.SalePayment{{PaidOn: today}}, glerrors.KindUnknown},
		{"not a sale", "tuition:x", []domain.SalePayment{{PaidOn: today}}, glerrors.KindInvalid},
		{"paid through a deposit", "uniforms:f1", []domain.SalePayment{{PaidOn: today, InDeposit: true}}, glerrors.KindConflict},
		{"an earlier day", "uniforms:f1", []domain.SalePayment{{PaidOn: today.AddDate(0, 0, -1)}}, glerrors.KindConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.CheckSaleVoid(tc.ref, tc.payments, today)
			if tc.want == glerrors.KindUnknown {
				if err != nil {
					t.Fatalf("err = %v, want none", err)
				}
				return
			}
			if glerrors.KindOf(err) != tc.want {
				t.Fatalf("err = %v, want kind %v", err, tc.want)
			}
		})
	}
}
