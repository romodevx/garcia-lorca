//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

type periodsRig struct {
	repo                     *pgadapter.Repository
	cycleID, levelID         string
	grade1, grade2, otherLvG string
}

func dt(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// seedPeriodsRig builds, for tenant, one cycle, a level with two grades and
// another level with one. prefix keeps ids unique per test (8 hex).
func seedPeriodsRig(t *testing.T, tenant, prefix string) periodsRig {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenant)
	id := func(n string) string { return prefix + "-0000-0000-0000-00000000000" + n }
	r := periodsRig{repo: repo, cycleID: id("1"), levelID: id("2"), grade1: id("4"), grade2: id("5"), otherLvG: id("6")}

	cycle, err := domain.NewCycle(r.cycleID, tenant, "Cycle-"+prefix, domain.CycleStatusActive, dt(2026, time.August, 24), dt(2027, time.July, 9))
	require.NoError(t, err)
	require.NoError(t, insertCycleFixture(t, cycle))
	otherLevel := id("3")
	for lid, name := range map[string]string{r.levelID: "Level-" + prefix, otherLevel: "Other-" + prefix} {
		level, err := domain.NewLevel(lid, tenant, name, "", domain.GranularityCycle)
		require.NoError(t, err)
		require.NoError(t, repo.CreateLevel(ctx, level))
	}
	for gid, spec := range map[string]struct {
		level string
		ord   int
	}{r.grade1: {r.levelID, 1}, r.grade2: {r.levelID, 2}, r.otherLvG: {otherLevel, 1}} {
		g, err := domain.NewGradeLevel(gid, tenant, spec.level, "G"+gid[len(gid)-1:], spec.ord)
		require.NoError(t, err)
		require.NoError(t, repo.CreateGradeLevel(ctx, g))
	}
	return r
}

func evalPeriod(t *testing.T, tenant, id, cycle, grade string, ordinal int, from, to time.Time) domain.EvaluationPeriod {
	t.Helper()
	p, err := domain.NewEvaluationPeriod(id, tenant, cycle, grade, "P", ordinal, from, to)
	require.NoError(t, err)
	return p
}

func (r periodsRig) trimesters(t *testing.T, tenant, grade, idPrefix string) []domain.EvaluationPeriod {
	t.Helper()
	return []domain.EvaluationPeriod{
		evalPeriod(t, tenant, idPrefix+"1", r.cycleID, grade, 1, dt(2026, time.August, 24), dt(2026, time.November, 30)),
		evalPeriod(t, tenant, idPrefix+"2", r.cycleID, grade, 2, dt(2026, time.December, 1), dt(2027, time.March, 31)),
		evalPeriod(t, tenant, idPrefix+"3", r.cycleID, grade, 3, dt(2027, time.April, 1), dt(2027, time.July, 9)),
	}
}

func TestEvaluationPeriodsRoundTripAndScopedLists(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00001")
	g1 := r.trimesters(t, tenantA, r.grade1, "e7a00001-0000-0000-0000-0000000001a")
	g2 := r.trimesters(t, tenantA, r.grade2, "e7a00001-0000-0000-0000-0000000002a")
	other := r.trimesters(t, tenantA, r.otherLvG, "e7a00001-0000-0000-0000-0000000003a")

	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{
		{GradeLevelID: r.grade2, Periods: g2}, {GradeLevelID: r.grade1, Periods: g1}, {GradeLevelID: r.otherLvG, Periods: other},
	}))

	level, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, r.levelID, "")
	require.NoError(t, err)
	require.Len(t, level, 6, "the level's two grades only")
	assert.Equal(t, r.grade1, level[0].GradeLevelID(), "grade ordinal first")
	assert.Equal(t, 3, level[2].Ordinal(), "then period ordinal")
	assert.Equal(t, r.grade2, level[3].GradeLevelID())
	assert.True(t, level[0].StartsOn().Equal(dt(2026, time.August, 24)))
	assert.True(t, level[0].EndsOn().Equal(dt(2026, time.November, 30)))

	one, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.otherLvG)
	require.NoError(t, err)
	assert.Len(t, one, 3)
	all, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", "")
	require.NoError(t, err)
	assert.Len(t, all, 9)
	none, err := r.repo.ListEvaluationPeriods(ctx, "not-a-uuid", "", "")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// Shifting every period a week later passes through states where neighbours
// overlap; the set is checked as a whole at the end. Ids survive, a period
// left out goes, and the term link is kept.
func TestReplaceEvaluationPeriodsRewritesTheWholeSetInPlace(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00002")
	g1 := r.trimesters(t, tenantA, r.grade1, "e7a00002-0000-0000-0000-0000000001a")
	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: g1}}))

	shifted := []domain.EvaluationPeriod{
		evalPeriod(t, tenantA, g1[0].ID(), r.cycleID, r.grade1, 1, dt(2026, time.August, 31), dt(2026, time.December, 7)),
		evalPeriod(t, tenantA, g1[1].ID(), r.cycleID, r.grade1, 2, dt(2026, time.December, 8), dt(2027, time.April, 7)),
	}
	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: shifted}}))

	got, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.grade1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, g1[0].ID(), got[0].ID())
	assert.True(t, got[1].EndsOn().Equal(dt(2027, time.April, 7)))

	// Swapping ordinals between two existing rows also works in one call.
	swapped := []domain.EvaluationPeriod{
		evalPeriod(t, tenantA, g1[0].ID(), r.cycleID, r.grade1, 2, dt(2026, time.August, 31), dt(2026, time.December, 7)),
		evalPeriod(t, tenantA, g1[1].ID(), r.cycleID, r.grade1, 1, dt(2026, time.December, 8), dt(2027, time.April, 7)),
	}
	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: swapped}}))

	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1}}))
	got, err = r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.grade1)
	require.NoError(t, err)
	assert.Empty(t, got, "an empty set clears the grade")
}

