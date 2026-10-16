package grpcadapter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The activity handlers translate only; these drive a real ActivityService over
// a tiny in-memory repository and read the wire, both directions (checklist #2).

type memActivities struct {
	activities  map[string]domain.Activity
	enrollments []domain.ActivityEnrollment
}

func (m *memActivities) CreateActivity(_ context.Context, a domain.Activity) (domain.Activity, error) {
	m.activities[a.ID()] = a
	return a, nil
}

func (m *memActivities) UpdateActivity(_ context.Context, a domain.Activity, _ time.Time) (domain.Activity, error) {
	m.activities[a.ID()] = a
	return a, nil
}

func (m *memActivities) GetActivity(_ context.Context, id string) (domain.Activity, error) {
	a, ok := m.activities[id]
	if !ok {
		return domain.Activity{}, glerrors.ErrNotFound
	}
	return a, nil
}

func (m *memActivities) ListActivities(context.Context, bool) ([]domain.Activity, error) {
	out := make([]domain.Activity, 0, len(m.activities))
	for _, a := range m.activities {
		out = append(out, a)
	}
	return out, nil
}

func (m *memActivities) CountActiveEnrollments(context.Context, time.Time) (map[string]int, error) {
	out := map[string]int{}
	for _, e := range m.enrollments {
		out[e.ActivityID()]++
	}
	return out, nil
}

func (m *memActivities) ListActivityEnrollments(context.Context, string) ([]domain.ActivityEnrollment, error) {
	return m.enrollments, nil
}

func (m *memActivities) ListBillableEnrollments(context.Context, time.Time) ([]domain.ActivityEnrollment, error) {
	return m.enrollments, nil
}

func (m *memActivities) CreateEnrollments(_ context.Context, es []domain.ActivityEnrollment) ([]domain.ActivityEnrollment, error) {
	m.enrollments = append(m.enrollments, es...)
	return es, nil
}

func (m *memActivities) EndEnrollments(_ context.Context, _ string, ids []string, _ time.Time) (int, error) {
	return len(ids), nil
}

func (m *memActivities) ChangeEnrollmentOption(_ context.Context, activityID, studentID, optionID string, _ time.Time) (domain.ActivityEnrollment, error) {
	return domain.RestoreActivityEnrollment("e-x", activityID, studentID, optionID, fixedNow, time.Time{})
}

var _ ports.ActivityRepository = (*memActivities)(nil)

type knownStudents struct{}

func (knownStudents) ByID(_ context.Context, id string) (ports.StudentRef, error) {
	return ports.StudentRef{ID: id, Status: "active"}, nil
}

type countingBiller struct{ n int }

func (b countingBiller) BillActivityEnrollments(context.Context, domain.Activity, []domain.ActivityEnrollment) (int, error) {
	return b.n, nil
}

func activityHandler() *grpcadapter.FinanceHandler {
	n := 0
	ids := func() string { n++; return fmt.Sprintf("id-%d", n) }
	svc := usecase.NewActivityService(&memActivities{activities: map[string]domain.Activity{}}, knownStudents{},
		countingBiller{n: 2}, ids, usecase.NewSchoolClock(func() time.Time { return fixedNow }, schoolZone))
	return grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance")).WithActivities(svc)
}

func swimWire() *financev1.ActivityInput {
	return &financev1.ActivityInput{
		Name: "Natación",
		Schedule: []*financev1.ActivitySlot{
			{Weekday: 2, Start: "15:00", End: "16:00"},
			{Weekday: 4, Start: "16:00", End: "17:30"},
		},
		Periodicity: "monthly", BillingStartsOn: "2026-09-01", FirstDueOn: "2026-09-10",
		LateFeeCents: 5000, Active: true,
		Options: []*financev1.ActivityPriceOption{
			{DaysPerWeek: 1, AmountCents: 40000},
			{DaysPerWeek: 0, AmountCents: 90000},
		},
	}
}

