package domain

import (
	"cmp"
	"maps"
	"slices"
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
	TermOrdinal      int // the term's order in its cycle (1 = first); 0 = unknown (issued before T8)
	// Periods are the card's columns (T15): the evaluation periods of the
	// pupil's grade from the first up to the one closed, in ordinal order.
	Periods []ReportCardPeriod
	// Months are the columns of a card issued before T15 (monthly capture);
	// a T15 card has none. Kept so an issued card reads back as it was.
	Months []Month
	Scale  GradingScale
	// Letters: the level grades with letters (preescolar). Its lines carry
	// PeriodLetters and no average; the card has no general average.
	Letters bool
}

// ReportCardPeriod is one column of a report card: an evaluation period of the
// pupil's grade as it read at issue time.
type ReportCardPeriod struct {
	ID       string
	Ordinal  int
	Name     string
	StartsOn time.Time
}

// ReportCardSubject is one subject a report card lists, in print order.
type ReportCardSubject struct {
	ID   string
	Name string
}

// ReportCardLine is one subject of a report card: its grade per period (a
// period without a grade is absent) and their average (exact mean, truncated
// to 2 decimals). A card issued before T15 carries MonthValues instead.
type ReportCardLine struct {
	SubjectID    string
	SubjectName  string
	Position     int
	PeriodValues map[string]GradeValue
	// PeriodLetters are the line's grades on a letters card, instead of values.
	PeriodLetters map[string]Letter
	MonthValues   map[Month]GradeValue
	Average       Average
}

