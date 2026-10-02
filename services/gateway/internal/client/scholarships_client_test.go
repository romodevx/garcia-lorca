package client

import (
	"encoding/json"
	"strings"
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T18, both directions of the wire (checklist #2).
func TestScholarshipCrossesTheWireBothWays(t *testing.T) {
	in := ScholarshipInput{Name: "Hermanos", Kind: "percent", Value: 5000, Targets: []string{"tuition", "enrollment"}, Active: true}
	p := scholarshipInputToProto(in)
	if p.GetName() != "Hermanos" || p.GetKind() != "percent" || p.GetValue() != 5000 || len(p.GetTargets()) != 2 || !p.GetActive() {
		t.Fatalf("input = %+v", p)
	}
	got := toScholarship(&financev1.Scholarship{
		Id: "s-1", Name: "Hermanos", Kind: "fixed_price", Value: 100000, Targets: []string{"tuition"}, Active: true, AssignedCount: 3,
	})
	if got.ID != "s-1" || got.Kind != "fixed_price" || got.Value != 100000 || got.AssignedCount != 3 || got.Targets[0] != "tuition" || !got.Active {
		t.Fatalf("scholarship = %+v", got)
	}
	if empty := toScholarship(&financev1.Scholarship{}); empty.Targets == nil {
		t.Fatal("targets must be [] on the wire, never null")
	}
	h := toScholarshipHolders([]*financev1.ScholarshipHolder{{StudentId: "st", StartsOn: "2026-10-02"}})
	raw, _ := json.Marshal(h[0])
	if !strings.Contains(string(raw), `"overlaps":[]`) || !strings.Contains(string(raw), `"startsOn":"2026-10-02"`) {
		t.Fatalf("holder = %s", raw)
	}
	h = toScholarshipHolders([]*financev1.ScholarshipHolder{{StudentId: "st", Overlaps: []*financev1.ScholarshipRef{{Id: "o", Name: "Otra"}}}})
	if h[0].Overlaps[0].ScholarshipID != "o" || h[0].Overlaps[0].Name != "Otra" {
		t.Fatalf("overlaps = %+v", h[0].Overlaps)
	}
}

func TestScholarshipPartsReachTheEdge(t *testing.T) {
	if c := toCharge(&financev1.Charge{ScholarshipCents: 150000}); c.ScholarshipCents != 150000 {
		t.Fatalf("charge = %+v", c)
	}
	if l := toExpectedLines([]*financev1.ExpectedLine{{ScholarshipCents: 150000}}); l[0].ScholarshipCents != 150000 {
		t.Fatalf("line = %+v", l[0])
	}
}
