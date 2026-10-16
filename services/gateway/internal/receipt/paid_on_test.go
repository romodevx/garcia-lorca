package receipt_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// ADR 0035: "Fecha de pago" is the deposit's school-local day (paid_on). A
// live payment at 19:00 in Morelia on the 15th is 01:00 UTC on the 16th; the
// UTC date of paid_at would print the 16th.
func TestTheReceiptDatesThePaymentOnItsDay(t *testing.T) {
	cases := []struct {
		name   string
		paidAt string
		paidOn string
		want   string
	}{
		{"a live evening payment", "2026-10-16T01:00:00Z", "2026-10-15", "2026-10-15"},
		{"a finance that does not send the day yet", "2026-10-10T00:00:00Z", "", "2026-10-10"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := fakeSources{
				profile: school, found: true,
				student: client.StudentResult{FirstName: "Camila", LastName: "Zamora"},
				deposit: client.DepositReceipt{Deposit: client.Deposit{
					ID: "dep-9", StudentID: "stu-1", AmountCents: 100, Method: "cash",
					PaidAt: tt.paidAt, PaidOn: tt.paidOn, Folio: "P-2026-000090",
				}},
			}
			got, err := builder(f).Payment(context.Background(), "dep-9")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.PaidOn)
		})
	}
}

// Review of D8: a counter sale at 19:00 in Morelia on the 15th is 01:00 UTC
// on the 16th; the sale receipt prints the school's day, not the UTC date.
func TestTheSaleReceiptDatesTheSaleOnTheSchoolsDay(t *testing.T) {
	morelia, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	f := fakeSources{
		profile: school, found: true,
		order: client.UniformOrder{
			ID: "ord-9", Folio: "U-2026-000090", Source: "counter",
			TotalCents: 32000, CreatedAt: "2026-10-16T01:00:00Z",
		},
	}
	b := receipt.NewBuilder(f, f, f, f, f, receipt.WithSchoolLocation(morelia))

	got, err := b.Sale(context.Background(), "ord-9")

	require.NoError(t, err)
	assert.Equal(t, "2026-10-15", got.PaidOn)
}
