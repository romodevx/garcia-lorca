package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

func TestNewPeriodGrade(t *testing.T) {
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "g1", TenantID: "ten", EnrollmentID: "enr", SubjectID: "mat", PeriodID: "p1",
		CapturedBy: "usr", CapturedByName: "Gabriela Rojas", Value: domain.GradeValueFromHundredths(900),
	})
	require.NoError(t, err)
	assert.Equal(t, "p1", g.PeriodID())
	assert.Equal(t, "9", g.Value().String())
	assert.Equal(t, "Gabriela Rojas", g.CapturedByName())

	for _, missing := range []string{"ID", "TenantID", "EnrollmentID", "SubjectID", "PeriodID", "CapturedBy"} {
		f := domain.PeriodGradeFields{ID: "g1", TenantID: "ten", EnrollmentID: "enr", SubjectID: "mat", PeriodID: "p1", CapturedBy: "usr"}
		switch missing {
		case "ID":
			f.ID = ""
		case "TenantID":
			f.TenantID = ""
		case "EnrollmentID":
			f.EnrollmentID = ""
		case "SubjectID":
			f.SubjectID = ""
		case "PeriodID":
			f.PeriodID = ""
		case "CapturedBy":
			f.CapturedBy = " "
		}
		_, err := domain.NewPeriodGrade(f)
		assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), missing)
	}
}

// Owner decision (T15): grades are captured as INTEGERS ONLY, whatever the
// level's decimals say. The text is judged exactly; range first, then
// wholeness.
func TestParseWholeValue(t *testing.T) {
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	cases := []struct {
		text string
		want string
		code domain.RefusalCode
	}{
		{"9", "9", ""},
		{"10", "10", ""},
		{"5", "5", ""},
		{" 7 ", "7", ""},
		{"9.0", "9", ""},
		{"9.5", "", domain.CodeGradeNotInteger},
		{"8.01", "", domain.CodeGradeNotInteger},
		{"11", "", domain.CodeGradeOutOfRange},
		{"4", "", domain.CodeGradeOutOfRange},
		{"10.5", "", domain.CodeGradeOutOfRange},
		{"abc", "", domain.CodeGradeNotNumber},
		{"1e1", "", domain.CodeGradeNotNumber},
		{"", "", domain.CodeGradeNotNumber},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			v, err := scale.ParseWholeValue(c.text)
			if c.code == "" {
				require.NoError(t, err)
				assert.Equal(t, c.want, v.String())
				return
			}
			assert.Equal(t, c.code, domain.RefusalCodeOf(err))
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
		})
	}
	_, err = scale.ParseWholeValue("9.5")
	assert.Equal(t, "Solo se permiten calificaciones enteras", glerrors.SafeMessage(err))
}
