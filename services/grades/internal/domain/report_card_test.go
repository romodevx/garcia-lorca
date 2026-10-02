package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

func mustMonth(t *testing.T, raw string) domain.Month {
	t.Helper()
	m, err := domain.ParseMonth(raw)
	require.NoError(t, err)
	return m
}

func pgrade(t *testing.T, enrollment, subject, period string, whole int) domain.PeriodGrade {
	t.Helper()
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "g-" + subject + period, TenantID: "ten", EnrollmentID: enrollment, SubjectID: subject, PeriodID: period,
		CapturedBy: "usr", Value: domain.GradeValueFromHundredths(whole * 100),
	})
	require.NoError(t, err)
	return g
}

// cardPeriods are the columns of a 2.º trimestre card: periods 1 and 2.
func cardPeriods() []domain.ReportCardPeriod {
	return []domain.ReportCardPeriod{
		{ID: "p1", Ordinal: 1, Name: "1.er trimestre", StartsOn: date(2026, 8, 24)},
		{ID: "p2", Ordinal: 2, Name: "2.º trimestre", StartsOn: date(2026, 12, 7)},
	}
}

func header(t *testing.T) domain.ReportCardHeader {
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	return domain.ReportCardHeader{
		ID: "rc-1", TenantID: "ten", TermID: "term-2", CycleID: "cyc", LevelID: "lvl", GroupID: "grp",
		EnrollmentID: "enr-1", StudentID: "stu-1", Version: 1, IssuedAt: closeAt, IssuedBy: "usr-dir",
		StudentName: "Ana López", EnrollmentNumber: "2026-00001", GroupLabel: "1° A", LevelName: "Primaria",
		CycleName: "2026-2027", TermName: "2.º trimestre", TermOrdinal: 2,
		Periods: cardPeriods(),
		Scale:   scale,
	}
}

// T15: a report card snapshots each subject's grade in every period of the
// cycle up to the one closed (a column each), the exact mean of those it has
// (truncated to 2 decimals), and the general average: the exact mean of the
// subjects' exact averages. Grades of other periods or enrollments are ignored.
func TestBuildReportCard(t *testing.T) {
	subjects := []domain.ReportCardSubject{{ID: "mat", Name: "Matemáticas"}, {ID: "esp", Name: "Español"}, {ID: "art", Name: "Artes"}}
	grades := []domain.PeriodGrade{
		pgrade(t, "enr-1", "mat", "p1", 9),
		pgrade(t, "enr-1", "mat", "p2", 8),  // mat: 8.5
		pgrade(t, "enr-1", "esp", "p2", 7),  // esp: 7 (p1 missing, ignored)
		pgrade(t, "enr-1", "mat", "p3", 5),  // a later period: not on this card
		pgrade(t, "enr-2", "art", "p1", 10), // another pupil
	}
	rc, err := domain.BuildReportCard(header(t), subjects, grades)
	require.NoError(t, err)

	assert.Equal(t, domain.ReportCardPublished, rc.Status())
	require.Len(t, rc.Lines(), 3)
	mat, esp, art := rc.Lines()[0], rc.Lines()[1], rc.Lines()[2]
	assert.Equal(t, "Matemáticas", mat.SubjectName)
	assert.Equal(t, 1, mat.Position)
	assert.Equal(t, "8.5", mat.Average.String())
	assert.Equal(t, map[string]string{"p1": "9", "p2": "8"}, mat.PeriodValueStrings())
	assert.Equal(t, "7", esp.Average.String())
	assert.Equal(t, 2, esp.Position)
	assert.False(t, art.Average.Present(), "a subject with no grade has no average")
	assert.Empty(t, art.PeriodValues)
	assert.Equal(t, "7.75", rc.GeneralAverage().String(), "mean of 8.5 and 7; the ungraded subject is excluded")
	assert.Equal(t, "Ana López", rc.StudentName())
	assert.Equal(t, 1, rc.Version())
}

