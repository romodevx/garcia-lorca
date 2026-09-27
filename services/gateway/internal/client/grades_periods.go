package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
)

// Period is one term with its closure state and capture progress (T7).
// Status is "open" or "closed"; Version counts the closes (0 = never closed).
type Period struct {
	TermID       string
	TermName     string
	Ordinal      int
	LevelID      string
	LevelName    string
	Months       []string
	Status       string
	ClosedAt     time.Time
	ClosedByName string
	Version      int
	Captured     int
	Expected     int
}

// MissingGrades is how many captures one group+subject still owes.
type MissingGrades struct {
	GroupID      string
	GroupLabel   string
	SubjectID    string
	SubjectName  string
	TeacherName  string
	MissingCount int
}

// PeriodProgress is one term's capture progress and what is missing.
type PeriodProgress struct {
	TermID   string
	Months   []string
	Expected int
	Captured int
	Missing  []MissingGrades
}

// ClosePeriodInput closes a term on behalf of the authenticated director.
type ClosePeriodInput struct {
	TermID    string
	Force     bool
	ActorID   string
	ActorName string
}

// ClosePeriodResult is what a close answers.
type ClosePeriodResult struct {
	Status      string
	Version     int
	ReportCards int
}

// ReportCardFilter narrows a report-card list; empty fields do not filter.
type ReportCardFilter struct {
	TermID    string
	GroupID   string
	StudentID string
}

// ReportCardSummary is one report card in a list. GeneralAverage is one
// decimal ("8.5") or "" when there is none.
type ReportCardSummary struct {
	ID             string
	TermID         string
	TermName       string
	StudentID      string
	StudentName    string
	GroupID        string
	GroupLabel     string
	Version        int
	IssuedAt       time.Time
	GeneralAverage string
	EnrollmentID   string
}

// ReportCardLine is one subject of a report card: month → exact grade text,
// and the one-decimal trimester grade ("" = none).
type ReportCardLine struct {
	SubjectID   string
	SubjectName string
	Position    int
	MonthValues map[string]string
	TermAverage string
}

// ReportCard is the frozen snapshot a period's close issued.
type ReportCard struct {
	ID               string
	TermID           string
	CycleID          string
	LevelID          string
	GroupID          string
	EnrollmentID     string
	StudentID        string
	Version          int
	Status           string
	IssuedAt         time.Time
	IssuedBy         string
	StudentName      string
	EnrollmentNumber string
	GroupLabel       string
	LevelName        string
	CycleName        string
	TermName         string
	Months           []string
	GeneralAverage   string
	Scale            GradeScale
	Lines            []ReportCardLine
}

