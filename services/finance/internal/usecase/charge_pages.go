package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// MaxPageSize is both the default and the largest page of a charge walk. It
// bounds what one round trip holds in memory, NOT what a caller can read: the
// walk goes on until Next comes back zero.
const MaxPageSize = 500

// maxWalkPages stops a whole-set walk that never ends — a repository handing
// back the same cursor forever — with an error instead of a hang. 1,000 pages
// of 500 is half a million charges, far past any school.
const maxWalkPages = 1000

// pageSize is the size actually asked of the repository: the default for zero
// or less, clamped to the maximum above it.
func pageSize(requested int) int {
	if requested <= 0 || requested > MaxPageSize {
		return MaxPageSize
	}
	return requested
}

// ListChargesPage returns one page of charges with what has been collected
// against each, narrowed by student and/or cycle (either filter alone is
// enough; an empty one means "any"), in (due_date, id) order after `after`.
//
// At least one filter is REQUIRED: an unscoped read would hand the caller the
// tenant's whole ledger, so it is refused as bad input rather than served. The
// page is priced today, as the counter and the statement read it (T18).
//
// The read is paged because a cycle is thousands of charges (567 pupils times
// nine months is 5,103): the unpaged read stopped at 500 and the "Por cobrar"
// figure was silently a tenth of the truth.
func (s *FinanceService) ListChargesPage(ctx context.Context, studentID, cycleID string, after ports.ChargeCursor, size int) (ports.ChargePage, error) {
	if studentID == "" && cycleID == "" {
		return ports.ChargePage{}, glerrors.New(glerrors.KindInvalid, "specify a student or a cycle")
	}
	page, err := s.repo.ListChargesPage(ctx, studentID, cycleID, after, pageSize(size))
	if err != nil {
		return ports.ChargePage{}, err
	}
	pricer, err := s.pagePricer(ctx, studentID, page.Charges)
	if err != nil {
		return ports.ChargePage{}, err
	}
	priced, err := pricer.priceAll(page.Charges, s.clock.Today())
	if err != nil {
		return ports.ChargePage{}, err
	}
	return ports.ChargePage{Charges: priced, Next: page.Next}, nil
}

// ListOverduePage returns one page of the charges past due that still owe
// money as of the school's today. An empty cycleID spans every cycle.
//
// The page examines `size` candidates; those a payment or a scholarship has
// settled are dropped (T18: a month a scholarship settled is not a debt), so
// the page may be short or empty while Next still points further on.
func (s *FinanceService) ListOverduePage(ctx context.Context, cycleID string, after ports.ChargeCursor, size int) (ports.ChargePage, error) {
	return s.overduePage(ctx, cycleID, s.clock.Today(), after, pageSize(size))
}

// overduePage is one page of the overdue set as of asOf, with the charges a
// payment or a scholarship settled dropped (T18). The screen's page and the
// billing run's whole walk (allOverdue) both read it, so they share one pricing
// and one still-owed rule. Next is the repository's: the last candidate
// examined, so an emptied page never ends the walk.
func (s *FinanceService) overduePage(ctx context.Context, cycleID string, asOf time.Time, after ports.ChargeCursor, size int) (ports.ChargePage, error) {
	page, err := s.repo.ListOverduePage(ctx, cycleID, asOf, after, size)
	if err != nil {
		return ports.ChargePage{}, err
	}
	pricer, err := s.pagePricer(ctx, "", page.Charges)
	if err != nil {
		return ports.ChargePage{}, err
	}
	owed, err := stillOwed(pricer, page.Charges, asOf)
	if err != nil {
		return ports.ChargePage{}, err
	}
	return ports.ChargePage{Charges: owed, Next: page.Next}, nil
}

// pagePricer prices one page's pupils.
func (s *FinanceService) pagePricer(ctx context.Context, studentID string, charges []ports.ChargeWithPaid) (*scholarshipPricer, error) {
	ids, err := s.conceptIDs(ctx)
	if err != nil {
		return nil, err
	}
	return s.pricerFor(ctx, studentsOf(studentID, charges), ids)
}

// walkPages follows a keyset walk until Next comes back zero, never stopping on
// a short or empty page.
func walkPages(read func(after ports.ChargeCursor) (ports.ChargePage, error)) ([]ports.ChargeWithPaid, error) {
	var (
		out    []ports.ChargeWithPaid
		cursor ports.ChargeCursor
	)
	for range maxWalkPages {
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
	return nil, glerrors.New(glerrors.KindInternal, "charge walk did not end")
}
