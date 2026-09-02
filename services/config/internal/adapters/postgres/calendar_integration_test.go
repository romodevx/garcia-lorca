//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

const (
	calendarCycleA = "caca0001-0000-0000-0000-000000000001"
	calendarCycleB = "caca0001-0000-0000-0000-000000000002"
)

func seedCalendarCycle(t *testing.T, repo *pgadapter.Repository, tenant, id, name string) {
	t.Helper()
	cycle, err := domain.NewCycle(id, tenant, name, domain.CycleStatusDraft, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(context.Background(), cycle); err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
}

func nonWorking(t *testing.T, id, tenant, cycleID, label, kind string, from, to time.Time) domain.NonWorkingPeriod {
	t.Helper()
	p, err := domain.NewNonWorkingPeriod(id, tenant, cycleID, label, kind, from, to)
	if err != nil {
		t.Fatalf("NewNonWorkingPeriod: %v", err)
	}
	return p
}

// A holiday goes in as a date and has to come back as the same date, in order,
// and be removable. The whole collection rule hangs off these days being right.
func TestNonWorkingPeriodsRoundTripInOrder(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	seedCalendarCycle(t, repo, tenantA, calendarCycleA, "Calendar-Cycle-A")

	sep16 := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	nov20 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if err := repo.CreateNonWorkingPeriod(ctx,
		nonWorking(t, "caca0002-0000-0000-0000-000000000002", tenantA, calendarCycleA, "Revolución", domain.KindHoliday, nov20, nov20)); err != nil {
		t.Fatalf("CreateNonWorkingPeriod(Revolución): %v", err)
	}
	if err := repo.CreateNonWorkingPeriod(ctx,
		nonWorking(t, "caca0002-0000-0000-0000-000000000001", tenantA, calendarCycleA, "Independencia", domain.KindHoliday, sep16, sep16)); err != nil {
		t.Fatalf("CreateNonWorkingPeriod(Independencia): %v", err)
	}

	periods, err := repo.ListNonWorkingPeriods(ctx, calendarCycleA)
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods: %v", err)
	}
	if len(periods) != 2 {
		t.Fatalf("listed %d periods, want 2", len(periods))
	}
	// Earliest first: a person reads this as a calendar, not as an insert log.
	if periods[0].Label() != "Independencia" {
		t.Errorf("order wrong: %s came first", periods[0].Label())
	}
	if !periods[0].From().Equal(sep16) || !periods[0].To().Equal(sep16) {
		t.Errorf("dates did not round-trip: %v..%v", periods[0].From(), periods[0].To())
	}

	// The same closure entered twice is a mistake, not two closures.
	dup := nonWorking(t, "caca0002-0000-0000-0000-00000000000f", tenantA, calendarCycleA, "Independencia", domain.KindHoliday, sep16, sep16)
	if err := repo.CreateNonWorkingPeriod(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate label: want KindConflict, got %v", err)
	}

	if err := repo.DeleteNonWorkingPeriod(ctx, periods[0].ID()); err != nil {
		t.Fatalf("DeleteNonWorkingPeriod: %v", err)
	}
	if err := repo.DeleteNonWorkingPeriod(ctx, periods[0].ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("second delete: want KindNotFound, got %v", err)
	}
}

// RLS, same posture as every other tenant-scoped table here: another school
// cannot read this one's calendar, and cannot delete a day out of it.
func TestRLSIsolatesTheSchoolCalendar(t *testing.T) {
	ctx := context.Background()
	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	seedCalendarCycle(t, repoA, tenantA, calendarCycleB, "Calendar-Cycle-B")

	day := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	periodID := "caca0003-0000-0000-0000-000000000001"
	if err := repoA.CreateNonWorkingPeriod(ctx,
		nonWorking(t, periodID, tenantA, calendarCycleB, "Navidad", domain.KindVacation, day, day)); err != nil {
		t.Fatalf("CreateNonWorkingPeriod: %v", err)
	}

	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	seen, err := repoB.ListNonWorkingPeriods(ctx, calendarCycleB)
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods as tenant B: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("tenant B read %d of tenant A's closed days", len(seen))
	}
	if err := repoB.DeleteNonWorkingPeriod(ctx, periodID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B deleting tenant A's day: want KindNotFound, got %v", err)
	}
}
