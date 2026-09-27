package postgres

import (
	"math"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

// hundredthsPerUnit converts numeric(5,2) marks to exact hundredths. Any value
// with at most two decimals up to 100 survives the float64 trip exactly once
// rounded, so the float never decides anything: the domain judged the typed
// text before it was stored.
const hundredthsPerUnit = 100

func fromDomainMonthly(g domain.MonthlyGrade) monthlyGradeModel {
	return monthlyGradeModel{
		ID:           g.ID(),
		TenantID:     g.TenantID(),
		EnrollmentID: g.EnrollmentID(),
		SubjectID:    g.SubjectID(),
		TeacherID:    g.TeacherID(),
		Month:        g.Month().FirstDay(),
		Value:        float64(g.Value().Hundredths()) / hundredthsPerUnit,
		CapturedBy:   g.CapturedBy(),
	}
}

func toDomainMonthly(m monthlyGradeModel) (domain.MonthlyGrade, error) {
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID:           m.ID,
		TenantID:     m.TenantID,
		EnrollmentID: m.EnrollmentID,
		SubjectID:    m.SubjectID,
		TeacherID:    m.TeacherID,
		CapturedBy:   m.CapturedBy,
		Month:        domain.MonthOf(m.Month),
		Value:        domain.GradeValueFromHundredths(int(math.Round(m.Value * hundredthsPerUnit))),
		UpdatedAt:    m.UpdatedAt,
	})
	if err != nil {
		return domain.MonthlyGrade{}, glerrors.Wrap(glerrors.KindInternal, "invalid monthly grade row", err)
	}
	return g, nil
}
