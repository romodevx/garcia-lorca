package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// ReportCardStatus is whether a report card is the one parents see.
type ReportCardStatus string

// A report card is published when issued; the next close of its period
// supersedes it with a new version.
const (
	ReportCardPublished  ReportCardStatus = "published"
	ReportCardSuperseded ReportCardStatus = "superseded"
)

// ReportCardHeader is everything a report card freezes besides its lines: the
// identities it belongs to and the names as they read at issue time, so a
// later rename never changes an issued report card.
type ReportCardHeader struct {
	ID               string
	TenantID         string
	TermID           string
	CycleID          string
	LevelID          string
	GroupID          string
	EnrollmentID     string
	StudentID        string
	Version          int
	IssuedAt         time.Time
	IssuedBy         string
	StudentName      string
	EnrollmentNumber string
	GroupLabel       string
	LevelName        string
	CycleName        string
	TermName         string
	Months           []Month
	Scale            GradingScale
}

// ReportCardSubject is one subject a report card lists, in print order.
type ReportCardSubject struct {
	ID   string
	Name string
}

// ReportCardLine is one subject of a report card: its grade per month of the
// term (a month without a grade is absent) and its trimester grade.
type ReportCardLine struct {
	SubjectID   string
	SubjectName string
	Position    int
	MonthValues map[Month]GradeValue
	TermAverage Average
}

// MonthValueStrings is MonthValues keyed and valued by their text
// ({"2026-09": "9.5"}).
func (l ReportCardLine) MonthValueStrings() map[string]string {
	out := make(map[string]string, len(l.MonthValues))
	for m, v := range l.MonthValues {
		out[m.String()] = v.String()
	}
	return out
}

// ReportCard is the frozen snapshot of one enrollment's grades for one period
// (T7), issued when the period closes.
type ReportCard struct {
	header         ReportCardHeader
	status         ReportCardStatus
	generalAverage Average
	lines          []ReportCardLine
}

// BuildReportCard freezes one enrollment's grades for the header's term: for
// each subject (in the given order), the grades of that enrollment in the
// term's months and their half-up mean; then the general average of the
// subjects that have one. Grades of other enrollments or other months are
// ignored. The card is issued published.
func BuildReportCard(h ReportCardHeader, subjects []ReportCardSubject, grades []MonthlyGrade) (ReportCard, error) {
	if err := validateHeader(h); err != nil {
		return ReportCard{}, err
	}
	bySubject := map[string]map[Month]GradeValue{}
	for _, g := range grades {
		if g.EnrollmentID() != h.EnrollmentID || !ContainsMonth(h.Months, g.Month()) {
			continue
		}
		if bySubject[g.SubjectID()] == nil {
			bySubject[g.SubjectID()] = map[Month]GradeValue{}
		}
		bySubject[g.SubjectID()][g.Month()] = g.Value()
	}
	lines := make([]ReportCardLine, 0, len(subjects))
	averages := make([]Average, 0, len(subjects))
	for i, s := range subjects {
		monthValues := bySubject[s.ID]
		if monthValues == nil {
			monthValues = map[Month]GradeValue{}
		}
		values := make([]GradeValue, 0, len(monthValues))
		for _, v := range monthValues {
			values = append(values, v)
		}
		avg := SubjectAverage(values)
		averages = append(averages, avg)
		lines = append(lines, ReportCardLine{
			SubjectID: s.ID, SubjectName: s.Name, Position: i + 1, MonthValues: monthValues, TermAverage: avg,
		})
	}
	return ReportCard{header: h, status: ReportCardPublished, generalAverage: GeneralAverage(averages), lines: lines}, nil
}

// RestoreReportCard rebuilds a stored report card exactly as it was frozen.
func RestoreReportCard(h ReportCardHeader, status ReportCardStatus, general Average, lines []ReportCardLine) (ReportCard, error) {
	if err := validateHeader(h); err != nil {
		return ReportCard{}, err
	}
	if status != ReportCardPublished && status != ReportCardSuperseded {
		return ReportCard{}, glerrors.New(glerrors.KindInvalid, "report card status must be published or superseded")
	}
	return ReportCard{header: h, status: status, generalAverage: general, lines: lines}, nil
}

func validateHeader(h ReportCardHeader) error {
	required := []struct{ value, what string }{
		{h.ID, "report card id"},
		{h.TenantID, "tenant id"},
		{h.TermID, "term id"},
		{h.EnrollmentID, "enrollment id"},
		{h.StudentID, "student id"},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return glerrors.New(glerrors.KindInvalid, r.what+" is required")
		}
	}
	if h.Version < 1 {
		return glerrors.New(glerrors.KindInvalid, "report card version must be at least 1")
	}
	if len(h.Months) == 0 {
		return glerrors.New(glerrors.KindInvalid, "report card months are required")
	}
	return nil
}

// Accessors.
func (r ReportCard) Header() ReportCardHeader { return r.header }
func (r ReportCard) ID() string               { return r.header.ID }
func (r ReportCard) TermID() string           { return r.header.TermID }
func (r ReportCard) EnrollmentID() string     { return r.header.EnrollmentID }
func (r ReportCard) StudentID() string        { return r.header.StudentID }
func (r ReportCard) StudentName() string      { return r.header.StudentName }
func (r ReportCard) Version() int             { return r.header.Version }
func (r ReportCard) IssuedAt() time.Time      { return r.header.IssuedAt }
func (r ReportCard) Status() ReportCardStatus { return r.status }
func (r ReportCard) GeneralAverage() Average  { return r.generalAverage }
func (r ReportCard) Lines() []ReportCardLine  { return r.lines }
func (r ReportCard) WithStatus(s ReportCardStatus) ReportCard {
	r.status = s
	return r
}