func TestReplaceEvaluationPeriodsKeepsTheTermLink(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00003")
	term, err := domain.NewTerm("e7a00003-0000-0000-0000-0000000000f1", tenantA, r.cycleID, r.levelID, "T1", 1, dt(2026, time.August, 24), dt(2026, time.November, 30))
	require.NoError(t, err)
	require.NoError(t, r.repo.CreateTerm(ctx, term))
	p := evalPeriod(t, tenantA, "e7a00003-0000-0000-0000-0000000001a1", r.cycleID, r.grade1, 1, dt(2026, time.August, 24), dt(2026, time.November, 30)).WithTermID(term.ID())

	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: []domain.EvaluationPeriod{p}}}))
	got, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.grade1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, term.ID(), got[0].TermID())
}

// The repository refuses an overlap even when the use case is skipped, and
// writes nothing of the batch.
func TestReplaceEvaluationPeriodsRefusesOverlapsAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00004")
	g2 := r.trimesters(t, tenantA, r.grade2, "e7a00004-0000-0000-0000-0000000002a")
	overlapping := []domain.EvaluationPeriod{
		evalPeriod(t, tenantA, "e7a00004-0000-0000-0000-0000000001a1", r.cycleID, r.grade1, 1, dt(2026, time.September, 1), dt(2026, time.November, 30)),
		evalPeriod(t, tenantA, "e7a00004-0000-0000-0000-0000000001a2", r.cycleID, r.grade1, 2, dt(2026, time.November, 30), dt(2027, time.March, 31)),
	}
	err := r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{
		{GradeLevelID: r.grade2, Periods: g2}, {GradeLevelID: r.grade1, Periods: overlapping},
	})
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodePeriodOverlap, domain.RefusalCodeOf(err))

	all, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", "")
	require.NoError(t, err)
	assert.Empty(t, all, "the whole batch rolled back")
}

// Two editors racing on one grade with overlapping sets: one wins, the other
// is the coded conflict — never two overlapping periods.
func TestReplaceEvaluationPeriodsConcurrentOverlapsOneWins(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00005")
	sets := [][]domain.EvaluationPeriod{
		{evalPeriod(t, tenantA, "e7a00005-0000-0000-0000-0000000001a1", r.cycleID, r.grade1, 1, dt(2026, time.September, 1), dt(2026, time.November, 30))},
		{evalPeriod(t, tenantA, "e7a00005-0000-0000-0000-0000000001b1", r.cycleID, r.grade1, 2, dt(2026, time.October, 1), dt(2026, time.December, 20))},
	}
	errs := make([]error, len(sets))
	var wg sync.WaitGroup
	for i, set := range sets {
		wg.Add(1)
		go func(i int, set []domain.EvaluationPeriod) {
			defer wg.Done()
			repo := pgadapter.NewRepository(appDB(t), tenantA)
			// Each writer only adds its own row (no deletes), as two
			// clients that read an empty grade would.
			errs[i] = repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: set}})
		}(i, set)
	}
	wg.Wait()

	got, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.grade1)
	require.NoError(t, err)
	assert.Len(t, got, 1, "exactly one set survives: %v", errs)
}

func TestEvaluationPeriodsAreTenantIsolated(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00006")
	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{
		{GradeLevelID: r.grade1, Periods: r.trimesters(t, tenantA, r.grade1, "e7a00006-0000-0000-0000-0000000001a")},
	}))

	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	seen, err := repoB.ListEvaluationPeriods(ctx, r.cycleID, "", "")
	require.NoError(t, err)
	assert.Empty(t, seen, "tenant B sees none of A's periods")

	err = repoB.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{
		{GradeLevelID: r.grade1, Periods: r.trimesters(t, tenantA, r.grade1, "e7a00006-0000-0000-0000-0000000009a")},
	})
	require.Error(t, err, "B cannot write rows stamped with A's tenant")

	err = repoB.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1}})
	require.NoError(t, err, "clearing as B touches nothing it cannot see")
	still, err := r.repo.ListEvaluationPeriods(ctx, r.cycleID, "", r.grade1)
	require.NoError(t, err)
	assert.Len(t, still, 3, "A's periods survive B's clear")
}

// A save that re-inserts an id another row already holds (what the later of two
// simultaneous saves of one grade does) is the coded "changed meanwhile".
func TestReplaceEvaluationPeriodsTakenIdIsChangedMeanwhile(t *testing.T) {
	ctx := context.Background()
	r := seedPeriodsRig(t, tenantA, "e7a00007")
	g2 := r.trimesters(t, tenantA, r.grade2, "e7a00007-0000-0000-0000-0000000002a")
	require.NoError(t, r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade2, Periods: g2}}))

	clash := evalPeriod(t, tenantA, g2[0].ID(), r.cycleID, r.grade1, 1, dt(2026, time.September, 1), dt(2026, time.September, 30))
	err := r.repo.ReplaceEvaluationPeriods(ctx, r.cycleID, []ports.GradePeriodSet{{GradeLevelID: r.grade1, Periods: []domain.EvaluationPeriod{clash}}})
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodePeriodsChanged, domain.RefusalCodeOf(err))
}