func TestActivityRoundTripsTheWire(t *testing.T) {
	h := activityHandler()
	ctx := context.Background()
	created, err := h.CreateActivity(ctx, connect.NewRequest(&financev1.CreateActivityRequest{Activity: swimWire()}))
	if err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}
	a := created.Msg.GetActivity()
	if a.GetId() == "" || a.GetName() != "Natación" || a.GetPeriodicity() != "monthly" ||
		a.GetBillingStartsOn() != "2026-09-01" || a.GetFirstDueOn() != "2026-09-10" ||
		a.GetLateFeeCents() != 5000 || !a.GetActive() {
		t.Fatalf("activity on the wire = %+v", a)
	}
	if len(a.GetSchedule()) != 2 || a.GetSchedule()[1].GetEnd() != "17:30" || a.GetSchedule()[1].GetWeekday() != 4 {
		t.Fatalf("schedule = %+v", a.GetSchedule())
	}
	if len(a.GetOptions()) != 2 || a.GetOptions()[0].GetDaysPerWeek() != 1 || a.GetOptions()[1].GetDaysPerWeek() != 0 ||
		a.GetOptions()[1].GetAmountCents() != 90000 || a.GetOptions()[0].GetId() == "" {
		t.Fatalf("options = %+v", a.GetOptions())
	}

	// And back: the options' ids go in, the update keeps them.
	in := swimWire()
	in.Options = a.GetOptions()
	in.Options[0].AmountCents = 45000
	updated, err := h.UpdateActivity(ctx, connect.NewRequest(&financev1.UpdateActivityRequest{Id: a.GetId(), Activity: in}))
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if got := updated.Msg.GetActivity().GetOptions()[0]; got.GetId() != a.GetOptions()[0].GetId() || got.GetAmountCents() != 45000 {
		t.Fatalf("updated option = %+v", got)
	}

	enrolled, err := h.EnrollInActivity(ctx, connect.NewRequest(&financev1.EnrollInActivityRequest{
		ActivityId: a.GetId(), StartsOn: "2026-09-02",
		Items: []*financev1.EnrollItem{{StudentId: "stu-1", OptionId: a.GetOptions()[0].GetId()}},
	}))
	if err != nil {
		t.Fatalf("EnrollInActivity: %v", err)
	}
	e := enrolled.Msg.GetEnrollments()[0]
	if enrolled.Msg.GetChargesCreated() != 2 || e.GetStartsOn() != "2026-09-02" || e.GetEndsOn() != "" || e.GetStudentId() != "stu-1" {
		t.Fatalf("enrolment on the wire = %+v (%d charges)", e, enrolled.Msg.GetChargesCreated())
	}

	listed, err := h.ListActivities(ctx, connect.NewRequest(&financev1.ListActivitiesRequest{}))
	if err != nil || len(listed.Msg.GetActivities()) != 1 || listed.Msg.GetActivities()[0].GetEnrolledCount() != 1 {
		t.Fatalf("ListActivities = %+v, %v", listed, err)
	}
	ended, err := h.UnenrollFromActivity(ctx, connect.NewRequest(&financev1.UnenrollFromActivityRequest{
		ActivityId: a.GetId(), StudentIds: []string{"stu-1"}, EndsOn: "2026-10-01",
	}))
	if err != nil || ended.Msg.GetEnded() != 1 {
		t.Fatalf("Unenroll = %+v, %v", ended, err)
	}
}

func TestActivityHandlerRefusesBadInput(t *testing.T) {
	h := activityHandler()
	ctx := context.Background()
	bad := swimWire()
	bad.Schedule[0].Start = "3pm"
	if _, err := h.CreateActivity(ctx, connect.NewRequest(&financev1.CreateActivityRequest{Activity: bad})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad clock: %v", err)
	}
	bad = swimWire()
	bad.FirstDueOn = "10/09/2026"
	if _, err := h.CreateActivity(ctx, connect.NewRequest(&financev1.CreateActivityRequest{Activity: bad})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad date: %v", err)
	}
	if _, err := h.CreateActivity(ctx, connect.NewRequest(&financev1.CreateActivityRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing activity: %v", err)
	}
	if _, err := h.GetActivity(ctx, connect.NewRequest(&financev1.GetActivityRequest{Id: "nope"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown activity: %v", err)
	}
}

func TestActivityRPCsAreUnimplementedWhenUnwired(t *testing.T) {
	h := grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance"))
	if _, err := h.ListActivities(context.Background(), connect.NewRequest(&financev1.ListActivitiesRequest{})); connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("unwired: %v", err)
	}
}
