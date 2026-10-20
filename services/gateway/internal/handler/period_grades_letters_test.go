package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// letterGrades answers the upsert with the letter it was sent, as grades does.
type letterGrades struct{ fakePeriodGrades }

func (f *letterGrades) UpsertPeriodGrade(ctx context.Context, in client.PeriodGradeInput) (client.PeriodGrade, error) {
	g, err := f.fakePeriodGrades.UpsertPeriodGrade(ctx, in)
	g.Value = "B"
	return g, err
}

// Preescolar's cell is a letter: PUT takes it as a JSON string and answers it
// in "letter", with "value" null.
func TestPeriodPutTakesALetter(t *testing.T) {
	grades := &letterGrades{}
	rec := httptest.NewRecorder()
	directorHandler(grades).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/period-grades", putBody(`" b "`)))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "B", grades.upsertIn.Value, "the letter reaches grades trimmed and upper-cased")
	assert.JSONEq(t, `{"enrollmentId":"`+mEnr+`","periodId":"`+mPer+`","value":null,"letter":"B","updatedAt":"2026-10-03T15:04:05Z"}`, rec.Body.String())
}

func TestPeriodPutRefusesAStringThatIsNotALetter(t *testing.T) {
	for _, body := range []string{`"9"`, `"E"`, `"AB"`, `""`} {
		grades := &fakePeriodGrades{}
		rec := httptest.NewRecorder()
		directorHandler(grades).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/period-grades", putBody(body)))
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Equal(t, "GRADE_NOT_NUMBER", rec.Header().Get("X-Error-Code"), body)
		assert.Zero(t, grades.upsertCalls, body)
	}
}

// The sheet of a letters level says so, and each captured cell is a letter.
func TestPeriodSheetOfALettersLevel(t *testing.T) {
	sheet := sheetFixture()
	sheet.GradingEnabled, sheet.GradingLetters = false, true
	sheet.Rows[1].Value = "A"
	rec := httptest.NewRecorder()
	directorHandler(&fakePeriodGrades{sheet: sheet}).Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/period-sheet?assignmentId="+mAsg, ""))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"gradingLetters":true`)
	assert.Contains(t, rec.Body.String(), `"name":"Ávila Ana","value":null,"letter":"A"`)
}
