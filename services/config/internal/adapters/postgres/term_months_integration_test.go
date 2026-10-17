//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

type termMonthsRig struct {
	repo           *pgadapter.Repository
	tenant         string
	cycleID        string
	levelID        string
	otherLevelID   string
	t1, t2, tOther domain.Term
}

func ym(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

func monthsOf(t domain.Term) []string {
	out := make([]string, 0, len(t.Months()))
	for _, m := range t.Months() {
		out = append(out, domain.FormatMonth(m))
	}
	return out
}

func withMonths(t *testing.T, term domain.Term, ms ...time.Time) domain.Term {
	t.Helper()
	out, err := term.WithMonths(ms)
	require.NoError(t, err)
	return out
}

// seedTermMonthsRig builds, for tenant, one cycle, two levels, two terms of the
// first level and one of the second. prefix keeps ids unique per test (8 hex).
func seedTermMonthsRig(t *testing.T, tenant, prefix string) termMonthsRig {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenant)
	id := func(n string) string { return prefix + "-0000-0000-0000-00000000000" + n }
	r := termMonthsRig{repo: repo, tenant: tenant, cycleID: id("1"), levelID: id("2"), otherLevelID: id("3")}

	cycle, err := domain.NewCycle(r.cycleID, tenant, "Cycle-"+prefix, domain.CycleStatusActive, startsOn, endsOn)
	require.NoError(t, err)
	require.NoError(t, insertCycleFixture(t, cycle))
	for lid, name := range map[string]string{r.levelID: "Level-" + prefix, r.otherLevelID: "Other-" + prefix} {
		level, err := domain.NewLevel(lid, tenant, name, "", domain.GranularityCycle)
		require.NoError(t, err)
		require.NoError(t, repo.CreateLevel(ctx, level))
	}
	mk := func(tid, lid string, ord int) domain.Term {
		term, err := domain.NewTerm(tid, tenant, r.cycleID, lid, "T", ord, startsOn, endsOn)
		require.NoError(t, err)
		require.NoError(t, repo.CreateTerm(ctx, term))
		return term
	}
	r.t1, r.t2, r.tOther = mk(id("4"), r.levelID, 1), mk(id("5"), r.levelID, 2), mk(id("6"), r.otherLevelID, 1)
	return r
}

func TestTermMonthsRoundTripAndBatchedRead(t *testing.T) {
	ctx := context.Background()
	r := seedTermMonthsRig(t, tenantA, "7e3a0001")

	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{
		withMonths(t, r.t1, ym(2026, time.October), ym(2026, time.September)),
		withMonths(t, r.t2, ym(2026, time.November)),
		withMonths(t, r.tOther, ym(2026, time.September)), // another level may repeat a month
	}))

	got, err := r.repo.GetTerm(ctx, r.t1.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10"}, monthsOf(got))

	list, err := r.repo.ListTerms(ctx, r.cycleID, "")
	require.NoError(t, err)
	byID := map[string][]string{}
	for _, term := range list {
		byID[term.ID()] = monthsOf(term)
	}
	assert.Equal(t, map[string][]string{
		r.t1.ID():     {"2026-09", "2026-10"},
		r.t2.ID():     {"2026-11"},
		r.tOther.ID(): {"2026-09"},
	}, byID)

	// Replace, not merge.
	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{withMonths(t, r.t1, ym(2026, time.December))}))
	got, err = r.repo.GetTerm(ctx, r.t1.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-12"}, monthsOf(got))

	// Clear.
	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{withMonths(t, r.t1)}))
	got, err = r.repo.GetTerm(ctx, r.t1.ID())
	require.NoError(t, err)
	assert.Empty(t, got.Months())
}

func TestApplyTermMonthsSwapsMonthsBetweenSiblingsInOneCall(t *testing.T) {
	ctx := context.Background()
	r := seedTermMonthsRig(t, tenantA, "7e3a0002")
	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{
		withMonths(t, r.t1, ym(2026, time.September)),
		withMonths(t, r.t2, ym(2026, time.October)),
	}))

	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{
		withMonths(t, r.t1, ym(2026, time.October)),
		withMonths(t, r.t2, ym(2026, time.September)),
	}), "every update's old months are released before any new one is claimed")

	got, err := r.repo.GetTerm(ctx, r.t2.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09"}, monthsOf(got))
}

func TestApplyTermMonthsConflictNamesTheMonthAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	r := seedTermMonthsRig(t, tenantA, "7e3a0003")
	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{withMonths(t, r.t2, ym(2026, time.October))}))

	created, err := domain.NewTerm("7e3a0003-0000-0000-0000-000000000009", tenantA, r.cycleID, r.otherLevelID, "T2", 2, startsOn, endsOn)
	require.NoError(t, err)
	err = r.repo.ApplyTermMonths(ctx,
		[]domain.Term{withMonths(t, created, ym(2026, time.November))},
		[]domain.Term{withMonths(t, r.t1, ym(2026, time.September), ym(2026, time.October))})

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMonthInOtherTerm, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ese mes ya pertenece a otro periodo del nivel: 2026-10", glerrors.SafeMessage(err))

	_, err = r.repo.GetTerm(ctx, created.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "the create in the same call rolled back")
	got, err := r.repo.GetTerm(ctx, r.t1.ID())
	require.NoError(t, err)
	assert.Empty(t, got.Months(), "the update rolled back")
}