// PeriodValueStrings is the line's grades by their text ({"<period id>": "9"},
// or "B" on a letters card).
func (l ReportCardLine) PeriodValueStrings() map[string]string {
	out := make(map[string]string, len(l.PeriodValues)+len(l.PeriodLetters))
	for id, v := range l.PeriodValues {
		out[id] = v.String()
	}
	for id, v := range l.PeriodLetters {
		out[id] = string(v)
	}
	return out
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

// BuildReportCard freezes one enrollment's grades for the header's columns
// (T15): for each subject (in the given order), its grade in each of the
// header's periods and their exact mean; then the general average — the exact
// mean of the subjects' exact averages, only subjects that have one. Grades of
// other enrollments or periods are ignored. The card is issued published. A
// letters card (h.Letters) carries each subject's letters and no average.
func BuildReportCard(h ReportCardHeader, subjects []ReportCardSubject, grades []PeriodGrade) (ReportCard, error) {
	if err := validateHeader(h); err != nil {
		return ReportCard{}, err
	}
	if h.Letters {
		return buildLettersCard(h, subjects, grades), nil
	}
	inCard := make(map[string]bool, len(h.Periods))
	for _, p := range h.Periods {
		inCard[p.ID] = true
	}
	bySubject := map[string]map[string]GradeValue{}
	for _, g := range grades {
		if g.EnrollmentID() != h.EnrollmentID || !inCard[g.PeriodID()] || g.IsLetter() {
			continue
		}
		if bySubject[g.SubjectID()] == nil {
			bySubject[g.SubjectID()] = map[string]GradeValue{}
		}
		bySubject[g.SubjectID()][g.PeriodID()] = g.Value()
	}
	lines := make([]ReportCardLine, 0, len(subjects))
	averages := make([]Average, 0, len(subjects))
	for i, s := range subjects {
		values := bySubject[s.ID]
		if values == nil {
			values = map[string]GradeValue{}
		}
		avg := SubjectAverage(slices.Collect(maps.Values(values)))
		averages = append(averages, avg)
		lines = append(lines, ReportCardLine{
			SubjectID: s.ID, SubjectName: s.Name, Position: i + 1, PeriodValues: values, Average: avg,
		})
	}
	return ReportCard{header: h, status: ReportCardPublished, generalAverage: GeneralAverage(averages), lines: lines}, nil
}

// buildLettersCard is BuildReportCard for a level that grades with letters.
func buildLettersCard(h ReportCardHeader, subjects []ReportCardSubject, grades []PeriodGrade) ReportCard {
	inCard := make(map[string]bool, len(h.Periods))
	for _, p := range h.Periods {
		inCard[p.ID] = true
	}
	bySubject := map[string]map[string]Letter{}
	for _, g := range grades {
		if g.EnrollmentID() != h.EnrollmentID || !inCard[g.PeriodID()] || !g.IsLetter() {
			continue
		}
		if bySubject[g.SubjectID()] == nil {
			bySubject[g.SubjectID()] = map[string]Letter{}
		}
		bySubject[g.SubjectID()][g.PeriodID()] = g.Letter()
	}
	lines := make([]ReportCardLine, 0, len(subjects))
	for i, s := range subjects {
		letters := bySubject[s.ID]
		if letters == nil {
			letters = map[string]Letter{}
		}
		lines = append(lines, ReportCardLine{
			SubjectID: s.ID, SubjectName: s.Name, Position: i + 1,
			PeriodValues: map[string]GradeValue{}, PeriodLetters: letters,
		})
	}
	return ReportCard{header: h, status: ReportCardPublished, lines: lines}
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
	if len(h.Periods) == 0 && len(h.Months) == 0 {
		return glerrors.New(glerrors.KindInvalid, "report card periods are required")
	}
	return nil
}

// SameGradesAs reports whether r carries the grades other did: the same
// subjects (by id), and per subject the same period (or month) values and
// average, and the same general average — averages compared as they read
// (truncated), so a fresh exact average equals the stored one. Ids, versions, names and print order do not
// count — a re-close that changes none of these has nothing new to tell the
// family.
func (r ReportCard) SameGradesAs(other ReportCard) bool {
	if !r.generalAverage.SameAs(other.generalAverage) || len(r.lines) != len(other.lines) {
		return false
	}
	previous := make(map[string]ReportCardLine, len(other.lines))
	for _, l := range other.lines {
		previous[l.SubjectID] = l
	}
	for _, l := range r.lines {
		p, ok := previous[l.SubjectID]
		if !ok || !l.Average.SameAs(p.Average) ||
			!maps.Equal(l.PeriodValueStrings(), p.PeriodValueStrings()) ||
			!maps.Equal(l.MonthValueStrings(), p.MonthValueStrings()) {
			return false
		}
	}
	return true
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

// SortNewestTermFirst returns cards (a copy; the input keeps its order) newest
// period first: the newest cycle first, then, within a cycle, the card that
// starts latest (its first month; a T15 card, the start of the period it was
// closed for). A cycle is dated by the earliest month any of its
// cards covers — never by its name, which the school chooses freely, nor by
// issue date, which a reopen moves. The term ordinal only breaks ties: legacy
// cards carry ordinal 0, so ordering by it first would put a legacy T3 behind
// a new T1. Remaining ties fall back to ids so the order is deterministic.
func SortNewestTermFirst(cards []ReportCard) []ReportCard {
	cycleStart := map[string]time.Time{}
	for _, c := range cards {
		first := startOf(c.header)
		if first.IsZero() {
			continue // only a zero ReportCard has no columns; it sorts last
		}
		if start, ok := cycleStart[c.header.CycleID]; !ok || first.Before(start) {
			cycleStart[c.header.CycleID] = first
		}
	}
	out := slices.Clone(cards)
	slices.SortStableFunc(out, func(a, b ReportCard) int {
		ha, hb := a.header, b.header
		return cmp.Or(
			cycleStart[hb.CycleID].Compare(cycleStart[ha.CycleID]),
			cmp.Compare(hb.CycleID, ha.CycleID),
			startOf(hb).Compare(startOf(ha)),
			cmp.Compare(hb.TermOrdinal, ha.TermOrdinal),
			cmp.Compare(ha.TermID, hb.TermID),
			cmp.Compare(ha.ID, hb.ID),
		)
	})
	return out
}

// startOf dates a card: the first day of its first month (issued before
// T15), or the start of the period it was closed for — the column whose
// ordinal is the term's, else its last column. A card without columns answers
// the zero time, so it sorts last.
func startOf(h ReportCardHeader) time.Time {
	if len(h.Months) > 0 {
		return h.Months[0].FirstDay()
	}
	if len(h.Periods) == 0 {
		return time.Time{}
	}
	for _, p := range h.Periods {
		if p.Ordinal == h.TermOrdinal {
			return p.StartsOn
		}
	}
	return h.Periods[len(h.Periods)-1].StartsOn
}