// The general average is computed from the subjects' EXACT averages and
// truncated once: 26/3 and 25/3 make 8.5 (not 8.66 + 8.33 → 8.49).
func TestBuildReportCardGeneralAverageIsExact(t *testing.T) {
	h := header(t)
	h.Periods = append(cardPeriods(), domain.ReportCardPeriod{ID: "p3", Ordinal: 3, Name: "3.er trimestre", StartsOn: date(2027, 4, 5)})
	grades := []domain.PeriodGrade{
		pgrade(t, "enr-1", "mat", "p1", 9), pgrade(t, "enr-1", "mat", "p2", 8), pgrade(t, "enr-1", "mat", "p3", 9),
		pgrade(t, "enr-1", "esp", "p1", 8), pgrade(t, "enr-1", "esp", "p2", 8), pgrade(t, "enr-1", "esp", "p3", 9),
	}
	rc, err := domain.BuildReportCard(h, []domain.ReportCardSubject{{ID: "mat", Name: "M"}, {ID: "esp", Name: "E"}}, grades)
	require.NoError(t, err)
	assert.Equal(t, "8.66", rc.Lines()[0].Average.String())
	assert.Equal(t, "8.33", rc.Lines()[1].Average.String())
	assert.Equal(t, "8.5", rc.GeneralAverage().String())
}

func TestBuildReportCardWithoutGrades(t *testing.T) {
	rc, err := domain.BuildReportCard(header(t), []domain.ReportCardSubject{{ID: "mat", Name: "Matemáticas"}}, nil)
	require.NoError(t, err)
	assert.False(t, rc.GeneralAverage().Present())
}

func TestBuildReportCardValidates(t *testing.T) {
	h := header(t)
	h.EnrollmentID = ""
	_, err := domain.BuildReportCard(h, nil, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))

	h = header(t)
	h.Version = 0
	_, err = domain.BuildReportCard(h, nil, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))

	h = header(t)
	h.Periods = nil
	_, err = domain.BuildReportCard(h, nil, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a card has columns: periods (or, issued before T15, months)")
}

// A stored report card is rebuilt as it was frozen, including its status. A
// card issued before T15 (month columns) reads back unchanged.
func TestRestoreReportCard(t *testing.T) {
	h := header(t)
	lines := []domain.ReportCardLine{{SubjectID: "mat", SubjectName: "Matemáticas", Position: 1,
		PeriodValues: map[string]domain.GradeValue{"p1": domain.GradeValueFromHundredths(900)},
		Average:      domain.AverageFromHundredths(900)}}
	rc, err := domain.RestoreReportCard(h, domain.ReportCardSuperseded, domain.AverageFromHundredths(900), lines)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardSuperseded, rc.Status())
	assert.Equal(t, "9", rc.GeneralAverage().String())
	assert.Equal(t, time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC), rc.IssuedAt())

	legacy := header(t)
	legacy.Periods = nil
	legacy.Months = []domain.Month{mustMonth(t, "2026-09"), mustMonth(t, "2026-10")}
	old := []domain.ReportCardLine{{SubjectID: "mat", SubjectName: "Matemáticas", Position: 1,
		MonthValues: map[domain.Month]domain.GradeValue{mustMonth(t, "2026-09"): domain.GradeValueFromHundredths(950)},
		Average:     domain.AverageFromHundredths(950)}}
	rc, err = domain.RestoreReportCard(legacy, domain.ReportCardPublished, domain.AverageFromHundredths(950), old)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"2026-09": "9.5"}, rc.Lines()[0].MonthValueStrings())
	assert.Equal(t, "9.5", rc.GeneralAverage().String())

	_, err = domain.RestoreReportCard(h, "weird", domain.Average{}, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

// Owner decision: a re-close notifies only the pupils whose card changed. Two
// cards carry the same grades when their subject set, period values, averages
// and general average read the same; ids, versions, names and order do not count.
func TestReportCardSameGradesAs(t *testing.T) {
	subjects := []domain.ReportCardSubject{{ID: "mat", Name: "Matemáticas"}, {ID: "esp", Name: "Español"}}
	base := []domain.PeriodGrade{pgrade(t, "enr-1", "mat", "p1", 9), pgrade(t, "enr-1", "esp", "p2", 9)}
	build := func(h domain.ReportCardHeader, subs []domain.ReportCardSubject, gs []domain.PeriodGrade) domain.ReportCard {
		rc, err := domain.BuildReportCard(h, subs, gs)
		require.NoError(t, err)
		return rc
	}
	previous := build(header(t), subjects, base)
	v2 := header(t)
	v2.ID, v2.Version, v2.StudentName = "rc-2", 2, "Ana López R."

	cases := []struct {
		name string
		next domain.ReportCard
		same bool
	}{
		{"same grades, new version and names", build(v2, subjects, base), true},
		{"subjects in another order", build(v2, []domain.ReportCardSubject{subjects[1], subjects[0]}, base), true},
		{"a period value changed", build(v2, subjects, []domain.PeriodGrade{pgrade(t, "enr-1", "mat", "p1", 8), base[1]}), false},
		{"a grade added in another period", build(v2, subjects, append(base, pgrade(t, "enr-1", "mat", "p2", 9))), false},
		{"a subject added without grades", build(v2, append(subjects, domain.ReportCardSubject{ID: "art", Name: "Artes"}), base), false},
		{"a subject dropped", build(v2, subjects[:1], base[:1]), false},
		{"grades removed", build(v2, subjects, base[:1]), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.same, c.next.SameGradesAs(previous))
		})
	}
}

