package postgres

import (
	"encoding/json"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

// periodColumn is one report card column as report_cards.periods stores it.
type periodColumn struct {
	ID       string `json:"id"`
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name"`
	StartsOn string `json:"startsOn"`
}

// dateLayout is how a column's first day is stored ("2026-08-24").
const dateLayout = "2006-01-02"

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

		ReopenedBy:     stringPtr(c.ReopenedBy()),
		ReopenedByName: stringPtr(c.ReopenedByName()),
		ReopenReason:   stringPtr(c.ReopenReason()),
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
		ReopenedAt: deref(m.ReopenedAt), ReopenedBy: derefString(m.ReopenedBy),
		ReopenedByName: derefString(m.ReopenedByName), ReopenReason: derefString(m.ReopenReason),
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
		Months: monthTexts(h.Months), GeneralAverage: averageText(rc.GeneralAverage()),
		ScaleMinHundredths: h.Scale.MinHundredths(), ScaleMaxHundredths: h.Scale.MaxHundredths(),
		ScaleDecimals: h.Scale.Decimals(), ScalePassingHundredths: h.Scale.PassingHundredths(),
	}
	columns := make([]periodColumn, 0, len(h.Periods))
	for _, p := range h.Periods {
		columns = append(columns, periodColumn{ID: p.ID, Ordinal: p.Ordinal, Name: p.Name, StartsOn: p.StartsOn.Format(dateLayout)})
	}
	rawColumns, err := json.Marshal(columns)
	if err != nil {
		return reportCardModel{}, nil, glerrors.Wrap(glerrors.KindInternal, "encoding report card periods", err)
	}
	card.Periods = string(rawColumns)
	lines := make([]reportCardLineModel, 0, len(rc.Lines()))
	for _, l := range rc.Lines() {
		months, err := json.Marshal(l.MonthValueStrings())
		if err != nil {
			return reportCardModel{}, nil, glerrors.Wrap(glerrors.KindInternal, "encoding report card month values", err)
		}
		periods, err := json.Marshal(l.PeriodValueStrings())
		if err != nil {
			return reportCardModel{}, nil, glerrors.Wrap(glerrors.KindInternal, "encoding report card period values", err)
		}
		lines = append(lines, reportCardLineModel{
			ReportCardID: h.ID, TenantID: h.TenantID, SubjectID: l.SubjectID, SubjectName: l.SubjectName,
			Position: l.Position, MonthValues: string(months), PeriodValues: string(periods),
			TermAverage: averageText(l.Average),
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
	periods, err := parseColumns(m.Periods)
	if err != nil {
		return domain.ReportCard{}, err
	}
	general, err := averageOf(m.GeneralAverage)
	if err != nil {
		return domain.ReportCard{}, err
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
		TermOrdinal: m.TermOrdinal, Months: months, Periods: periods, Scale: scale,
	}, domain.ReportCardStatus(m.Status), general, lines)
	if err != nil {
		return domain.ReportCard{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card row", err)
	}
	return rc, nil
}

func toDomainLine(l reportCardLineModel) (domain.ReportCardLine, error) {
	monthTexts, err := decodeValues(l.MonthValues)
	if err != nil {
		return domain.ReportCardLine{}, err
	}
	months := make(map[domain.Month]domain.GradeValue, len(monthTexts))
	for month, v := range monthTexts {
		m, err := domain.ParseMonth(month)
		if err != nil {
			return domain.ReportCardLine{}, glerrors.Wrap(glerrors.KindInternal, "invalid report card line month", err)
		}
		months[m] = v
	}
	periods, err := decodeValues(l.PeriodValues)
	if err != nil {
		return domain.ReportCardLine{}, err
	}
	avg, err := averageOf(l.TermAverage)
	if err != nil {
		return domain.ReportCardLine{}, err
	}
	return domain.ReportCardLine{
		SubjectID: l.SubjectID, SubjectName: l.SubjectName, Position: l.Position,
		MonthValues: months, PeriodValues: periods, Average: avg,
	}, nil
}

// decodeValues reads a jsonb object of exact grade texts ({"key": "9.5"}). An
// empty column (a row read before its default) is no values.
func decodeValues(raw string) (map[string]domain.GradeValue, error) {
	out := map[string]domain.GradeValue{}
	if raw == "" {
		return out, nil
	}
	var texts map[string]string
	if err := json.Unmarshal([]byte(raw), &texts); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "invalid report card line values", err)
	}
	for key, text := range texts {
		h, err := hundredthsOf(text)
		if err != nil {
			return nil, err
		}
		out[key] = domain.GradeValueFromHundredths(h)
	}
	return out, nil
}

// parseColumns reads report_cards.periods back ("[]" before T15).
func parseColumns(raw string) ([]domain.ReportCardPeriod, error) {
	if raw == "" {
		return nil, nil
	}
	var cols []periodColumn
	if err := json.Unmarshal([]byte(raw), &cols); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "invalid report card periods", err)
	}
	out := make([]domain.ReportCardPeriod, 0, len(cols))
	for _, c := range cols {
		start, err := time.Parse(dateLayout, c.StartsOn)
		if err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "invalid report card period start", err)
		}
		out = append(out, domain.ReportCardPeriod{ID: c.ID, Ordinal: c.Ordinal, Name: c.Name, StartsOn: start})
	}
	return out, nil
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

// averageText is a truncated average as numeric text (NULL = none).
func averageText(a domain.Average) *string {
	if !a.Present() {
		return nil
	}
	v := hundredthsText(a.Hundredths())
	return &v
}

// averageOf reads a stored average back exactly (NULL = none).
func averageOf(v *string) (domain.Average, error) {
	if v == nil {
		return domain.Average{}, nil
	}
	h, err := hundredthsOf(*v)
	if err != nil {
		return domain.Average{}, err
	}
	return domain.AverageFromHundredths(h), nil
}
