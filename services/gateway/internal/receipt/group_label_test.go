package receipt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

type fakeLabeler struct {
	label string
	found bool
	err   error
	asked []string
	days  []string
}

func (f *fakeLabeler) StudentGroupLabel(_ context.Context, studentID, day string) (string, bool, error) {
	f.asked = append(f.asked, studentID)
	f.days = append(f.days, day)
	return f.label, f.found, f.err
}

// ISSUE-017: the receipt names the pupil's group as every screen does —
// grade, level and letter — not the bare letter the pupil's row stores.
func TestTheReceiptNamesTheGroupWithItsGrade(t *testing.T) {
	deposit := client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-1", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-10T00:00:00Z", Folio: "P-2026-000050",
	}}
	order := client.UniformOrder{ID: "ord-1", Folio: "U-2026-000011", Source: "counter", StudentID: "stu-1",
		StudentName: "Zoe Villalobos", StudentGradeGroup: "A", CreatedAt: "2026-09-25T06:36:26Z"}
	cases := []struct {
		name    string
		labeler *fakeLabeler
		want    string
	}{
		{"the group as the school names it", &fakeLabeler{label: "1° PRIMARIA · A", found: true}, "1° PRIMARIA · A"},
		{"a pupil with no active group keeps the stored letter", &fakeLabeler{}, "A"},
		{"a catalog that cannot answer still prints the receipt", &fakeLabeler{err: errors.New("config down")}, "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeSources{deposit: deposit, order: order, student: client.StudentResult{FirstName: "Camila", LastName: "Zamora", GradeGroup: "A"}}
			b := receipt.NewBuilder(f, f, f, f, f, receipt.WithGroupLabeler(tc.labeler))

			pay, err := b.Payment(context.Background(), "dep-1")
			if err != nil {
				t.Fatalf("Payment: %v", err)
			}
			sale, err := b.Sale(context.Background(), "ord-1")
			if err != nil {
				t.Fatalf("Sale: %v", err)
			}
			if pay.BuyerDetail != tc.want || sale.BuyerDetail != tc.want {
				t.Errorf("payment %q, sale %q; want %q", pay.BuyerDetail, sale.BuyerDetail, tc.want)
			}
			if len(tc.labeler.asked) != 2 || tc.labeler.asked[0] != "stu-1" {
				t.Errorf("asked for %v, want the pupil twice", tc.labeler.asked)
			}
			// The group of the day the money came: the payment's day, the sale's school day.
			if len(tc.labeler.days) != 2 || tc.labeler.days[0] != "2026-10-10" || tc.labeler.days[1] != "2026-09-25" {
				t.Errorf("asked for days %v, want [2026-10-10 2026-09-25]", tc.labeler.days)
			}
		})
	}
}

// A walk-in sale has no pupil: nobody is asked for a group.
func TestAWalkInSaleAsksForNoGroup(t *testing.T) {
	labeler := &fakeLabeler{label: "1° PRIMARIA · A", found: true}
	f := fakeSources{order: client.UniformOrder{ID: "ord-2", Folio: "U-2026-000012", Source: "counter", CreatedAt: "2026-09-25T06:36:26Z"}}
	got, err := receipt.NewBuilder(f, f, f, f, f, receipt.WithGroupLabeler(labeler)).Sale(context.Background(), "ord-2")
	if err != nil {
		t.Fatalf("Sale: %v", err)
	}
	if got.BuyerDetail != "" || len(labeler.asked) != 0 {
		t.Errorf("detail %q, asked %v; want none", got.BuyerDetail, labeler.asked)
	}
}
