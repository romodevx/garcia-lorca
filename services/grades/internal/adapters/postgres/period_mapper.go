package postgres

import (
	"encoding/json"
	"math"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

// tenthsPerUnit converts a one-decimal numeric(5,2) average to exact tenths.
const tenthsPerUnit = 10

func fromDomainClosure(tenantID string, c domain.PeriodClosure) periodClosureModel {
	return periodClosureModel{
		TenantID:     tenantID,
		TermID:       c.TermID(),
		CycleID:      c.CycleID(),
		LevelID:      c.LevelID(),
		Status:       string(c.Status()),
		Version:      c.Version(),
		ClosedAt:     timePtr(c.ClosedAt()),
		ClosedBy:     stringPtr(c.ClosedBy()),
		ClosedByName: stringPtr(c.ClosedByName()),
		ReopenedAt:   timePtr(c.ReopenedAt()),
		Months:       monthTexts(c.Months()),
	}
}

func toDomainClosure(m periodClosureModel) (domain.PeriodClosure, error) {
	months, err := parseMonthTexts(m.Months)
	if err != nil {
		return domain.PeriodClosure{}, glerrors.Wrap(glerrors.KindInternal, "invalid period closure months", err)
	}
	c, err := domain.RestorePeriodClosure(domain.PeriodClosureFields{
		Months: months,
		TermID: m.TermID, CycleID: m.CycleID, LevelID: m.LevelID,
		Status: domain.PeriodStatus(m.Status), Version: m.Version,
		ClosedAt: deref(m.ClosedAt), ClosedBy: derefString(m.ClosedBy), ClosedByName: derefString(m.ClosedByName),
		ReopenedAt: deref(m.ReopenedAt),
	})
	if err != nil {
		return domain.PeriodClosure{}, glerrors.Wrap(glerrors.KindInternal, "invalid period closure row", err)
	}
	return c, nil
}

func fromDomainReportCard(rc domain.ReportCard) (reportCardModel, []reportCardLineModel, error) {
	h := rc.Header()
	card := reportCardModel{
		ID: h.ID, TenantID: h.TenantID, TermID: h.TermID, CycleID: h.CycleID, LevelID: h.LevelID,
		GroupID: h.GroupID, EnrollmentID: h.EnrollmentID, StudentID: h.StudentID, Version: h.Version,
		Status: string(rc.Status()), IssuedAt: h.IssuedAt, IssuedBy: h.IssuedBy,
		StudentName: h.StudentName, EnrollmentNumber: h.EnrollmentNumber, GroupLabel: h.GroupLabel,
		LevelName: h.LevelName, CycleName: h.CycleName, TermName: h.TermName, TermOrdinal: h.TermOrdinal,
		Months: monthTexts(h.Months), GeneralAverage: averagePtr(rc.GeneralAverage()),
		ScaleMinHundredths: h.Scale.MinHundredths(), ScaleMaxHundredths: h.Scale.MaxHundredths(),
		ScaleDecimals: h.Scale.Decimals(), ScalePassingHundredths: h.Scale.PassingHundredths(),
	}
	lines := make([]reportCardLineModel, 0, len(rc.Lines()))
	for _, l := range rc.Lines() {
		raw, err := json.Marshal(l.MonthValueStrings())
		if err != nil {
			return reportCardModel{}, nil, glerrors.Wrap(glerrors.KindInternal, "encoding report card month values", err)
		}
		lines = append(lines, reportCardLineModel{
			ReportCardID: h.ID, TenantID: h.TenantID, SubjectID: l.SubjectID, SubjectName: l.SubjectName,
			Position: l.Position, MonthValues: string(raw), TermAverage: averagePtr(l.TermAverage),
		})
	}
	return card, lines, nil
}

func toDomainReportCard(m reportCardModel, lineRows []reportCardLineModel) (domain.ReportCard, error) {
	months, err := parseMonthTexts(m.Months)
	if err != nil {
		return domain.ReportCard{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card month", err)
	}
	scale, err := domain.NewGradingScale(m.ScaleMinHundredths, m.ScaleMaxHundredths, m.ScalePassingHundredths, m.ScaleDecimals)
	if err != nil {
		return domain.ReportCard{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card scale", err)
	}
	lines := make([]domain.ReportCardLine, 0, len(lineRows))
	for _, l := range lineRows {
		line, err := toDomainLine(l)
		if err != nil {
			return domain.ReportCard{}, err
		}
		lines = append(lines, line)
	}
	rc, err := domain.RestoreReportCard(domain.ReportCardHeader{
		ID: m.ID, TenantID: m.TenantID, TermID: m.TermID, CycleID: m.CycleID, LevelID: m.LevelID,
		GroupID: m.GroupID, EnrollmentID: m.EnrollmentID, StudentID: m.StudentID, Version: m.Version,
		IssuedAt: m.IssuedAt, IssuedBy: m.IssuedBy, StudentName: m.StudentName, EnrollmentNumber: m.EnrollmentNumber,
		GroupLabel: m.GroupLabel, LevelName: m.LevelName, CycleName: m.CycleName, TermName: m.TermName,
		TermOrdinal: m.TermOrdinal, Months: months, Scale: scale,
	}, domain.ReportCardStatus(m.Status), averageOf(m.GeneralAverage), lines)
	if err != nil {
		return domain.ReportCard{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card row", err)
	}
	return rc, nil
}

func toDomainLine(l reportCardLineModel) (domain.ReportCardLine, error) {
	var raw map[string]string
	if err := json.Unmarshal([]byte(l.MonthValues), &raw); err != nil {
		return domain.ReportCardLine{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card month values", err)
	}
	values := make(map[domain.Month]domain.GradeValue, len(raw))
	for month, text := range raw {
		m, err := domain.ParseMonth(month)
		if err != nil {
			return domain.ReportCardLine{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card line month", err)
		}
		h, err := hundredthsOf(text)
		if err != nil {
			return domain.ReportCardLine{}, err
		}
		values[m] = domain.GradeValueFromHundredths(h)
	}
	return domain.ReportCardLine{
		SubjectID: l.SubjectID, SubjectName: l.SubjectName, Position: l.Position,
		MonthValues: values, TermAverage: averageOf(l.TermAverage),
	}, nil
}

// wideScale reads any stored mark back: 0–100 with two decimals, the column's
// own bounds (a snapshot is never re-judged against today's scale).
var wideScale = mustWideScale()

func mustWideScale() domain.GradingScale {
	s, err := domain.NewGradingScale(0, scaleCeilingHundredths, 1, maxDecimals)
	if err != nil {
		panic(err)
	}
	return s
}

const (
	scaleCeilingHundredths = 10000
	maxDecimals            = 2
)

// hundredthsOf reads a snapshot's exact decimal text back to hundredths.
func hundredthsOf(text string) (int, error) {
	v, err := wideScale.ParseValue(text)
	if err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "invalid report card grade text", err)
	}
	return v.Hundredths(), nil
}

// parseMonthTexts reads a text[] of "YYYY-MM" back to months.
func parseMonthTexts(raw []string) ([]domain.Month, error) {
	out := make([]domain.Month, 0, len(raw))
	for _, r := range raw {
		m, err := domain.ParseMonth(r)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func monthTexts(ms []domain.Month) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.String())
	}
	return out
}

func averagePtr(a domain.Average) *float64 {
	if !a.Present() {
		return nil
	}
	v := float64(a.Tenths()) / tenthsPerUnit
	return &v
}

func averageOf(v *float64) domain.Average {
	if v == nil {
		return domain.Average{}
	}
	return domain.AverageFromTenths(int(math.Round(*v * tenthsPerUnit)))
}
