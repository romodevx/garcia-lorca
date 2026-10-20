package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// gradeWithLetters turns the fixture's preescolar into a level that grades
// with letters (owner, 20-oct), with its three trimesters.
func gradeWithLetters(f gradesFixture) {
	ref := f.catalog.levelByGroup[kinderGroupID]
	ref.GradingLetters = true
	f.catalog.levelByGroup[kinderGroupID] = ref
	f.evals.byGrade[gradeKinder] = trimesters(gradeKinder)
	f.catalog.assignments = append(f.catalog.assignments, f.asg.assignments[kinderAsgID])
}

func kinderInput(value string) usecase.UpsertPeriodGradeInput {
	return usecase.UpsertPeriodGradeInput{
		AssignmentID: kinderAsgID, PeriodID: gradeKinder + "-" + period1, EnrollmentID: "enr-kinder", Value: value,
		CapturedBy: capturer, CapturedByName: capturerName,
	}
}

func TestPreescolarCapturesALetter(t *testing.T) {
	f := newGradesFixture(t)
	gradeWithLetters(f)

	g, err := f.svc.Upsert(context.Background(), kinderInput(" b "))
	require.NoError(t, err)
	assert.Equal(t, domain.LetterB, g.Letter())
	assert.True(t, g.IsLetter())
}

func TestPreescolarRefusesANumber(t *testing.T) {
	f := newGradesFixture(t)
	gradeWithLetters(f)

	_, err := f.svc.Upsert(context.Background(), kinderInput("9"))
	assert.Equal(t, domain.CodeGradeNotLetter, domain.RefusalCodeOf(err))
}

func TestANumericLevelRefusesALetter(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "A"))
	assert.Equal(t, domain.CodeGradeNotNumber, domain.RefusalCodeOf(err))
}

func TestPreescolarSheetIsEditableAndShowsTheLetter(t *testing.T) {
	f := newGradesFixture(t)
	gradeWithLetters(f)
	_, err := f.svc.Upsert(context.Background(), kinderInput("A"))
	require.NoError(t, err)

	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, kinderAsgID, "")
	require.NoError(t, err)
	assert.True(t, sheet.GradingLetters)
	assert.False(t, sheet.GradingEnabled, "letters are not numbers")
	assert.True(t, sheet.Editable, sheet.ReadOnlyReason)
	require.Len(t, sheet.Rows, 1)
	assert.True(t, sheet.Rows[0].HasValue)
	assert.Equal(t, domain.LetterA, sheet.Rows[0].Letter)
	assert.True(t, sheet.Rows[0].Editable, sheet.Rows[0].ReadOnlyReason)
}

func TestPreescolarEnrollmentListsItsSubjectsAndLetters(t *testing.T) {
	f := newGradesFixture(t)
	gradeWithLetters(f)
	_, err := f.svc.Upsert(context.Background(), kinderInput("C"))
	require.NoError(t, err)

	got, err := f.svc.ForEnrollment(context.Background(), "enr-kinder")
	require.NoError(t, err)
	assert.True(t, got.GradingLetters)
	assert.Equal(t, []string{subjectID}, got.SubjectIDs, "the group's subjects, before every one is graded")
	require.Len(t, got.Grades, 1)
	assert.Equal(t, "C", got.Grades[0].Text())
}
