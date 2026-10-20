package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

// Preescolar grades with letters (owner, 20-oct): A Excelente, B Bien,
// C Suficiente, D Requiere apoyo. Typed in either case, with spaces around.
func TestParseLetter(t *testing.T) {
	for text, want := range map[string]domain.Letter{"A": domain.LetterA, " b ": domain.LetterB, "c": domain.LetterC, "D": domain.LetterD} {
		got, err := domain.ParseLetter(text)
		require.NoError(t, err, text)
		assert.Equal(t, want, got, text)
	}
	for _, bad := range []string{"", "E", "AB", "9", "Excelente"} {
		_, err := domain.ParseLetter(bad)
		assert.Equal(t, domain.CodeGradeNotLetter, domain.RefusalCodeOf(err), bad)
	}
}

func TestLetterMeaning(t *testing.T) {
	assert.Equal(t, "Excelente", domain.LetterA.Meaning())
	assert.Equal(t, "Bien", domain.LetterB.Meaning())
	assert.Equal(t, "Suficiente", domain.LetterC.Meaning())
	assert.Equal(t, "Requiere apoyo", domain.LetterD.Meaning())
}

// A period grade holds a number or a letter: the letter one says so.
func TestPeriodGradeWithALetter(t *testing.T) {
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "g1", TenantID: "t1", EnrollmentID: "e1", SubjectID: "s1", PeriodID: "p1", CapturedBy: "u1",
		Letter: domain.LetterB,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.LetterB, g.Letter())
	assert.True(t, g.IsLetter())
	assert.Equal(t, "B", g.Text())

	n, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "g2", TenantID: "t1", EnrollmentID: "e1", SubjectID: "s1", PeriodID: "p1", CapturedBy: "u1",
		Value: domain.GradeValueFromHundredths(900),
	})
	require.NoError(t, err)
	assert.False(t, n.IsLetter())
	assert.Equal(t, "9", n.Text())
}

func lgrade(t *testing.T, enrollment, subject, period string, l domain.Letter) domain.PeriodGrade {
	t.Helper()
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: enrollment + subject + period, TenantID: "ten", EnrollmentID: enrollment, SubjectID: subject,
		PeriodID: period, CapturedBy: "usr", Letter: l,
	})
	require.NoError(t, err)
	return g
}

// A preescolar report card carries each subject's letter per period and no
// average at all: letters are never averaged.
func TestBuildReportCardWithLetters(t *testing.T) {
	h := header(t)
	h.Letters = true
	subjects := []domain.ReportCardSubject{{ID: "len", Name: "Lenguajes"}, {ID: "eti", Name: "Ética"}}
	grades := []domain.PeriodGrade{
		lgrade(t, "enr-1", "len", "p1", domain.LetterA),
		lgrade(t, "enr-1", "len", "p2", domain.LetterB),
		lgrade(t, "enr-1", "eti", "p2", domain.LetterD),
		lgrade(t, "enr-2", "len", "p1", domain.LetterC),
	}
	rc, err := domain.BuildReportCard(h, subjects, grades)
	require.NoError(t, err)
	assert.True(t, rc.Header().Letters)
	assert.False(t, rc.GeneralAverage().Present(), "no general average with letters")
	lines := rc.Lines()
	require.Len(t, lines, 2)
	assert.Equal(t, map[string]string{"p1": "A", "p2": "B"}, lines[0].PeriodValueStrings())
	assert.Equal(t, map[string]string{"p2": "D"}, lines[1].PeriodValueStrings())
	assert.False(t, lines[0].Average.Present(), "no subject average with letters")

	again, err := domain.BuildReportCard(h, subjects, grades[:2])
	require.NoError(t, err)
	assert.False(t, rc.SameGradesAs(again), "a letter that changed is news for the family")
	same, err := domain.BuildReportCard(h, subjects, grades)
	require.NoError(t, err)
	assert.True(t, rc.SameGradesAs(same))
}

// A level switched from letters to numbers mid-cycle: a numeric card never
// reads a captured letter as a zero.
func TestNumericReportCardIgnoresLetters(t *testing.T) {
	grades := []domain.PeriodGrade{
		pgrade(t, "enr-1", "len", "p1", 9),
		lgrade(t, "enr-1", "len", "p2", domain.LetterA),
	}
	rc, err := domain.BuildReportCard(header(t), []domain.ReportCardSubject{{ID: "len", Name: "Lenguajes"}}, grades)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"p1": "9"}, rc.Lines()[0].PeriodValueStrings())
	assert.Equal(t, "9", rc.GeneralAverage().String())
}
