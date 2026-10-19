package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// PeriodsUseCase is the period closing and report card surface this handler
// consumes (T7). *usecase.PeriodsService satisfies it.
type PeriodsUseCase interface {
	ListPeriods(ctx context.Context, cycleID string) ([]usecase.Period, error)
	Progress(ctx context.Context, termID string) (usecase.PeriodProgress, error)
	Close(ctx context.Context, termID string, force bool, actor domain.Actor) (usecase.CloseResult, error)
	Reopen(ctx context.Context, termID string, actor domain.Actor, reason string) (domain.PeriodStatus, error)
	ReportCards(ctx context.Context, filter ports.ReportCardFilter) ([]domain.ReportCard, error)
	ReportCard(ctx context.Context, id string) (domain.ReportCard, error)
	ClosedTerms(ctx context.Context, termIDs []string) ([]string, error)
}

// ClosedTerms answers which of the requested terms are closed now.
func (h *GradesHandler) ClosedTerms(ctx context.Context, req *connect.Request[gradesv1.ClosedTermsRequest]) (*connect.Response[gradesv1.ClosedTermsResponse], error) {
	closed, err := h.periods.ClosedTerms(ctx, req.Msg.GetTermIds())
	if err != nil {
		return nil, connectErr(err)
	}
	if closed == nil {
		closed = []string{}
	}
	return connect.NewResponse(&gradesv1.ClosedTermsResponse{ClosedTermIds: closed}), nil
}

// ListPeriods returns a cycle's terms with their closure and progress.
func (h *GradesHandler) ListPeriods(ctx context.Context, req *connect.Request[gradesv1.ListPeriodsRequest]) (*connect.Response[gradesv1.ListPeriodsResponse], error) {
	periods, err := h.periods.ListPeriods(ctx, req.Msg.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*gradesv1.Period, 0, len(periods))
	for _, p := range periods {
		c := p.Closure
		out = append(out, &gradesv1.Period{
			TermId: p.Term.ID, TermName: p.Term.Name, Ordinal: int32(p.Term.Ordinal),
			LevelId: p.Term.LevelID, LevelName: p.LevelName, StartsOn: dayText(p.StartsOn), EndsOn: dayText(p.EndsOn),
			Status: string(c.Status()), ClosedAt: timestampOrNil(c.ClosedAt()), ClosedByName: c.ClosedByName(),
			Version:  int32(c.Version()),
			Progress: &gradesv1.PeriodProgress{Captured: int32(p.Captured), Expected: int32(p.Expected)},
		})
	}
	return connect.NewResponse(&gradesv1.ListPeriodsResponse{Periods: out}), nil
}

// GetPeriodProgress returns one term's progress and what is missing.
func (h *GradesHandler) GetPeriodProgress(ctx context.Context, req *connect.Request[gradesv1.GetPeriodProgressRequest]) (*connect.Response[gradesv1.GetPeriodProgressResponse], error) {
	p, err := h.periods.Progress(ctx, req.Msg.GetTermId())
	if err != nil {
		return nil, connectErr(err)
	}
	missing := make([]*gradesv1.MissingGrades, 0, len(p.Missing))
	for _, m := range p.Missing {
		missing = append(missing, &gradesv1.MissingGrades{
			GroupId: m.GroupID, GroupLabel: m.GroupLabel, SubjectId: m.SubjectID, SubjectName: m.SubjectName,
			TeacherName: m.TeacherName, MissingCount: int32(m.Missing),
		})
	}
	return connect.NewResponse(&gradesv1.GetPeriodProgressResponse{
		TermId: p.TermID, Unfinished: p.Unfinished, EndsOn: dayText(p.EndsOn),
		GroupsWithoutPeriod: nonNil(p.GroupsWithoutPeriod),
		Expected:            int32(p.Expected), Captured: int32(p.Captured), Missing: missing,
		ReportCards: int32(p.ReportCards), SkippedWithoutGrades: int32(p.SkippedWithoutGrades),
		WithdrawnReportCards: int32(p.WithdrawnReportCards),
	}), nil
}

// ClosePeriod closes a term and issues its report cards.
func (h *GradesHandler) ClosePeriod(ctx context.Context, req *connect.Request[gradesv1.ClosePeriodRequest]) (*connect.Response[gradesv1.ClosePeriodResponse], error) {
	m := req.Msg
	res, err := h.periods.Close(ctx, m.GetTermId(), m.GetForce(), domain.Actor{ID: m.GetActorId(), Name: m.GetActorName()})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.ClosePeriodResponse{
		Status: string(res.Status), Version: int32(res.Version), ReportCards: int32(res.ReportCards),
		SkippedWithoutGrades: int32(res.SkippedWithoutGrades), WithdrawnReportCards: int32(res.WithdrawnReportCards),
	}), nil
}

