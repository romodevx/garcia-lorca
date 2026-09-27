package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// ApplyTermMonths models the adapter's single transaction: every create lands
// with its months and every update's months are replaced, all or nothing.
func (f *fakeCatalog) ApplyTermMonths(_ context.Context, creates, updates []domain.Term) error {
	f.applyCalls++
	if f.applyErr != nil {
		return f.applyErr
	}
	for _, t := range creates {
		if err := f.claim("term|" + t.CycleID() + "|" + t.LevelID() + "|" + itoa(t.Ordinal())); err != nil {
			return err
		}
		f.terms[t.ID()] = t
	}
	for _, t := range updates {
		f.terms[t.ID()] = t
	}
	return nil
}

func months(ss ...string) []string { return ss }

func monthStrings(t domain.Term) []string {
	out := make([]string, 0, len(t.Months()))
	for _, m := range t.Months() {
		out = append(out, domain.FormatMonth(m))
	}
	return out
}

func seedTerm(t *testing.T, svc *usecase.CatalogService, cycleID, levelID, name string, ordinal int) domain.Term {
	t.Helper()
	term, err := svc.CreateTerm(ctx(), cycleID, levelID, name, ordinal, someDate, laterDate)
	require.NoError(t, err)
	return term
}

func TestSetTermMonths(t *testing.T) {
	type rig struct {
		svc   *usecase.CatalogService
		repo  *fakeCatalog
		t1    domain.Term
		t2    domain.Term
		other domain.Term // same ordinal, another level
	}
	setup := func(t *testing.T) rig {
		svc, repo := newService(t)
		c, l := seedCycle(t, svc), seedLevel(t, svc)
		l2, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityCycle)
		require.NoError(t, err)
		r := rig{svc: svc, repo: repo,
			t1:    seedTerm(t, svc, c.ID(), l.ID(), "Trimestre 1", 1),
			t2:    seedTerm(t, svc, c.ID(), l.ID(), "Trimestre 2", 2),
			other: seedTerm(t, svc, c.ID(), l2.ID(), "Trimestre 1", 1),
		}
		return r
	}

	tests := []struct {
		name     string
		before   func(t *testing.T, r rig)
		termID   func(r rig) string
		months   []string
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
		want     []string
	}{
		{
			name:   "replaces the set, sorted and de-duplicated",
			termID: func(r rig) string { return r.t1.ID() },
			months: months("2026-11", "2026-09", "2026-10", "2026-09"),
			want:   []string{"2026-09", "2026-10", "2026-11"},
		},
		{
			name: "an empty set clears the term",
			before: func(t *testing.T, r rig) {
				_, err := r.svc.SetTermMonths(ctx(), r.t1.ID(), months("2026-09"))
				require.NoError(t, err)
			},
			termID: func(r rig) string { return r.t1.ID() },
			months: nil,
			want:   []string{},
		},
		{
			name:   "the months the cycle starts and ends in are inside it",
			termID: func(r rig) string { return r.t1.ID() },
			months: months("2026-08", "2027-08"),
			want:   []string{"2026-08", "2027-08"},
		},
		{
			name:     "a term longer than six months is refused",
			termID:   func(r rig) string { return r.t1.ID() },
			months:   months("2026-09", "2026-10", "2026-11", "2026-12", "2027-01", "2027-02", "2027-03"),
			wantKind: glerrors.KindInvalid,
			wantCode: domain.CodeTermTooLong,
		},
		{
			name:   "six months is the longest term",
			termID: func(r rig) string { return r.t1.ID() },
			months: months("2026-09", "2026-10", "2026-11", "2026-12", "2027-01", "2027-02"),
			want:   []string{"2026-09", "2026-10", "2026-11", "2026-12", "2027-01", "2027-02"},
		},
		{
			name:     "a month outside the cycle is refused",
			termID:   func(r rig) string { return r.t1.ID() },
			months:   months("2026-09", "2027-09"),
			wantKind: glerrors.KindInvalid,
			wantCode: domain.CodeMonthOutsideCycle,
		},
		{
			name:     "a malformed month is invalid",
			termID:   func(r rig) string { return r.t1.ID() },
			months:   months("2026-13"),
			wantKind: glerrors.KindInvalid,
		},
		{
			name: "a month a sibling term of the level holds is a conflict",
			before: func(t *testing.T, r rig) {
				_, err := r.svc.SetTermMonths(ctx(), r.t2.ID(), months("2026-12"))
				require.NoError(t, err)
			},
			termID:   func(r rig) string { return r.t1.ID() },
			months:   months("2026-11", "2026-12"),
			wantKind: glerrors.KindConflict,
			wantCode: domain.CodeMonthInOtherTerm,
		},
		{
			name: "another level may hold the same month",
			before: func(t *testing.T, r rig) {
				_, err := r.svc.SetTermMonths(ctx(), r.other.ID(), months("2026-09"))
				require.NoError(t, err)
			},
			termID: func(r rig) string { return r.t1.ID() },
			months: months("2026-09"),
			want:   []string{"2026-09"},
		},
		{
			name: "re-sending the term's own months is not a conflict with itself",
			before: func(t *testing.T, r rig) {
				_, err := r.svc.SetTermMonths(ctx(), r.t1.ID(), months("2026-09"))
				require.NoError(t, err)
			},
			termID: func(r rig) string { return r.t1.ID() },
			months: months("2026-09", "2026-10"),
			want:   []string{"2026-09", "2026-10"},
		},
		{
			name:     "an unknown term is not found",
			termID:   func(rig) string { return "no-such-term" },
			months:   months("2026-09"),
			wantKind: glerrors.KindNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setup(t)
			if tt.before != nil {
				tt.before(t, r)
			}
			calls := r.repo.applyCalls
			got, err := r.svc.SetTermMonths(ctx(), tt.termID(r), tt.months)
			if tt.wantKind != 0 {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err=%v", err)
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
				assert.Equal(t, calls, r.repo.applyCalls, "a refused set writes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, monthStrings(got))
			stored, err := r.svc.GetTerm(ctx(), got.ID())
			require.NoError(t, err)
			assert.Equal(t, tt.want, monthStrings(stored))
		})
	}
}

