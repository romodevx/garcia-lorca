package client

import (
	"context"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T17: activities (actividades) and who is enrolled in them. Money is integer
// cents and days are YYYY-MM-DD on the wire, always.

// ActivitySlot is one entry of the weekly schedule: weekday 1 (Monday) … 7.
type ActivitySlot struct {
	Weekday int32  `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// ActivityPriceOption is a charging scheme. DaysPerWeek nil is the flat fee.
type ActivityPriceOption struct {
	ID          string `json:"id,omitempty"`
	DaysPerWeek *int   `json:"daysPerWeek"`
	AmountCents int64  `json:"amountCents"`
}

// ActivityInput is what create and update take.
type ActivityInput struct {
	Name            string                `json:"name"`
	Schedule        []ActivitySlot        `json:"schedule"`
	Periodicity     string                `json:"periodicity"`
	BillingStartsOn string                `json:"billingStartsOn"`
	FirstDueOn      string                `json:"firstDueOn"`
	LateFeeCents    int64                 `json:"lateFeeCents"`
	Active          bool                  `json:"active"`
	Options         []ActivityPriceOption `json:"options"`
}

// Activity is an activity as the panel reads it.
type Activity struct {
	ID string `json:"id"`
	ActivityInput
	EnrolledCount int32 `json:"enrolledCount"`
}

// ActivityEnrollment is a pupil enrolled in an activity. EndsOn nil while
// still enrolled.
type ActivityEnrollment struct {
	ID         string  `json:"id"`
	ActivityID string  `json:"activityId"`
	StudentID  string  `json:"studentId"`
	OptionID   string  `json:"optionId"`
	StartsOn   string  `json:"startsOn"`
	EndsOn     *string `json:"endsOn"`
}

// ActivityEnrollItem is one pupil to enrol, with the option chosen.
type ActivityEnrollItem struct {
	StudentID string `json:"studentId"`
	OptionID  string `json:"optionId"`
}

// ListActivities lists the activities.
func (c *FinanceClient) ListActivities(ctx context.Context, includeInactive bool) ([]Activity, error) {
	res, err := c.rpc.ListActivities(ctx, connect.NewRequest(&financev1.ListActivitiesRequest{IncludeInactive: includeInactive}))
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(res.Msg.GetActivities()))
	for _, a := range res.Msg.GetActivities() {
		out = append(out, toActivity(a))
	}
	return out, nil
}

// GetActivity reads one activity.
func (c *FinanceClient) GetActivity(ctx context.Context, id string) (Activity, error) {
	res, err := c.rpc.GetActivity(ctx, connect.NewRequest(&financev1.GetActivityRequest{Id: id}))
	if err != nil {
		return Activity{}, err
	}
	return toActivity(res.Msg.GetActivity()), nil
}

// CreateActivity adds an activity.
func (c *FinanceClient) CreateActivity(ctx context.Context, in ActivityInput) (Activity, error) {
	res, err := c.rpc.CreateActivity(ctx, connect.NewRequest(&financev1.CreateActivityRequest{Activity: activityInputToProto(in)}))
	if err != nil {
		return Activity{}, err
	}
	return toActivity(res.Msg.GetActivity()), nil
}

// UpdateActivity replaces an activity's fields and options.
func (c *FinanceClient) UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error) {
	res, err := c.rpc.UpdateActivity(ctx, connect.NewRequest(&financev1.UpdateActivityRequest{Id: id, Activity: activityInputToProto(in)}))
	if err != nil {
		return Activity{}, err
	}
	return toActivity(res.Msg.GetActivity()), nil
}

// ListActivityEnrollments lists who is enrolled today.
func (c *FinanceClient) ListActivityEnrollments(ctx context.Context, activityID string) ([]ActivityEnrollment, error) {
	res, err := c.rpc.ListActivityEnrollments(ctx, connect.NewRequest(&financev1.ListActivityEnrollmentsRequest{ActivityId: activityID}))
	if err != nil {
		return nil, err
	}
	return toActivityEnrollments(res.Msg.GetEnrollments()), nil
}

// EnrollInActivity enrols pupils and reports the charges issued at once.
func (c *FinanceClient) EnrollInActivity(ctx context.Context, activityID, startsOn string, items []ActivityEnrollItem) ([]ActivityEnrollment, int32, error) {
	wire := make([]*financev1.EnrollItem, len(items))
	for i, it := range items {
		wire[i] = &financev1.EnrollItem{StudentId: it.StudentID, OptionId: it.OptionID}
	}
	res, err := c.rpc.EnrollInActivity(ctx, connect.NewRequest(&financev1.EnrollInActivityRequest{
		ActivityId: activityID, StartsOn: startsOn, Items: wire,
	}))
	if err != nil {
		return nil, 0, err
	}
	return toActivityEnrollments(res.Msg.GetEnrollments()), res.Msg.GetChargesCreated(), nil
}

// UnenrollFromActivity ends enrolments and reports how many.
func (c *FinanceClient) UnenrollFromActivity(ctx context.Context, activityID string, studentIDs []string, endsOn string) (int32, error) {
	res, err := c.rpc.UnenrollFromActivity(ctx, connect.NewRequest(&financev1.UnenrollFromActivityRequest{
		ActivityId: activityID, StudentIds: studentIDs, EndsOn: endsOn,
	}))
	if err != nil {
		return 0, err
	}
	return res.Msg.GetEnded(), nil
}

// ChangeActivityOption moves an enrolled pupil to another option.
func (c *FinanceClient) ChangeActivityOption(ctx context.Context, activityID, studentID, optionID string) (ActivityEnrollment, error) {
	res, err := c.rpc.ChangeActivityOption(ctx, connect.NewRequest(&financev1.ChangeActivityOptionRequest{
		ActivityId: activityID, StudentId: studentID, OptionId: optionID,
	}))
	if err != nil {
		return ActivityEnrollment{}, err
	}
	return toActivityEnrollment(res.Msg.GetEnrollment()), nil
}

// --- mapping ---------------------------------------------------------------

func activityInputToProto(in ActivityInput) *financev1.ActivityInput {
	schedule := make([]*financev1.ActivitySlot, len(in.Schedule))
	for i, s := range in.Schedule {
		schedule[i] = &financev1.ActivitySlot{Weekday: s.Weekday, Start: s.Start, End: s.End}
	}
	options := make([]*financev1.ActivityPriceOption, len(in.Options))
	for i, o := range in.Options {
		days := int32(0)
		if o.DaysPerWeek != nil {
			days = int32(*o.DaysPerWeek)
		}
		options[i] = &financev1.ActivityPriceOption{Id: o.ID, DaysPerWeek: days, AmountCents: o.AmountCents}
	}
	return &financev1.ActivityInput{
		Name: in.Name, Schedule: schedule, Periodicity: in.Periodicity,
		BillingStartsOn: in.BillingStartsOn, FirstDueOn: in.FirstDueOn,
		LateFeeCents: in.LateFeeCents, Active: in.Active, Options: options,
	}
}

func toActivity(a *financev1.Activity) Activity {
	schedule := make([]ActivitySlot, 0, len(a.GetSchedule()))
	for _, s := range a.GetSchedule() {
		schedule = append(schedule, ActivitySlot{Weekday: s.GetWeekday(), Start: s.GetStart(), End: s.GetEnd()})
	}
	options := make([]ActivityPriceOption, 0, len(a.GetOptions()))
	for _, o := range a.GetOptions() {
		opt := ActivityPriceOption{ID: o.GetId(), AmountCents: o.GetAmountCents()}
		if d := int(o.GetDaysPerWeek()); d > 0 {
			opt.DaysPerWeek = &d
		}
		options = append(options, opt)
	}
	return Activity{
		ID: a.GetId(),
		ActivityInput: ActivityInput{
			Name: a.GetName(), Schedule: schedule, Periodicity: a.GetPeriodicity(),
			BillingStartsOn: a.GetBillingStartsOn(), FirstDueOn: a.GetFirstDueOn(),
			LateFeeCents: a.GetLateFeeCents(), Active: a.GetActive(), Options: options,
		},
		EnrolledCount: a.GetEnrolledCount(),
	}
}

func toActivityEnrollments(rows []*financev1.ActivityEnrollment) []ActivityEnrollment {
	out := make([]ActivityEnrollment, 0, len(rows))
	for _, e := range rows {
		out = append(out, toActivityEnrollment(e))
	}
	return out
}

func toActivityEnrollment(e *financev1.ActivityEnrollment) ActivityEnrollment {
	out := ActivityEnrollment{
		ID: e.GetId(), ActivityID: e.GetActivityId(), StudentID: e.GetStudentId(),
		OptionID: e.GetOptionId(), StartsOn: e.GetStartsOn(),
	}
	if ends := e.GetEndsOn(); ends != "" {
		out.EndsOn = &ends
	}
	return out
}
