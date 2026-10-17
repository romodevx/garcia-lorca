package usecase_test

import (
	"context"
	"errors"

	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// maxTestPages ends a test walk that would never end on its own.
const maxTestPages = 100

// allCharges is every charge of the filter, read the way a client reads it:
// ListChargesPage followed until Next is zero.
func allCharges(ctx context.Context, svc *usecase.FinanceService, studentID, cycleID string) ([]ports.ChargeWithPaid, error) {
	return walkTestPages(func(after ports.ChargeCursor) (ports.ChargePage, error) {
		return svc.ListChargesPage(ctx, studentID, cycleID, after, usecase.MaxPageSize)
	})
}

// allOverdue is every overdue charge still owed, ListOverduePage walked to the
// end.
func allOverdue(ctx context.Context, svc *usecase.FinanceService, cycleID string) ([]ports.ChargeWithPaid, error) {
	return walkTestPages(func(after ports.ChargeCursor) (ports.ChargePage, error) {
		return svc.ListOverduePage(ctx, cycleID, after, usecase.MaxPageSize)
	})
}

func walkTestPages(read func(after ports.ChargeCursor) (ports.ChargePage, error)) ([]ports.ChargeWithPaid, error) {
	var (
		out    []ports.ChargeWithPaid
		cursor ports.ChargeCursor
	)
	for range maxTestPages {
		page, err := read(cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Charges...)
		if page.Next.IsZero() {
			return out, nil
		}
		cursor = page.Next
	}
	return nil, errors.New("test walk did not end")
}
