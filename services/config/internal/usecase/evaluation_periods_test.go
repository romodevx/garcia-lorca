package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// fakePeriods is an in-memory EvaluationPeriodRepository: one cycle, a level
// with three grades and another level with one.
type fakePeriods struct {
	cycle    domain.Cycle
	grades   map[string]domain.GradeLevel
	periods  map[string]domain.EvaluationPeriod // by id
	replaces int
	failWith error
}

const (
	pCycle   = "cycle-1"
	pLevel   = "level-1"
	pGrade1  = "grade-1"
	pGrade2  = "grade-2"
	pGrade3  = "grade-3"
	pOtherLv = "level-2"
	pOtherGr = "grade-9"
)

func d(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC) }

func newFakePeriods(t *testing.T) *fakePeriods {
	t.Helper()
	cycle, err := domain.NewCycle(pCycle, "ten", "2026-2027", domain.CycleStatusActive, d(2026, time.August, 24), d(2027, time.July, 9))
	require.NoError(t, err)
	f := &fakePeriods{cycle: cycle, grades: map[string]domain.GradeLevel{}, periods: map[string]domain.EvaluationPeriod{}}
	for i, id := range []string{pGrade1, pGrade2, pGrade3} {
		g, err := domain.NewGradeLevel(id, "ten", pLevel, fmt.Sprintf("%d°", i+1), i+1)
		require.NoError(t, err)
		f.grades[id] = g
	}
	g, err := domain.NewGradeLevel(pOtherGr, "ten", pOtherLv, "1° sec", 1)
	require.NoError(t, err)
	f.grades[pOtherGr] = g
	return f
}

func (f *fakePeriods) GetCycle(_ context.Context, id string) (domain.Cycle, error) {
	if id != f.cycle.ID() {
		return domain.Cycle{}, glerrors.New(glerrors.KindNotFound, "cycle not found")
	}
	return f.cycle, nil
}

func (f *fakePeriods) GetGradeLevel(_ context.Context, id string) (domain.GradeLevel, error) {
	g, ok := f.grades[id]
	if !ok {
		return domain.GradeLevel{}, glerrors.New(glerrors.KindNotFound, "grade not found")
	}
	return g, nil
}

func (f *fakePeriods) ListEvaluationPeriods(_ context.Context, cycleID, levelID, gradeID string) ([]domain.EvaluationPeriod, error) {
	var out []domain.EvaluationPeriod
	for _, p := range f.periods {
		if p.CycleID() != cycleID || (gradeID != "" && p.GradeLevelID() != gradeID) ||
			(levelID != "" && f.grades[p.GradeLevelID()].LevelID() != levelID) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		gi, gj := f.grades[out[i].GradeLevelID()].Ordinal(), f.grades[out[j].GradeLevelID()].Ordinal()
		if gi != gj {
			return gi < gj
		}
		return out[i].Ordinal() < out[j].Ordinal()
	})
	return out, nil
}

func (f *fakePeriods) ReplaceEvaluationPeriods(_ context.Context, cycleID string, sets []ports.GradePeriodSet) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.replaces++
	for _, set := range sets {
		for id, p := range f.periods {
			if p.CycleID() == cycleID && p.GradeLevelID() == set.GradeLevelID {
				delete(f.periods, id)
			}
		}
		for _, p := range set.Periods {
			f.periods[p.ID()] = p
		}
	}
	return nil
}

func newPeriodService(f *fakePeriods) *usecase.EvaluationPeriodService {
	n := 0
	return usecase.NewEvaluationPeriodService(f, "ten", func() string { n++; return fmt.Sprintf("id-%d", n) })
}

func trimesters() []usecase.PeriodInput {
	return []usecase.PeriodInput{
		{Ordinal: 1, Name: "1.er trimestre", StartsOn: d(2026, time.August, 24), EndsOn: d(2026, time.November, 30)},
		{Ordinal: 2, Name: "2.º trimestre", StartsOn: d(2026, time.December, 1), EndsOn: d(2027, time.March, 31)},
		{Ordinal: 3, Name: "3.er trimestre", StartsOn: d(2027, time.April, 1), EndsOn: d(2027, time.July, 9)},
	}
}

