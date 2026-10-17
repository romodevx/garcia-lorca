//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Each test here owns its tenant: the overlap rule is tenant-wide, so a cycle
// another test left in a shared tenant would decide the outcome.
const (
	tenantOverlap     = "c1c1c1c1-0000-4000-8000-000000000001"
	tenantOverlapRace = "c1c1c1c1-0000-4000-8000-000000000002"
	tenantRename      = "c1c1c1c1-0000-4000-8000-000000000003"
	tenantClosures    = "c1c1c1c1-0000-4000-8000-000000000004"
	tenantScoped      = "c1c1c1c1-0000-4000-8000-000000000006"
)

// insertCycleFixture stores c as the app role WITHOUT the overlap rule.
//
// Most suites here only need "a cycle exists" to hang terms, groups or periods
// off, and they all share tenant A with the same dates. Seeding through
// CreateCycle would make each one depend on which suite ran first; the rule
// itself is proven by the tests in this file, each in its own tenant.
func insertCycleFixture(t *testing.T, c domain.Cycle) error {
	t.Helper()
	return appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", c.TenantID()).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on) VALUES (?, ?, ?, ?, ?, ?)`,
			c.ID(), c.TenantID(), c.Name(), c.Status().String(), c.StartsOn(), c.EndsOn()).Error
	})
}

func ymd(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func newCycle(t *testing.T, tenant, id, name string, from, to time.Time) domain.Cycle {
	t.Helper()
	c, err := domain.NewCycle(id, tenant, name, domain.CycleStatusDraft, from, to)
	require.NoError(t, err)
	return c
}

// CC-101: a year inside the live one was stored and could be activated.
func TestCreateCycleRefusesDatesSharedWithAnotherCycle(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantOverlap)
	require.NoError(t, repo.CreateCycle(ctx, newCycle(t, tenantOverlap,
		"c1c1c1c1-0001-4000-8000-000000000001", "2026-2027", ymd(2026, time.August, 24), ymd(2027, time.July, 9))))

	err := repo.CreateCycle(ctx, newCycle(t, tenantOverlap,
		"c1c1c1c1-0001-4000-8000-000000000002", "2027", ymd(2027, time.January, 10), ymd(2027, time.December, 20)))

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeCycleOverlap, domain.RefusalCodeOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), `"2026-2027"`)
	cycles, lerr := repo.ListCycles(ctx)
	require.NoError(t, lerr)
	assert.Len(t, cycles, 1, "the refused cycle must not be stored")

	// The next year, starting the day after, is fine.
	require.NoError(t, repo.CreateCycle(ctx, newCycle(t, tenantOverlap,
		"c1c1c1c1-0001-4000-8000-000000000003", "2027-2028", ymd(2027, time.July, 10), ymd(2028, time.July, 7))))
}

// Another school's year never blocks this school's.
func TestCreateCycleOverlapIsScopedToTheTenant(t *testing.T) {
	ctx := context.Background()
	other := pgadapter.NewRepository(appDB(t), tenantOverlapRace)
	require.NoError(t, other.CreateCycle(ctx, newCycle(t, tenantOverlapRace,
		"c1c1c1c1-0002-4000-8000-000000000001", "Otra escuela", ymd(2030, time.August, 1), ymd(2031, time.July, 1))))

	mine := pgadapter.NewRepository(appDB(t), tenantScoped)
	require.NoError(t, mine.CreateCycle(ctx, newCycle(t, tenantScoped,
		"c1c1c1c1-0002-4000-8000-000000000002", "2030-2031", ymd(2030, time.August, 1), ymd(2031, time.July, 1))))
}