// ReopenPeriod reopens a closed term.
func (h *GradesHandler) ReopenPeriod(ctx context.Context, req *connect.Request[gradesv1.ReopenPeriodRequest]) (*connect.Response[gradesv1.ReopenPeriodResponse], error) {
	m := req.Msg
	status, err := h.periods.Reopen(ctx, m.GetTermId(), domain.Actor{ID: m.GetActorId(), Name: m.GetActorName()}, m.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.ReopenPeriodResponse{Status: string(status)}), nil
}

// ListReportCards returns the latest published report card of each matching
// enrollment.
func (h *GradesHandler) ListReportCards(ctx context.Context, req *connect.Request[gradesv1.ListReportCardsRequest]) (*connect.Response[gradesv1.ListReportCardsResponse], error) {
	m := req.Msg
	cards, err := h.periods.ReportCards(ctx, ports.ReportCardFilter{TermID: m.GetTermId(), GroupID: m.GetGroupId(), StudentID: m.GetStudentId()})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*gradesv1.ReportCardSummary, 0, len(cards))
	for _, rc := range cards {
		hd := rc.Header()
		out = append(out, &gradesv1.ReportCardSummary{
			Id: hd.ID, TermId: hd.TermID, TermName: hd.TermName, StudentId: hd.StudentID, StudentName: hd.StudentName,
			GroupId: hd.GroupID, GroupLabel: hd.GroupLabel, Version: int32(hd.Version), IssuedAt: timestampOrNil(hd.IssuedAt),
			GeneralAverage: rc.GeneralAverage().String(), EnrollmentId: hd.EnrollmentID,
			TermOrdinal: int32(hd.TermOrdinal), CycleId: hd.CycleID, CycleName: hd.CycleName,
		})
	}
	return connect.NewResponse(&gradesv1.ListReportCardsResponse{ReportCards: out}), nil
}

// GetReportCard returns one report card's full snapshot.
func (h *GradesHandler) GetReportCard(ctx context.Context, req *connect.Request[gradesv1.GetReportCardRequest]) (*connect.Response[gradesv1.GetReportCardResponse], error) {
	rc, err := h.periods.ReportCard(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.GetReportCardResponse{ReportCard: toProtoReportCard(rc)}), nil
}

func toProtoReportCard(rc domain.ReportCard) *gradesv1.ReportCard {
	hd := rc.Header()
	lines := make([]*gradesv1.ReportCardLine, 0, len(rc.Lines()))
	for _, l := range rc.Lines() {
		lines = append(lines, &gradesv1.ReportCardLine{
			SubjectId: l.SubjectID, SubjectName: l.SubjectName, Position: int32(l.Position),
			MonthValues: l.MonthValueStrings(), PeriodValues: l.PeriodValueStrings(), TermAverage: l.Average.String(),
		})
	}
	periods := make([]*gradesv1.ReportCardPeriod, 0, len(hd.Periods))
	for _, p := range hd.Periods {
		periods = append(periods, &gradesv1.ReportCardPeriod{Id: p.ID, Ordinal: int32(p.Ordinal), Name: p.Name})
	}
	return &gradesv1.ReportCard{
		Id: hd.ID, TermId: hd.TermID, CycleId: hd.CycleID, LevelId: hd.LevelID, GroupId: hd.GroupID,
		EnrollmentId: hd.EnrollmentID, StudentId: hd.StudentID, Version: int32(hd.Version), Status: string(rc.Status()),
		IssuedAt: timestampOrNil(hd.IssuedAt), IssuedBy: hd.IssuedBy, StudentName: hd.StudentName,
		EnrollmentNumber: hd.EnrollmentNumber, GroupLabel: hd.GroupLabel, LevelName: hd.LevelName,
		CycleName: hd.CycleName, TermName: hd.TermName, TermOrdinal: int32(hd.TermOrdinal), Months: monthStrings(hd.Months),
		GeneralAverage: rc.GeneralAverage().String(), Scale: toProtoScale(hd.Scale), Lines: lines, Periods: periods,
	}
}

func nonNil(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}
