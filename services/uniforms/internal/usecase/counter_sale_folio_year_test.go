package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// T5b review: the receipt prints the school's day, so the folio must number
// under the school's YEAR. A sale at 19:00 on 31 December in Morelia is
// 01:00 UTC on 1 January — and is still U-2026-…, not U-2027-….
func TestCounterSaleIsNumberedInTheSchoolsYear(t *testing.T) {
	morelia, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		at   time.Time
		want int
	}{
		{"19:00 on 31 Dec in Morelia", time.Date(2026, time.December, 31, 19, 0, 0, 0, morelia), 2026},
		{"00:00 on 1 Jan in Morelia", time.Date(2027, time.January, 1, 0, 0, 0, 0, morelia), 2027},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.svc = usecase.NewService(f.repo, f.students, f.finance, f.pub, testTenant,
				seqID("id"), func() time.Time { return tt.at.UTC() }, morelia)
			item := f.seedItem(t, "PLA", "8", 32000, 5)

			got, err := f.svc.CounterSale(context.Background(), saleFor(item, "8", 1))
			if err != nil {
				t.Fatalf("CounterSale: %v", err)
			}
			if len(f.repo.sales) != 1 || f.repo.sales[0].FolioYear != tt.want {
				t.Fatalf("sales = %+v, want one numbered in %d", f.repo.sales, tt.want)
			}
			if prefix := fmt.Sprintf("U-%d-", tt.want); !strings.HasPrefix(got.Order.Folio(), prefix) {
				t.Errorf("folio = %q, want prefix %q", got.Order.Folio(), prefix)
			}
		})
	}
}

// T5b review: the finance charge of a counter sale falls due on the school's
// day — a sale at 19:00 on 31 October in Morelia is due 2026-10-31, the day
// its receipt prints, not 2026-11-01 (the UTC day).
func TestCounterSaleChargeFallsDueOnTheSchoolsDay(t *testing.T) {
	morelia, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.October, 31, 19, 0, 0, 0, morelia)
	f := newFixture(t)
	f.svc = usecase.NewService(f.repo, f.students, f.finance, f.pub, testTenant,
		seqID("id"), func() time.Time { return at.UTC() }, morelia)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	if _, err := f.svc.CounterSale(context.Background(), saleFor(item, "8", 1)); err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if len(f.finance.charges) != 1 || f.finance.charges[0].DueDate != "2026-10-31" {
		t.Fatalf("charges = %+v, want one due 2026-10-31", f.finance.charges)
	}
}
