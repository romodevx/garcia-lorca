package grpcadapter_test

import (
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// seedCycleID gives the calendar a school year to hang from.
func seedCycleID(t *testing.T, h handlerUnderTest) string {
	t.Helper()
	res, err := h.CreateCycle(ctx(), connect.NewRequest(&configv1.CreateCycleRequest{
		Name: "2026-2027", StartsOn: tsOf(someDate), EndsOn: tsOf(laterDate),
	}))
	if err != nil {
		t.Fatalf("seed cycle: %v", err)
	}
	return res.Msg.GetCycle().GetId()
}

// A holiday goes over the wire as a plain YYYY-MM-DD and has to come back as the
// same day. A timezone shifting it by one is not a rounding error here: it moves
// the day a due date lands on.
func TestNonWorkingPeriodRoundTripsAsAPlainDate(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycleID(t, h)

	res, err := h.CreateNonWorkingPeriod(ctx(), connect.NewRequest(&configv1.CreateNonWorkingPeriodRequest{
		CycleId: cycleID, Label: "Independencia", Kind: domain.KindHoliday,
		StartsOn: "2026-09-16", EndsOn: "2026-09-16",
	}))
	if err != nil {
		t.Fatalf("CreateNonWorkingPeriod: %v", err)
	}
	p := res.Msg.GetPeriod()
	if p.GetStartsOn() != "2026-09-16" || p.GetEndsOn() != "2026-09-16" {
		t.Errorf("dates round-trip wrong: %s..%s", p.GetStartsOn(), p.GetEndsOn())
	}

	list, err := h.ListNonWorkingPeriods(ctx(), connect.NewRequest(&configv1.ListNonWorkingPeriodsRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods: %v", err)
	}
	if len(list.Msg.GetPeriods()) != 1 {
		t.Fatalf("listed %d periods, want 1", len(list.Msg.GetPeriods()))
	}

	if _, err := h.DeleteNonWorkingPeriod(ctx(), connect.NewRequest(&configv1.DeleteNonWorkingPeriodRequest{
		Id: p.GetId(),
	})); err != nil {
		t.Fatalf("DeleteNonWorkingPeriod: %v", err)
	}
	list, err = h.ListNonWorkingPeriods(ctx(), connect.NewRequest(&configv1.ListNonWorkingPeriodsRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods after delete: %v", err)
	}
	if len(list.Msg.GetPeriods()) != 0 {
		t.Errorf("listed %d periods after deleting the only one", len(list.Msg.GetPeriods()))
	}
}

// A malformed date is the caller's mistake and must be said so, not accepted as
// a zero date that would silently mean year one.
func TestMalformedDateIsInvalidArgument(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycleID(t, h)

	_, err := h.CreateNonWorkingPeriod(ctx(), connect.NewRequest(&configv1.CreateNonWorkingPeriodRequest{
		CycleId: cycleID, Label: "Mal escrito", Kind: domain.KindHoliday,
		StartsOn: "16/09/2026", EndsOn: "16/09/2026",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("malformed date: want InvalidArgument, got %v", err)
	}
}
