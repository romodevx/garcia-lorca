//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The deposit rules of the QA of 17 October 2026 (CC-201, CC-202) against
// real SQL, through the use case that owns them. Every id starts with "cc2" so
// no other test of the shared container collides; each test has its tenant.

// knownStudents answers every student as an active one: the deposit asks.
type knownStudents struct{}

func (knownStudents) ByID(_ context.Context, id string) (ports.StudentRef, error) {
	return ports.StudentRef{ID: id, Status: "active"}, nil
}

// noEnrollments places nobody in a group: no tuition table prices them.
type noEnrollments struct{}

func (noEnrollments) ListByCycle(context.Context, string, string) ([]ports.EnrollmentRef, error) {
	return nil, nil
}

func (noEnrollments) ListByStudent(context.Context, string) ([]ports.EnrollmentRef, error) {
	return nil, nil
}

// counterDay is the school's today in these tests: before every due date
// below, so nothing is late and no surcharge enters.
var counterDay = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

// depositService is the use case over the real repository, scholarships on.
func depositService(t *testing.T, repo *pgadapter.Repository, tenant string) *usecase.FinanceService {
	t.Helper()
	svc := usecase.NewFinanceService(repo, knownStudents{}, nil, noEnrollments{}, nil, nil, nil, tenant,
		uuid.NewString, usecase.NewSchoolClock(func() time.Time { return counterDay.Add(12 * time.Hour) }, time.UTC))
	return svc.WithScholarships(repo)
}

func payChosen(svc *usecase.FinanceService, student, charge, key string, cents int64) (usecase.DepositResult, error) {
	return svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: student, AmountCents: cents, Method: domain.MethodTransfer, PaidAt: counterDay,
		ChargeIDs: []string{charge}, IdempotencyKey: key,
	})
}

// CC-202: two cashiers take the same month at the same time. Both quotes said
// it was owed; the lock serialises them, and the second one, planned after the
// first committed, finds it paid: 409 in Spanish, and NOTHING stored — no
// deposit, no folio, no credit that no charge will ever take.
func TestTwoCashiersOnTheSameChargeOneIsRefused(t *testing.T) {
	ctx := context.Background()
	const (
		tenant  = "cc202000-0000-0000-0000-000000000001"
		student = "cc202000-0000-0000-0000-0000000000a1"
		charge  = "cc202000-0000-0000-0000-0000000000c1"
	)
	seedConceptAndCharge(t, tenant, "cc202000-0000-0000-0000-0000000000b1", "NATACION QA", charge, student,
		"cc202000-0000-0000-0000-0000000000e1", 25000, 0, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	repo := appRepo(t, tenant)
	svc := depositService(t, repo, tenant)

	// Hold the charge's row so both cashiers are planned against the same
	// unpaid month and queue on the lock — a real race, not two turns.
	locked := superDB.Begin()
	require.NoError(t, locked.Error)
	var lockedID string
	require.NoError(t, locked.Raw("SELECT id FROM charges WHERE id = ? FOR UPDATE", charge).Scan(&lockedID).Error)

	const cashiers = 2
	results := make(chan error, cashiers)
	var wg sync.WaitGroup
	for i := range cashiers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := payChosen(svc, student, charge, uuid.NewString(), 25000)
			results <- err
		}(i)
	}
	if err := awaitLockWaiters(cashiers); err != nil {
		_ = locked.Rollback().Error
		t.Fatal(err)
	}
	require.NoError(t, locked.Commit().Error)
	wg.Wait()
	close(results)

	var ok, refused int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case glerrors.KindOf(err) == glerrors.KindConflict:
			refused++
			assert.Contains(t, glerrors.SafeMessage(err), "ya está pagado")
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, ok, "one cashier takes the money")
	assert.Equal(t, 1, refused, "the other is told it is paid")

	deposits, err := repo.ListDeposits(ctx, student)
	require.NoError(t, err)
	assert.Len(t, deposits, 1, "the refused deposit is not stored")
	credit, err := repo.UnallocatedCents(ctx, student)
	require.NoError(t, err)
	assert.Zero(t, credit, "no orphan credit")
}

// A double click: two requests with the SAME idempotency key at the same
// instant. Both look the key up before either has stored it, and both queue on
// the charges' lock. The second one, let through after the first committed,
// must find the key again and answer the original — a replay — not a 409 from
// the unique index nor the CC-202 refusal ("ya está pagado").
func TestASimultaneousDoubleClickIsAReplay(t *testing.T) {
	ctx := context.Background()
	const (
		tenant  = "cc203000-0000-0000-0000-000000000001"
		student = "cc203000-0000-0000-0000-0000000000a1"
		charge  = "cc203000-0000-0000-0000-0000000000c1"
	)
	seedConceptAndCharge(t, tenant, "cc203000-0000-0000-0000-0000000000b1", "COLEGIATURA QA", charge, student,
		"cc203000-0000-0000-0000-0000000000e1", 320000, 0, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	repo := appRepo(t, tenant)
	svc := depositService(t, repo, tenant)

	locked := superDB.Begin()
	require.NoError(t, locked.Error)
	var lockedID string
	require.NoError(t, locked.Raw("SELECT id FROM charges WHERE id = ? FOR UPDATE", charge).Scan(&lockedID).Error)

	const clicks = 2
	type outcome struct {
		res usecase.DepositResult
		err error
	}
	results := make(chan outcome, clicks)
	var wg sync.WaitGroup
	for range clicks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := payChosen(svc, student, charge, "cc203-same-click", 320000)
			results <- outcome{res, err}
		}()
	}
	if err := awaitLockWaiters(clicks); err != nil {
		_ = locked.Rollback().Error
		t.Fatal(err)
	}
	require.NoError(t, locked.Commit().Error)
	wg.Wait()
	close(results)

	var ids []string
	replays := 0
	for o := range results {
		require.NoError(t, o.err)
		ids = append(ids, o.res.Deposit.ID())
		if o.res.Replayed {
			replays++
		}
	}
	require.Len(t, ids, clicks)
	assert.Equal(t, ids[0], ids[1], "both clicks answer the same deposit")
	assert.Equal(t, 1, replays, "the second click is the replay")
	deposits, err := repo.ListDeposits(ctx, student)
	require.NoError(t, err)
	assert.Len(t, deposits, 1)
}

// awaitLockWaiters waits until n sessions wait on a row lock of charges.
func awaitLockWaiters(n int) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiters int64
		if err := superDB.Raw(`SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query ILIKE '%charges%'`).Scan(&waiters).Error; err != nil {
			return err
		}
		if waiters >= int64(n) {
			return nil
		}
		if time.Now().After(deadline) {
			return glerrors.New(glerrors.KindInternal, "the cashiers never queued on the charge lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