func TestApplyTermMonthsCreatesTermsWithTheirMonths(t *testing.T) {
	ctx := context.Background()
	r := seedTermMonthsRig(t, tenantA, "7e3a0004")
	created, err := domain.NewTerm("7e3a0004-0000-0000-0000-000000000009", tenantA, r.cycleID, r.otherLevelID, "Trimestre 2", 2, startsOn, endsOn)
	require.NoError(t, err)

	require.NoError(t, r.repo.ApplyTermMonths(ctx, []domain.Term{withMonths(t, created, ym(2026, time.November))}, nil))

	got, err := r.repo.GetTerm(ctx, created.ID())
	require.NoError(t, err)
	assert.Equal(t, "Trimestre 2", got.Name())
	assert.Equal(t, []string{"2026-11"}, monthsOf(got))

	dup, err := domain.NewTerm("7e3a0004-0000-0000-0000-00000000000a", tenantA, r.cycleID, r.otherLevelID, "otra", 2, startsOn, endsOn)
	require.NoError(t, err)
	err = r.repo.ApplyTermMonths(ctx, []domain.Term{dup}, nil)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err), "a duplicate ordinal is still a conflict")
}

// The schema holds on its own, whatever the adapter does.
func TestTermMonthsSchemaConstraints(t *testing.T) {
	r := seedTermMonthsRig(t, tenantA, "7e3a0005")
	insert := `INSERT INTO term_months (tenant_id, term_id, cycle_id, level_id, month) VALUES (?, ?, ?, ?, ?)`

	tests := []struct {
		name  string
		args  []any
		fails bool
	}{
		{"a first-of-month row", []any{tenantA, r.t1.ID(), r.cycleID, r.levelID, "2026-09-01"}, false},
		{"a mid-month day", []any{tenantA, r.t1.ID(), r.cycleID, r.levelID, "2026-10-02"}, true},
		{"a level that is not the term's", []any{tenantA, r.t1.ID(), r.cycleID, r.otherLevelID, "2026-11-01"}, true},
		{"a month a sibling already holds", []any{tenantA, r.t2.ID(), r.cycleID, r.levelID, "2026-09-01"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := execAsTenant(t, tenantA, insert, tt.args...)
			assert.Equal(t, tt.fails, err != nil, "err=%v", err)
		})
	}

	// Deleting the term takes its months with it.
	require.NoError(t, execAsTenant(t, tenantA, `DELETE FROM terms WHERE id = ?`, r.t1.ID()))
	assert.Zero(t, countAsTenant(t, tenantA, `SELECT count(*) FROM term_months WHERE term_id = ?`, r.t1.ID()))
}

func TestRLSIsolatesTermMonths(t *testing.T) {
	ctx := context.Background()
	r := seedTermMonthsRig(t, tenantA, "7e3a0006")
	require.NoError(t, r.repo.ApplyTermMonths(ctx, nil, []domain.Term{withMonths(t, r.t1, ym(2026, time.September))}))

	assert.Zero(t, countAsTenant(t, tenantB, `SELECT count(*) FROM term_months WHERE term_id = ?`, r.t1.ID()),
		"tenant B read tenant A's months (RLS breach)")
	if n := writeAsTenant(t, tenantB, `DELETE FROM term_months WHERE term_id = ?`, r.t1.ID()); n != 0 {
		t.Errorf("tenant B DELETE affected %d rows (RLS breach)", n)
	}
	err := execAsTenant(t, tenantB, `INSERT INTO term_months (tenant_id, term_id, cycle_id, level_id, month) VALUES (?, ?, ?, ?, '2026-10-01')`,
		tenantA, r.t1.ID(), r.cycleID, r.levelID)
	assert.Error(t, err, "tenant B wrote a row stamped with tenant A (WITH CHECK breach)")

	// Tenant B's repository cannot write months onto tenant A's term either:
	// the term is invisible, so the composite FK finds nothing.
	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	err = repoB.ApplyTermMonths(ctx, nil, []domain.Term{withMonths(t, r.t1, ym(2026, time.November))})
	assert.Error(t, err)

	got, err := r.repo.GetTerm(ctx, r.t1.ID())
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09"}, monthsOf(got), "tenant A's months are untouched")
}

// execAsTenant runs one statement in a transaction bound to tenantID.
func execAsTenant(t *testing.T, tenantID, sql string, args ...any) error {
	t.Helper()
	return appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID).Error; err != nil {
			return err
		}
		return tx.Exec(sql, args...).Error
	})
}

// countAsTenant runs a count(*) in a transaction bound to tenantID.
func countAsTenant(t *testing.T, tenantID, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	err := appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID).Error; err != nil {
			return err
		}
		return tx.Raw(sql, args...).Scan(&n).Error
	})
	require.NoError(t, err)
	return n
}
