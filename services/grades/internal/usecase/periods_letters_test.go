package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

const kinderSubject = "subj-lenguajes"

// lettersPreescolar makes the fixture's preescolar grade with letters: one
// group with one subject, its trimesters, and its pupil's name.
func lettersPreescolar(t *testing.T, f periodsFixture) {
	t.Helper()
	f.catalog.levelCatalogs[levelPreescolar] = ports.LevelCatalog{
		LevelName: "Preescolar", GradingLetters: true,
		Groups:       []ports.GroupRef{{ID: kinderGroupID, Label: "K · A", GradeLevelID: gradeKinder}},
		Assignments:  []ports.AssignmentRef{{ID: kinderAsgID, GroupID: kinderGroupID, SubjectID: kinderSubject, TeacherID: teacherID, CycleID: cycleID}},
		SubjectNames: map[string]string{kinderSubject: "Lenguajes"},
		TeacherNames: map[string]string{teacherID: "Gabriela Rojas"},
	}
	f.evals.byGrade[gradeKinder] = trimesters(gradeKinder)
	f.directory.names["stu-5"] = ports.StudentRef{ID: "stu-5", Name: "Kim Ortiz", EnrollmentNumber: "2026-00005"}
}

func (f periodsFixture) captureLetter(t *testing.T, enrollment, subject, period string, l domain.Letter) {
	t.Helper()
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "pg-" + enrollment + subject + period, TenantID: testTenant, EnrollmentID: enrollment, SubjectID: subject,
		PeriodID: period, CapturedBy: capturer, Letter: l,
	})
	require.NoError(t, err)
	f.grades.rows[gradeKey(enrollment, subject, period)] = g
}

// Preescolar's period now lists, counts its letters and closes: the card
// carries the letter and no average.
func TestPreescolarPeriodClosesWithLetters(t *testing.T) {
	f := newPeriodsFixture(t)
	lettersPreescolar(t, f)

	periods, err := f.svc.ListPeriods(context.Background(), cycleID)
	require.NoError(t, err)
	var kinder *struct{ expected, captured int }
	for _, p := range periods {
		if p.Term.ID == "term-kinder" {
			kinder = &struct{ expected, captured int }{p.Expected, p.Captured}
		}
	}
	require.NotNil(t, kinder, "a letters level lists its periods")
	assert.Equal(t, 1, kinder.expected)
	assert.Equal(t, 0, kinder.captured)

	f.captureLetter(t, "enr-kid", kinderSubject, gradeKinder+"-"+period1, domain.LetterA)
	_, err = f.svc.Close(context.Background(), "term-kinder", false, dirActor)
	require.NoError(t, err)
	require.Len(t, f.periods.cards, 1)
	card := f.periods.cards[0]
	assert.True(t, card.Header().Letters)
	assert.False(t, card.GeneralAverage().Present())
	assert.Equal(t, map[string]string{gradeKinder + "-" + period1: "A"}, card.Lines()[0].PeriodValueStrings())
}