func TestSetTermMonthsNamesTheConflictingMonth(t *testing.T) {
	svc, _ := newService(t)
	c, l := seedCycle(t, svc), seedLevel(t, svc)
	t1 := seedTerm(t, svc, c.ID(), l.ID(), "Trimestre 1", 1)
	t2 := seedTerm(t, svc, c.ID(), l.ID(), "Trimestre 2", 2)
	_, err := svc.SetTermMonths(ctx(), t2.ID(), months("2026-12"))
	require.NoError(t, err)

	_, err = svc.SetTermMonths(ctx(), t1.ID(), months("2026-11", "2026-12"))
	assert.Equal(t, "Ese mes ya pertenece a otro periodo del nivel: 2026-12", glerrors.SafeMessage(err))
}

func TestCopyTermMonths(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	primaria := seedLevel(t, svc)
	secundaria, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityCycle)
	require.NoError(t, err)
	prepa, err := svc.CreateLevel(ctx(), "Preparatoria", "SEP-PREP", domain.GranularityCycle)
	require.NoError(t, err)

	src1 := seedTerm(t, svc, c.ID(), primaria.ID(), "Trimestre 1", 1)
	src2 := seedTerm(t, svc, c.ID(), primaria.ID(), "Trimestre 2", 2)
	_, err = svc.SetTermMonths(ctx(), src1.ID(), months("2026-09", "2026-10"))
	require.NoError(t, err)
	_, err = svc.SetTermMonths(ctx(), src2.ID(), months("2026-11", "2026-12"))
	require.NoError(t, err)

	// Secundaria already has T1 (own name, months that the source gives to T2)
	// and a T3 the source lacks, holding a month the source's T1 claims.
	sec1 := seedTerm(t, svc, c.ID(), secundaria.ID(), "Bloque 1", 1)
	sec3 := seedTerm(t, svc, c.ID(), secundaria.ID(), "Bloque 3", 3)
	_, err = svc.SetTermMonths(ctx(), sec1.ID(), months("2026-11"))
	require.NoError(t, err)
	_, err = svc.SetTermMonths(ctx(), sec3.ID(), months("2026-09"))
	require.NoError(t, err)

	calls := repo.applyCalls
	got, err := svc.CopyTermMonths(ctx(), c.ID(), primaria.ID(), []string{secundaria.ID(), prepa.ID(), secundaria.ID()})
	require.NoError(t, err)
	assert.Equal(t, calls+1, repo.applyCalls, "every level is written in ONE transaction")

	type row struct {
		level, name string
		ordinal     int
		months      []string
	}
	rows := make([]row, 0, len(got))
	for _, term := range got {
		rows = append(rows, row{term.LevelID(), term.Name(), term.Ordinal(), monthStrings(term)})
	}
	assert.Equal(t, []row{
		{secundaria.ID(), "Bloque 1", 1, []string{"2026-09", "2026-10"}},
		{secundaria.ID(), "Trimestre 2", 2, []string{"2026-11", "2026-12"}},
		{secundaria.ID(), "Bloque 3", 3, []string{}},
		{prepa.ID(), "Trimestre 1", 1, []string{"2026-09", "2026-10"}},
		{prepa.ID(), "Trimestre 2", 2, []string{"2026-11", "2026-12"}},
	}, rows, "matched by ordinal; missing terms created from the source; unmatched ones cleared")

	for _, term := range got {
		if term.LevelID() == prepa.ID() {
			assert.True(t, term.StartsOn().Equal(someDate) && term.EndsOn().Equal(laterDate), "a created term copies the source dates")
		}
	}
	stored, err := svc.ListTerms(ctx(), c.ID(), prepa.ID())
	require.NoError(t, err)
	assert.Len(t, stored, 2, "the created terms are persisted")
}