func TestUpsertEvaluationPeriodsCreatesThenEditsInPlace(t *testing.T) {
	ctx := context.Background()
	f := newFakePeriods(t)
	svc := newPeriodService(f)

	created, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, trimesters())
	require.NoError(t, err)
	require.Len(t, created, 3)
	firstID := created[0].ID()

	// Move the first trimester's end a week earlier and the second's start
	// with it; drop the third.
	edited, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, []usecase.PeriodInput{
		{ID: created[1].ID(), Ordinal: 2, Name: "2.º trimestre", StartsOn: d(2026, time.November, 24), EndsOn: d(2027, time.March, 31)},
		{ID: firstID, Ordinal: 1, Name: "Primer trimestre", StartsOn: d(2026, time.August, 24), EndsOn: d(2026, time.November, 23)},
	})
	require.NoError(t, err)
	require.Len(t, edited, 2)
	assert.Equal(t, firstID, edited[0].ID(), "an edited period keeps its id (T15's grades point at it)")
	assert.Equal(t, "Primer trimestre", edited[0].Name())
	assert.Equal(t, 1, edited[0].Ordinal(), "returned by ordinal")
	assert.True(t, edited[0].EndsOn().Equal(d(2026, time.November, 23)))

	left, err := svc.ListEvaluationPeriods(ctx, pCycle, "", pGrade1)
	require.NoError(t, err)
	assert.Len(t, left, 2, "a period left out of the set is removed")
}

// The id follows the row the person edited, never its position: removing the
// 1st trimester removes IT, and the 2nd and 3rd keep their ids although they
// are renumbered 1 and 2.
func TestUpsertEvaluationPeriodsIdsFollowTheRowNotTheOrdinal(t *testing.T) {
	ctx := context.Background()
	f := newFakePeriods(t)
	svc := newPeriodService(f)
	created, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, trimesters())
	require.NoError(t, err)
	second, third := created[1], created[2]

	got, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, []usecase.PeriodInput{
		{ID: second.ID(), Ordinal: 1, Name: second.Name(), StartsOn: second.StartsOn(), EndsOn: second.EndsOn()},
		{ID: third.ID(), Ordinal: 2, Name: third.Name(), StartsOn: third.StartsOn(), EndsOn: third.EndsOn()},
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, second.ID(), got[0].ID())
	assert.Equal(t, third.ID(), got[1].ID())
	_, stillThere := f.periods[created[0].ID()]
	assert.False(t, stillThere, "the removed period is the one that goes")

	// An id that is not one of the grade's periods is a new period, never a
	// way to take over another grade's row.
	foreign, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade2, []usecase.PeriodInput{
		{ID: second.ID(), Ordinal: 1, Name: "X", StartsOn: second.StartsOn(), EndsOn: second.EndsOn()},
	})
	require.NoError(t, err)
	assert.NotEqual(t, second.ID(), foreign[0].ID())
}

func TestUpsertEvaluationPeriodsKeepsTheTermLink(t *testing.T) {
	ctx := context.Background()
	f := newFakePeriods(t)
	migrated, err := domain.NewEvaluationPeriod("p-old", "ten", pCycle, pGrade1, "T1", 1, d(2026, time.August, 24), d(2026, time.November, 30))
	require.NoError(t, err)
	f.periods["p-old"] = migrated.WithTermID("term-1")

	in := trimesters()[:1]
	in[0].ID = "p-old"
	got, err := newPeriodService(f).UpsertEvaluationPeriods(ctx, pCycle, pGrade1, in)
	require.NoError(t, err)
	assert.Equal(t, "p-old", got[0].ID())
	assert.Equal(t, "term-1", got[0].TermID())
}

func TestUpsertEvaluationPeriodsRefusals(t *testing.T) {
	overlap := trimesters()
	overlap[1].StartsOn = d(2026, time.November, 30)
	outside := trimesters()
	outside[2].EndsOn = d(2027, time.July, 31)
	backwards := trimesters()
	backwards[0].StartsOn, backwards[0].EndsOn = backwards[0].EndsOn, backwards[0].StartsOn

	tests := []struct {
		name     string
		cycle    string
		grade    string
		in       []usecase.PeriodInput
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
	}{
		{name: "overlap", cycle: pCycle, grade: pGrade1, in: overlap, wantKind: glerrors.KindConflict, wantCode: domain.CodePeriodOverlap},
		{name: "outside the cycle", cycle: pCycle, grade: pGrade1, in: outside, wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodOutsideCycle},
		{name: "ends before it starts", cycle: pCycle, grade: pGrade1, in: backwards, wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodEndsBeforeStart},
		{name: "unknown cycle", cycle: "nope", grade: pGrade1, in: trimesters(), wantKind: glerrors.KindInvalid},
		{name: "unknown grade", cycle: pCycle, grade: "nope", in: trimesters(), wantKind: glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePeriods(t)
			_, err := newPeriodService(f).UpsertEvaluationPeriods(context.Background(), tt.cycle, tt.grade, tt.in)
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			}
			assert.Zero(t, f.replaces, "nothing is written")
		})
	}
}