// A re-close compares a freshly built (exact) card with the stored one, whose
// averages were truncated when frozen: 26/3 and a stored 8.66 are the same.
func TestReportCardSameGradesAsStoredTruncatedAverages(t *testing.T) {
	h := header(t)
	h.Periods = append(cardPeriods(), domain.ReportCardPeriod{ID: "p3", Ordinal: 3, Name: "3.er trimestre", StartsOn: date(2027, 4, 5)})
	grades := []domain.PeriodGrade{pgrade(t, "enr-1", "mat", "p1", 9), pgrade(t, "enr-1", "mat", "p2", 8), pgrade(t, "enr-1", "mat", "p3", 9)}
	fresh, err := domain.BuildReportCard(h, []domain.ReportCardSubject{{ID: "mat", Name: "M"}}, grades)
	require.NoError(t, err)
	stored, err := domain.RestoreReportCard(h, domain.ReportCardPublished, domain.AverageFromHundredths(866), []domain.ReportCardLine{{
		SubjectID: "mat", SubjectName: "M", Position: 1, Average: domain.AverageFromHundredths(866),
		PeriodValues: map[string]domain.GradeValue{"p1": domain.GradeValueFromHundredths(900), "p2": domain.GradeValueFromHundredths(800), "p3": domain.GradeValueFromHundredths(900)},
	}})
	require.NoError(t, err)
	assert.True(t, fresh.SameGradesAs(stored))
}