func TestCopyTermMonthsRefusals(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	primaria := seedLevel(t, svc)
	secundaria, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityCycle)
	require.NoError(t, err)
	empty, err := svc.CreateLevel(ctx(), "Maternal", "SEP-MAT", domain.GranularityCycle)
	require.NoError(t, err)
	seedTerm(t, svc, c.ID(), primaria.ID(), "Trimestre 1", 1)

	tests := []struct {
		name     string
		cycleID  string
		from     string
		to       []string
		wantKind glerrors.Kind
	}{
		{"no target levels", c.ID(), primaria.ID(), nil, glerrors.KindInvalid},
		{"copying a level onto itself", c.ID(), primaria.ID(), []string{primaria.ID()}, glerrors.KindInvalid},
		{"an unknown target level", c.ID(), primaria.ID(), []string{"no-such-level"}, glerrors.KindInvalid},
		{"an unknown source level", c.ID(), "no-such-level", []string{secundaria.ID()}, glerrors.KindInvalid},
		{"an unknown cycle", "no-such-cycle", primaria.ID(), []string{secundaria.ID()}, glerrors.KindInvalid},
		{"a source with no terms", c.ID(), empty.ID(), []string{secundaria.ID()}, glerrors.KindInvalid},
		{"a blank target id", c.ID(), primaria.ID(), []string{" "}, glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := repo.applyCalls
			_, err := svc.CopyTermMonths(ctx(), tt.cycleID, tt.from, tt.to)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err=%v", err)
			assert.Equal(t, calls, repo.applyCalls, "a refused copy writes nothing")
		})
	}
}

func TestCopyTermMonthsPropagatesTheWriteError(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	primaria := seedLevel(t, svc)
	secundaria, err := svc.CreateLevel(ctx(), "Secundaria", "SEP-SEC", domain.GranularityCycle)
	require.NoError(t, err)
	seedTerm(t, svc, c.ID(), primaria.ID(), "Trimestre 1", 1)

	repo.applyErr = domain.MonthInOtherTerm(time.Time{})
	_, err = svc.CopyTermMonths(ctx(), c.ID(), primaria.ID(), []string{secundaria.ID()})
	assert.Equal(t, domain.CodeMonthInOtherTerm, domain.RefusalCodeOf(err))
}
