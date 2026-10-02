package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// WithActivities wires T17's activities. Without it those RPCs answer
// Unimplemented.
func (h *FinanceHandler) WithActivities(svc *usecase.ActivityService) *FinanceHandler {
	h.activities = svc
	return h
}

var errActivitiesUnwired = connect.NewError(connect.CodeUnimplemented, glerrors.New(glerrors.KindInternal, "activities are not enabled"))

// ListActivities lists the activities with their enrolled counts.
func (h *FinanceHandler) ListActivities(ctx context.Context, req *connect.Request[financev1.ListActivitiesRequest]) (*connect.Response[financev1.ListActivitiesResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	views, err := h.activities.ListActivities(ctx, req.Msg.GetIncludeInactive())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.Activity, len(views))
	for i, v := range views {
		out[i] = toProtoActivity(v.Activity, v.EnrolledCount)
	}
	return connect.NewResponse(&financev1.ListActivitiesResponse{Activities: out}), nil
}

// GetActivity reads one activity.
func (h *FinanceHandler) GetActivity(ctx context.Context, req *connect.Request[financev1.GetActivityRequest]) (*connect.Response[financev1.GetActivityResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	v, err := h.activities.GetActivity(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GetActivityResponse{Activity: toProtoActivity(v.Activity, v.EnrolledCount)}), nil
}

// CreateActivity adds an activity.
func (h *FinanceHandler) CreateActivity(ctx context.Context, req *connect.Request[financev1.CreateActivityRequest]) (*connect.Response[financev1.CreateActivityResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	in, err := fromProtoActivityInput(req.Msg.GetActivity())
	if err != nil {
		return nil, connectErr(err)
	}
	a, err := h.activities.CreateActivity(ctx, in)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateActivityResponse{Activity: toProtoActivity(a, 0)}), nil
}

// UpdateActivity replaces an activity's fields and options.
func (h *FinanceHandler) UpdateActivity(ctx context.Context, req *connect.Request[financev1.UpdateActivityRequest]) (*connect.Response[financev1.UpdateActivityResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	in, err := fromProtoActivityInput(req.Msg.GetActivity())
	if err != nil {
		return nil, connectErr(err)
	}
	if _, err := h.activities.UpdateActivity(ctx, req.Msg.GetId(), in); err != nil {
		return nil, connectErr(err)
	}
	// Read back with the count, as the list shows it.
	v, err := h.activities.GetActivity(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateActivityResponse{Activity: toProtoActivity(v.Activity, v.EnrolledCount)}), nil
}

// ListActivityEnrollments lists who is enrolled today.
func (h *FinanceHandler) ListActivityEnrollments(ctx context.Context, req *connect.Request[financev1.ListActivityEnrollmentsRequest]) (*connect.Response[financev1.ListActivityEnrollmentsResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	rows, err := h.activities.ListEnrollments(ctx, req.Msg.GetActivityId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListActivityEnrollmentsResponse{Enrollments: toProtoEnrollments(rows)}), nil
}

// EnrollInActivity enrols pupils and charges what is already billable.
func (h *FinanceHandler) EnrollInActivity(ctx context.Context, req *connect.Request[financev1.EnrollInActivityRequest]) (*connect.Response[financev1.EnrollInActivityResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	startsOn, err := parseOptionalDay(req.Msg.GetStartsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	items := make([]usecase.EnrollItem, len(req.Msg.GetItems()))
	for i, it := range req.Msg.GetItems() {
		items[i] = usecase.EnrollItem{StudentID: it.GetStudentId(), OptionID: it.GetOptionId()}
	}
	got, err := h.activities.Enroll(ctx, req.Msg.GetActivityId(), startsOn, items)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.EnrollInActivityResponse{
		Enrollments: toProtoEnrollments(got.Enrollments), ChargesCreated: int32(got.ChargesCreated),
	}), nil
}

// UnenrollFromActivity ends enrolments.
func (h *FinanceHandler) UnenrollFromActivity(ctx context.Context, req *connect.Request[financev1.UnenrollFromActivityRequest]) (*connect.Response[financev1.UnenrollFromActivityResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	endsOn, err := parseOptionalDay(req.Msg.GetEndsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	n, err := h.activities.Unenroll(ctx, req.Msg.GetActivityId(), req.Msg.GetStudentIds(), endsOn)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UnenrollFromActivityResponse{Ended: int32(n)}), nil
}

// ChangeActivityOption moves a pupil to another option.
func (h *FinanceHandler) ChangeActivityOption(ctx context.Context, req *connect.Request[financev1.ChangeActivityOptionRequest]) (*connect.Response[financev1.ChangeActivityOptionResponse], error) {
	if h.activities == nil {
		return nil, errActivitiesUnwired
	}
	e, err := h.activities.ChangeOption(ctx, req.Msg.GetActivityId(), req.Msg.GetStudentId(), req.Msg.GetOptionId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ChangeActivityOptionResponse{Enrollment: toProtoEnrollment(e)}), nil
}

// --- mapping ---------------------------------------------------------------

func fromProtoActivityInput(in *financev1.ActivityInput) (usecase.ActivityInput, error) {
	if in == nil {
		return usecase.ActivityInput{}, glerrors.New(glerrors.KindInvalid, "activity is required")
	}
	schedule := make([]domain.ActivitySlot, len(in.GetSchedule()))
	for i, s := range in.GetSchedule() {
		start, err := domain.ParseClock(s.GetStart())
		if err != nil {
			return usecase.ActivityInput{}, err
		}
		end, err := domain.ParseClock(s.GetEnd())
		if err != nil {
			return usecase.ActivityInput{}, err
		}
		schedule[i] = domain.ActivitySlot{Weekday: int(s.GetWeekday()), StartMin: start, EndMin: end}
	}
	startsOn, err := parseDay(in.GetBillingStartsOn())
	if err != nil {
		return usecase.ActivityInput{}, err
	}
	dueOn, err := parseDay(in.GetFirstDueOn())
	if err != nil {
		return usecase.ActivityInput{}, err
	}
	options := make([]domain.PriceOption, len(in.GetOptions()))
	for i, o := range in.GetOptions() {
		options[i] = domain.PriceOption{ID: o.GetId(), DaysPerWeek: int(o.GetDaysPerWeek()), AmountCents: o.GetAmountCents()}
	}
	return usecase.ActivityInput{
		Name: in.GetName(), Schedule: schedule, Periodicity: in.GetPeriodicity(),
		BillingStartsOn: startsOn, FirstDueOn: dueOn, LateFeeCents: in.GetLateFeeCents(),
		Active: in.GetActive(), Options: options,
	}, nil
}

func toProtoActivity(a domain.Activity, enrolled int) *financev1.Activity {
	schedule := make([]*financev1.ActivitySlot, 0, len(a.Schedule()))
	for _, s := range a.Schedule() {
		schedule = append(schedule, &financev1.ActivitySlot{
			Weekday: int32(s.Weekday), Start: domain.FormatClock(s.StartMin), End: domain.FormatClock(s.EndMin),
		})
	}
	options := make([]*financev1.ActivityPriceOption, 0, len(a.Options()))
	for _, o := range a.Options() {
		options = append(options, &financev1.ActivityPriceOption{Id: o.ID, DaysPerWeek: int32(o.DaysPerWeek), AmountCents: o.AmountCents})
	}
	return &financev1.Activity{
		Id: a.ID(), Name: a.Name(), Schedule: schedule, Periodicity: a.Periodicity(),
		BillingStartsOn: a.BillingStartsOn().Format(dayLayout), FirstDueOn: a.FirstDueOn().Format(dayLayout),
		LateFeeCents: a.LateFee().Int64(), Active: a.Active(), Options: options, EnrolledCount: int32(enrolled),
	}
}

func toProtoEnrollments(rows []domain.ActivityEnrollment) []*financev1.ActivityEnrollment {
	out := make([]*financev1.ActivityEnrollment, len(rows))
	for i, e := range rows {
		out[i] = toProtoEnrollment(e)
	}
	return out
}

func toProtoEnrollment(e domain.ActivityEnrollment) *financev1.ActivityEnrollment {
	ends := ""
	if !e.EndsOn().IsZero() {
		ends = e.EndsOn().Format(dayLayout)
	}
	return &financev1.ActivityEnrollment{
		Id: e.ID(), ActivityId: e.ActivityID(), StudentId: e.StudentID(), OptionId: e.OptionID(),
		StartsOn: e.StartsOn().Format(dayLayout), EndsOn: ends,
	}
}
