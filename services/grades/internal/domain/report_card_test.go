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

func grade(t *testing.T, enrollment, subject, month string, hundredths int) domain.MonthlyGrade {
	t.Helper()
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID: "g-" + subject + month, TenantID: "ten", EnrollmentID: enrollment, SubjectID: subject,
		TeacherID: "tch", CapturedBy: "usr", Month: mustMonth(t, month), Value: domain.GradeValueFromHundredths(hundredths),
	})
	require.NoError(t, err)
	return g
}

func header(t *testing.T) domain.ReportCardHeader {
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	return domain.ReportCardHeader{
		ID: "rc-1", TenantID: "ten", TermID: "term-1", CycleID: "cyc", LevelID: "lvl", GroupID: "grp",
		EnrollmentID: "enr-1", StudentID: "stu-1", Version: 1, IssuedAt: closeAt, IssuedBy: "usr-dir",
		StudentName: "Ana López", EnrollmentNumber: "2026-00001", GroupLabel: "1° A", LevelName: "Primaria",
		CycleName: "2026-2027", TermName: "1.er trimestre",
		Months: []domain.Month{mustMonth(t, "2026-09"), mustMonth(t, "2026-10"), mustMonth(t, "2026-11")},
		Scale:  scale,
	}
}

// A report card snapshots each subject's grades in the term's months, their
// half-up mean, and the general average of the subjects that have one.
// Grades outside the term's months, or of other enrollments, are ignored.
func TestBuildReportCard(t *testing.T) {
	subjects := []domain.ReportCardSubject{{ID: "mat", Name: "Matemáticas"}, {ID: "esp", Name: "Español"}, {ID: "art", Name: "Artes"}}
	grades := []domain.MonthlyGrade{
		grade(t, "enr-1", "mat", "2026-09", 840),
		grade(t, "enr-1", "mat", "2026-10", 850), // mat: 8.45 → 8.5
		grade(t, "enr-1", "esp", "2026-09", 800),
		grade(t, "enr-1", "esp", "2026-11", 850),  // esp: 8.25 → 8.3 (October missing, ignored)
		grade(t, "enr-1", "mat", "2026-12", 500),  // outside the term
		grade(t, "enr-2", "art", "2026-09", 1000), // another pupil
	}
	rc, err := domain.BuildReportCard(header(t), subjects, grades)
	require.NoError(t, err)

	assert.Equal(t, domain.ReportCardPublished, rc.Status())
	require.Len(t, rc.Lines(), 3)
	mat, esp, art := rc.Lines()[0], rc.Lines()[1], rc.Lines()[2]
	assert.Equal(t, "Matemáticas", mat.SubjectName)
	assert.Equal(t, 1, mat.Position)
	assert.Equal(t, "8.5", mat.TermAverage.String())
	assert.Equal(t, map[string]string{"2026-09": "8.4", "2026-10": "8.5"}, mat.MonthValueStrings())
	assert.Equal(t, "8.3", esp.TermAverage.String())
	assert.Equal(t, 2, esp.Position)
	assert.False(t, art.TermAverage.Present(), "a subject with no grade has no trimester grade")
	assert.Empty(t, art.MonthValues)
	assert.Equal(t, "8.4", rc.GeneralAverage().String(), "mean of 8.5 and 8.3; the ungraded subject is excluded")
	assert.Equal(t, "Ana López", rc.StudentName())
	assert.Equal(t, 1, rc.Version())
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
	h.Months = nil
	_, err = domain.BuildReportCard(h, nil, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

// A stored report card is rebuilt as it was frozen, including its status.
func TestRestoreReportCard(t *testing.T) {
	h := header(t)
	lines := []domain.ReportCardLine{{SubjectID: "mat", SubjectName: "Matemáticas", Position: 1,
		MonthValues: map[domain.Month]domain.GradeValue{mustMonth(t, "2026-09"): domain.GradeValueFromHundredths(950)},
		TermAverage: domain.AverageFromTenths(95)}}
	rc, err := domain.RestoreReportCard(h, domain.ReportCardSuperseded, domain.AverageFromTenths(95), lines)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardSuperseded, rc.Status())
	assert.Equal(t, "9.5", rc.GeneralAverage().String())
	assert.Equal(t, time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC), rc.IssuedAt())

	_, err = domain.RestoreReportCard(h, "weird", domain.Average{}, nil)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

// Owner decision: a re-close notifies only the pupils whose card changed. Two
// cards carry the same grades when their subject set, month values, trimester
// grades and general average agree; ids, versions, names and order do not count.
func TestReportCardSameGradesAs(t *testing.T) {
	subjects := []domain.ReportCardSubject{{ID: "mat", Name: "Matemáticas"}, {ID: "esp", Name: "Español"}}
	base := []domain.MonthlyGrade{grade(t, "enr-1", "mat", "2026-09", 840), grade(t, "enr-1", "esp", "2026-10", 900)}
	build := func(h domain.ReportCardHeader, subs []domain.ReportCardSubject, gs []domain.MonthlyGrade) domain.ReportCard {
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
		{"a month value changed", build(v2, subjects, []domain.MonthlyGrade{grade(t, "enr-1", "mat", "2026-09", 850), base[1]}), false},
		{"a grade added in another month", build(v2, subjects, append(base, grade(t, "enr-1", "mat", "2026-11", 840))), false},
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

// A family reads its child's report cards newest period first: the newest
// cycle first (cycles are dated by the earliest month any of their cards
// covers, never by name or issue date), then the highest term ordinal. A term
// re-issued late (a reopen) does not jump ahead of a later term.
func TestSortNewestTermFirst(t *testing.T) {
	card := func(id, cycle string, ordinal int, firstMonth string, issued time.Time) domain.ReportCard {
		h := header(t)
		h.ID, h.CycleID, h.TermID, h.TermOrdinal, h.IssuedAt = id, cycle, "term-"+id, ordinal, issued
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