// A family reads its child's report cards newest period first: the newest
// cycle first (cycles are dated by the earliest month any of their cards
// covers, never by name or issue date), then the highest term ordinal. A term
// re-issued late (a reopen) does not jump ahead of a later term.
func TestSortNewestTermFirst(t *testing.T) {
	card := func(id, cycle string, ordinal int, firstMonth string, issued time.Time) domain.ReportCard {
		h := header(t)
		h.ID, h.CycleID, h.TermID, h.TermOrdinal, h.IssuedAt = id, cycle, "term-"+id, ordinal, issued
		h.Periods = nil
		h.Months = []domain.Month{mustMonth(t, firstMonth)}
		rc, err := domain.RestoreReportCard(h, domain.ReportCardPublished, domain.Average{}, nil)
		require.NoError(t, err)
		return rc
	}
	day := func(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }
	cards := []domain.ReportCard{
		card("old-t2", "cyc-2025", 2, "2025-12", day(2026, 3)),
		card("new-t1", "cyc-2026", 1, "2026-09", day(2027, 5)), // T1 re-issued after T2
		card("old-t1", "cyc-2025", 1, "2025-09", day(2025, 11)),
		card("new-t2", "cyc-2026", 2, "2026-12", day(2027, 3)),
	}

	sorted := domain.SortNewestTermFirst(cards)

	ids := make([]string, 0, len(sorted))
	for _, c := range sorted {
		ids = append(ids, c.ID())
	}
	assert.Equal(t, []string{"new-t2", "new-t1", "old-t2", "old-t1"}, ids)
	assert.Equal(t, "old-t2", cards[0].ID(), "the input is not reordered in place")
	assert.Equal(t, 2, sorted[0].Header().TermOrdinal)
}

// A cycle can mix legacy cards (ordinal 0, issued before terms carried an
// ordinal) with new ones. Within a cycle the card's first month decides, so a
// legacy T3 is still newer than a new T1; the ordinal only breaks ties.
func TestSortNewestTermFirst_MixedLegacyAndOrdinalCards(t *testing.T) {
	card := func(id string, ordinal int, firstMonth string) domain.ReportCard {
		h := header(t)
		h.ID, h.CycleID, h.TermID, h.TermOrdinal = id, "cyc-2026", "term-"+id, ordinal
		h.Periods = nil
		h.Months = []domain.Month{mustMonth(t, firstMonth)}
		rc, err := domain.RestoreReportCard(h, domain.ReportCardPublished, domain.Average{}, nil)
		require.NoError(t, err)
		return rc
	}
	cards := []domain.ReportCard{
		card("t1-new", 1, "2026-09"),
		card("t3-legacy", 0, "2027-03"),
		card("t2-legacy", 0, "2026-12"),
	}

	sorted := domain.SortNewestTermFirst(cards)

	ids := make([]string, 0, len(sorted))
	for _, c := range sorted {
		ids = append(ids, c.ID())
	}
	assert.Equal(t, []string{"t3-legacy", "t2-legacy", "t1-new"}, ids)
}

// A T15 card has no months: it is dated by the start of the period it was
// closed for (the column whose ordinal is the term's), so it sorts among the
// legacy month cards of its cycle.
func TestSortNewestTermFirst_PeriodCards(t *testing.T) {
	periodCard := func(id string, ordinal int, periods []domain.ReportCardPeriod) domain.ReportCard {
		h := header(t)
		h.ID, h.CycleID, h.TermID, h.TermOrdinal, h.Periods = id, "cyc-2026", "term-"+id, ordinal, periods
		rc, err := domain.RestoreReportCard(h, domain.ReportCardPublished, domain.Average{}, nil)
		require.NoError(t, err)
		return rc
	}
	legacy := func(id string, firstMonth string) domain.ReportCard {
		h := header(t)
		h.ID, h.CycleID, h.TermID, h.TermOrdinal, h.Periods = id, "cyc-2026", "term-"+id, 0, nil
		h.Months = []domain.Month{mustMonth(t, firstMonth)}
		rc, err := domain.RestoreReportCard(h, domain.ReportCardPublished, domain.Average{}, nil)
		require.NoError(t, err)
		return rc
	}
	cards := []domain.ReportCard{
		legacy("t1-legacy", "2026-09"),
		periodCard("t2-period", 2, cardPeriods()),
		periodCard("t3-period", 3, append(cardPeriods(), domain.ReportCardPeriod{ID: "p3", Ordinal: 3, StartsOn: date(2027, 4, 5)})),
	}
	sorted := domain.SortNewestTermFirst(cards)
	ids := make([]string, 0, len(sorted))
	for _, c := range sorted {
		ids = append(ids, c.ID())
	}
	assert.Equal(t, []string{"t3-period", "t2-period", "t1-legacy"}, ids)
}