// Two overlapping years created at the same moment: the tenant lock makes the
// second one see the first, so exactly one is stored.
func TestCreateCycleOverlapHoldsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	const tenant = "c1c1c1c1-0000-4000-8000-000000000005"
	repo := pgadapter.NewRepository(appDB(t), tenant)
	ids := []string{"c1c1c1c1-0003-4000-8000-000000000001", "c1c1c1c1-0003-4000-8000-000000000002",
		"c1c1c1c1-0003-4000-8000-000000000003", "c1c1c1c1-0003-4000-8000-000000000004"}

	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			errs[i] = repo.CreateCycle(ctx, newCycle(t, tenant, id, "Ciclo "+id[len(id)-1:],
				ymd(2026, time.August, 24+i), ymd(2027, time.July, 9)))
		}(i, id)
	}
	wg.Wait()

	stored := 0
	for _, err := range errs {
		if err == nil {
			stored++
			continue
		}
		assert.Equal(t, domain.CodeCycleOverlap, domain.RefusalCodeOf(err), "unexpected error: %v", err)
	}
	assert.Equal(t, 1, stored)
}

// CC-103: a draft year could not be renamed.
func TestRenameCycle(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantRename)
	first := newCycle(t, tenantRename, "c1c1c1c1-0004-4000-8000-000000000001", "2026-2027", ymd(2026, time.August, 24), ymd(2027, time.July, 9))
	second := newCycle(t, tenantRename, "c1c1c1c1-0004-4000-8000-000000000002", "Borrador", ymd(2027, time.August, 23), ymd(2028, time.July, 7))
	require.NoError(t, repo.CreateCycle(ctx, first))
	require.NoError(t, repo.CreateCycle(ctx, second))

	renamed, err := second.Rename("2027-2028")
	require.NoError(t, err)
	require.NoError(t, repo.RenameCycle(ctx, renamed))
	got, err := repo.GetCycle(ctx, second.ID())
	require.NoError(t, err)
	assert.Equal(t, "2027-2028", got.Name())
	assert.Equal(t, second.StartsOn(), got.StartsOn())

	taken, err := second.Rename("2026-2027")
	require.NoError(t, err)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(repo.RenameCycle(ctx, taken)))

	ghost := newCycle(t, tenantRename, "c1c1c1c1-0004-4000-8000-0000000000ff", "x", ymd(2040, time.August, 1), ymd(2041, time.July, 1))
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repo.RenameCycle(ctx, ghost)))

	// RLS: another school cannot rename this school's year.
	stranger := pgadapter.NewRepository(appDB(t), tenantOverlapRace)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(stranger.RenameCycle(ctx, renamed)))
}

// CC-106: the same holiday entered twice under two labels was stored twice.
func TestCreateNonWorkingPeriodRefusesADayAlreadyClosed(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantClosures)
	const cycleID = "c1c1c1c1-0005-4000-8000-000000000001"
	require.NoError(t, repo.CreateCycle(ctx, newCycle(t, tenantClosures, cycleID, "2026-2027", ymd(2026, time.August, 24), ymd(2027, time.July, 9))))
	nov16 := ymd(2026, time.November, 16)
	require.NoError(t, repo.CreateNonWorkingPeriod(ctx,
		nonWorking(t, "c1c1c1c1-0005-4000-8000-000000000002", tenantClosures, cycleID, "Revolución", domain.KindHoliday, nov16, nov16)))

	err := repo.CreateNonWorkingPeriod(ctx,
		nonWorking(t, "c1c1c1c1-0005-4000-8000-000000000003", tenantClosures, cycleID, "Día de la Revolución", domain.KindHoliday, nov16, nov16))

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeNonWorkingOverlap, domain.RefusalCodeOf(err))
	periods, lerr := repo.ListNonWorkingPeriods(ctx, cycleID)
	require.NoError(t, lerr)
	assert.Len(t, periods, 1)

	nov17 := ymd(2026, time.November, 17)
	require.NoError(t, repo.CreateNonWorkingPeriod(ctx,
		nonWorking(t, "c1c1c1c1-0005-4000-8000-000000000004", tenantClosures, cycleID, "Puente", domain.KindHoliday, nov17, nov17)))
}