// ListPeriods lists a cycle's terms with their closure and progress.
func (c *GradesClient) ListPeriods(ctx context.Context, cycleID string) ([]Period, error) {
	res, err := c.rpc.ListPeriods(ctx, connect.NewRequest(&gradesv1.ListPeriodsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, err
	}
	out := make([]Period, 0, len(res.Msg.GetPeriods()))
	for _, p := range res.Msg.GetPeriods() {
		out = append(out, Period{
			TermID: p.GetTermId(), TermName: p.GetTermName(), Ordinal: int(p.GetOrdinal()), LevelID: p.GetLevelId(),
			LevelName: p.GetLevelName(), Months: p.GetMonths(), Status: p.GetStatus(), ClosedAt: timeOrZero(p.GetClosedAt()),
			ClosedByName: p.GetClosedByName(), Version: int(p.GetVersion()),
			Captured: int(p.GetProgress().GetCaptured()), Expected: int(p.GetProgress().GetExpected()),
		})
	}
	return out, nil
}

// GetPeriodProgress reads one term's progress and what is missing.
func (c *GradesClient) GetPeriodProgress(ctx context.Context, termID string) (PeriodProgress, error) {
	res, err := c.rpc.GetPeriodProgress(ctx, connect.NewRequest(&gradesv1.GetPeriodProgressRequest{TermId: termID}))
	if err != nil {
		return PeriodProgress{}, err
	}
	m := res.Msg
	missing := make([]MissingGrades, 0, len(m.GetMissing()))
	for _, g := range m.GetMissing() {
		missing = append(missing, MissingGrades{
			GroupID: g.GetGroupId(), GroupLabel: g.GetGroupLabel(), SubjectID: g.GetSubjectId(), SubjectName: g.GetSubjectName(),
			TeacherName: g.GetTeacherName(), MissingCount: int(g.GetMissingCount()),
		})
	}
	return PeriodProgress{TermID: m.GetTermId(), Months: m.GetMonths(), Expected: int(m.GetExpected()), Captured: int(m.GetCaptured()), Missing: missing}, nil
}

// ClosePeriod closes a term and issues its report cards. Refusals come back
// AlreadyExists with an X-Error-Code (MISSING_GRADES, TERM_WITHOUT_MONTHS,
// ALREADY_CLOSED, LEVEL_NOT_GRADED).
func (c *GradesClient) ClosePeriod(ctx context.Context, in ClosePeriodInput) (ClosePeriodResult, error) {
	res, err := c.rpc.ClosePeriod(ctx, connect.NewRequest(&gradesv1.ClosePeriodRequest{
		TermId: in.TermID, Force: in.Force, ActorId: in.ActorID, ActorName: in.ActorName,
	}))
	if err != nil {
		return ClosePeriodResult{}, err
	}
	return ClosePeriodResult{Status: res.Msg.GetStatus(), Version: int(res.Msg.GetVersion()), ReportCards: int(res.Msg.GetReportCards())}, nil
}

// ReopenPeriod reopens a closed term (ALREADY_OPEN otherwise).
func (c *GradesClient) ReopenPeriod(ctx context.Context, termID, actorID string) (string, error) {
	res, err := c.rpc.ReopenPeriod(ctx, connect.NewRequest(&gradesv1.ReopenPeriodRequest{TermId: termID, ActorId: actorID}))
	if err != nil {
		return "", err
	}
	return res.Msg.GetStatus(), nil
}

// ListReportCards lists the latest published report card of each matching
// enrollment.
func (c *GradesClient) ListReportCards(ctx context.Context, f ReportCardFilter) ([]ReportCardSummary, error) {
	res, err := c.rpc.ListReportCards(ctx, connect.NewRequest(&gradesv1.ListReportCardsRequest{
		TermId: f.TermID, GroupId: f.GroupID, StudentId: f.StudentID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]ReportCardSummary, 0, len(res.Msg.GetReportCards()))
	for _, s := range res.Msg.GetReportCards() {
		out = append(out, ReportCardSummary{
			ID: s.GetId(), TermID: s.GetTermId(), TermName: s.GetTermName(), StudentID: s.GetStudentId(), StudentName: s.GetStudentName(),
			GroupID: s.GetGroupId(), GroupLabel: s.GetGroupLabel(), Version: int(s.GetVersion()), IssuedAt: timeOrZero(s.GetIssuedAt()),
			GeneralAverage: s.GetGeneralAverage(), EnrollmentID: s.GetEnrollmentId(),
		})
	}
	return out, nil
}

// GetReportCard reads one report card's full snapshot.
func (c *GradesClient) GetReportCard(ctx context.Context, id string) (ReportCard, error) {
	res, err := c.rpc.GetReportCard(ctx, connect.NewRequest(&gradesv1.GetReportCardRequest{Id: id}))
	if err != nil {
		return ReportCard{}, err
	}
	rc := res.Msg.GetReportCard()
	lines := make([]ReportCardLine, 0, len(rc.GetLines()))
	for _, l := range rc.GetLines() {
		lines = append(lines, ReportCardLine{
			SubjectID: l.GetSubjectId(), SubjectName: l.GetSubjectName(), Position: int(l.GetPosition()),
			MonthValues: l.GetMonthValues(), TermAverage: l.GetTermAverage(),
		})
	}
	return ReportCard{
		ID: rc.GetId(), TermID: rc.GetTermId(), CycleID: rc.GetCycleId(), LevelID: rc.GetLevelId(), GroupID: rc.GetGroupId(),
		EnrollmentID: rc.GetEnrollmentId(), StudentID: rc.GetStudentId(), Version: int(rc.GetVersion()), Status: rc.GetStatus(),
		IssuedAt: timeOrZero(rc.GetIssuedAt()), IssuedBy: rc.GetIssuedBy(), StudentName: rc.GetStudentName(),
		EnrollmentNumber: rc.GetEnrollmentNumber(), GroupLabel: rc.GetGroupLabel(), LevelName: rc.GetLevelName(),
		CycleName: rc.GetCycleName(), TermName: rc.GetTermName(), Months: rc.GetMonths(), GeneralAverage: rc.GetGeneralAverage(),
		Scale: toGradeScale(rc.GetScale()), Lines: lines,
	}, nil
}