func TestCopyEvaluationPeriodsMirrorsTheSourceByOrdinal(t *testing.T) {
	ctx := context.Background()
	f := newFakePeriods(t)
	svc := newPeriodService(f)
	_, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, trimesters())
	require.NoError(t, err)
	// Grade 2 already has four bimesters: ordinals 1–3 take the source's
	// dates and names keeping their ids, the 4th goes.
	var bimesters []usecase.PeriodInput
	for i := 0; i < 4; i++ {
		from := d(2026, time.September, 1).AddDate(0, 2*i, 0)
		bimesters = append(bimesters, usecase.PeriodInput{Ordinal: i + 1, Name: fmt.Sprintf("B%d", i+1), StartsOn: from, EndsOn: from.AddDate(0, 2, -1)})
	}
	before, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade2, bimesters)
	require.NoError(t, err)
	writes := f.replaces

	got, err := svc.CopyEvaluationPeriods(ctx, pCycle, pGrade1, []string{pGrade2, pGrade3, pGrade2})
	require.NoError(t, err)
	assert.Equal(t, writes+1, f.replaces, "every target in ONE write")
	require.Len(t, got, 6, "grade 2 then grade 3, three periods each")

	g2, err := svc.ListEvaluationPeriods(ctx, pCycle, "", pGrade2)
	require.NoError(t, err)
	require.Len(t, g2, 3)
	for i, p := range g2 {
		assert.Equal(t, before[i].ID(), p.ID(), "ordinal %d keeps its id", i+1)
		assert.Equal(t, trimesters()[i].Name, p.Name())
		assert.True(t, trimesters()[i].StartsOn.Equal(p.StartsOn()))
		assert.True(t, trimesters()[i].EndsOn.Equal(p.EndsOn()))
	}
	g3, err := svc.ListEvaluationPeriods(ctx, pCycle, pLevel, pGrade3)
	require.NoError(t, err)
	assert.Len(t, g3, 3)

	whole, err := svc.ListEvaluationPeriods(ctx, pCycle, pLevel, "")
	require.NoError(t, err)
	assert.Len(t, whole, 9, "the whole level: three grades × three periods")
}

func TestCopyEvaluationPeriodsRefusals(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		from string
		to   []string
		seed bool
	}{
		{name: "source has no periods", from: pGrade1, to: []string{pGrade2}, seed: false},
		{name: "no targets", from: pGrade1, to: nil, seed: true},
		{name: "onto itself", from: pGrade1, to: []string{pGrade1}, seed: true},
		{name: "unknown target", from: pGrade1, to: []string{"nope"}, seed: true},
		{name: "blank target", from: pGrade1, to: []string{" "}, seed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakePeriods(t)
			svc := newPeriodService(f)
			if tt.seed {
				_, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, trimesters())
				require.NoError(t, err)
			}
			writes := f.replaces
			_, err := svc.CopyEvaluationPeriods(ctx, pCycle, tt.from, tt.to)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Equal(t, writes, f.replaces, "nothing is written")
		})
	}
}

func TestListEvaluationPeriodsNeedsACycle(t *testing.T) {
	_, err := newPeriodService(newFakePeriods(t)).ListEvaluationPeriods(context.Background(), " ", pLevel, "")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

func TestEvaluationPeriodOn(t *testing.T) {
	ctx := context.Background()
	f := newFakePeriods(t)
	svc := newPeriodService(f)
	_, err := svc.UpsertEvaluationPeriods(ctx, pCycle, pGrade1, trimesters())
	require.NoError(t, err)

	p, ok, err := svc.EvaluationPeriodOn(ctx, pCycle, pGrade1, d(2026, time.December, 1))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 2, p.Ordinal())

	_, ok, err = svc.EvaluationPeriodOn(ctx, pCycle, pGrade2, d(2026, time.December, 1))
	require.NoError(t, err)
	assert.False(t, ok, "grade 2 has no periods yet")
}
