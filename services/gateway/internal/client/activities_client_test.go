package client

import (
	"encoding/json"
	"strings"
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T17, both directions of the wire (checklist #2).
func TestActivityCrossesTheWireBothWays(t *testing.T) {
	two := 2
	in := ActivityInput{
		Name:        "Natación",
		Schedule:    []ActivitySlot{{Weekday: 2, Start: "15:00", End: "16:30"}},
		Periodicity: "weekly", BillingStartsOn: "2026-10-05", FirstDueOn: "2026-10-07",
		LateFeeCents: 5000, Active: true,
		Options: []ActivityPriceOption{{ID: "o-1", DaysPerWeek: &two, AmountCents: 70000}, {AmountCents: 90000}},
	}
	p := activityInputToProto(in)
	if p.GetName() != "Natación" || p.GetPeriodicity() != "weekly" || p.GetBillingStartsOn() != "2026-10-05" ||
		p.GetFirstDueOn() != "2026-10-07" || p.GetLateFeeCents() != 5000 || !p.GetActive() ||
		len(p.GetSchedule()) != 1 || p.GetSchedule()[0].GetEnd() != "16:30" || p.GetSchedule()[0].GetWeekday() != 2 {
		t.Fatalf("input = %+v", p)
	}
	if o := p.GetOptions(); len(o) != 2 || o[0].GetId() != "o-1" || o[0].GetDaysPerWeek() != 2 || o[1].GetDaysPerWeek() != 0 || o[1].GetAmountCents() != 90000 {
		t.Fatalf("options = %+v", o)
	}

	got := toActivity(&financev1.Activity{
		Id: "a-1", Name: "Natación", Periodicity: "monthly", BillingStartsOn: "2026-09-01", FirstDueOn: "2026-09-10",
		LateFeeCents: 5000, Active: true, EnrolledCount: 12,
		Schedule: []*financev1.ActivitySlot{{Weekday: 4, Start: "16:00", End: "17:00"}},
		Options:  []*financev1.ActivityPriceOption{{Id: "o-1", DaysPerWeek: 1, AmountCents: 40000}, {Id: "o-f", AmountCents: 90000}},
	})
	if got.ID != "a-1" || got.EnrolledCount != 12 || got.BillingStartsOn != "2026-09-01" || got.FirstDueOn != "2026-09-10" ||
		got.LateFeeCents != 5000 || !got.Active || got.Schedule[0].Start != "16:00" || got.Periodicity != "monthly" {
		t.Fatalf("activity = %+v", got)
	}
	if got.Options[0].DaysPerWeek == nil || *got.Options[0].DaysPerWeek != 1 || got.Options[1].DaysPerWeek != nil {
		t.Fatalf("options = %+v", got.Options)
	}
	raw, _ := json.Marshal(got.Options[1])
	if !strings.Contains(string(raw), `"daysPerWeek":null`) {
		t.Fatalf("the flat fee must be daysPerWeek null on the wire: %s", raw)
	}

	e := toActivityEnrollment(&financev1.ActivityEnrollment{Id: "e", ActivityId: "a", StudentId: "s", OptionId: "o", StartsOn: "2026-09-02"})
	raw, _ = json.Marshal(e)
	if e.StartsOn != "2026-09-02" || e.EndsOn != nil || !strings.Contains(string(raw), `"endsOn":null`) {
		t.Fatalf("open enrolment = %s", raw)
	}
	ended := toActivityEnrollment(&financev1.ActivityEnrollment{EndsOn: "2026-10-01"})
	if ended.EndsOn == nil || *ended.EndsOn != "2026-10-01" {
		t.Fatalf("ended = %+v", ended)
	}
	if toActivity(&financev1.Activity{}).Schedule == nil || toActivity(&financev1.Activity{}).Options == nil {
		t.Fatal("lists must be [] on the wire, never null")
	}
}

func TestABillingRunReportsTheActivitiesItIssued(t *testing.T) {
	if got := toBillingRun(&financev1.RunBillingResponse{ActivitiesCreated: 5}); got.ActivitiesCreated != 5 {
		t.Fatalf("activities created = %d, want 5", got.ActivitiesCreated)
	}
}
