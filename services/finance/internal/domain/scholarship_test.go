package domain_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

const (
	conceptUUID  = "7d1c2a4e-5b6f-4c3d-8e9f-0a1b2c3d4e5f"
	activityUUID = "0a1b2c3d-4e5f-4c3d-8e9f-7d1c2a4e5b6f"
)

func scholarship(t *testing.T, id, kind string, value int64, targets ...string) domain.Scholarship {
	t.Helper()
	s, err := domain.NewScholarship(domain.ScholarshipSpec{
		ID: id, TenantID: "t1", Name: "Beca " + id, Kind: kind, Value: value, Targets: targets, Active: true,
	})
	if err != nil {
		t.Fatalf("scholarship %s: %v", id, err)
	}
	return s
}

func TestNewScholarshipValidates(t *testing.T) {
	valid := domain.ScholarshipSpec{
		ID: "s1", TenantID: "t1", Name: "  Beca hermanos ", Kind: domain.ScholarshipPercent, Value: 5000,
		Targets: []string{"tuition", "enrollment", "concept:" + conceptUUID, "activity:" + activityUUID}, Active: true,
	}
	s, err := domain.NewScholarship(valid)
	if err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	if s.Name() != "Beca hermanos" || len(s.Targets()) != 4 || !s.Active() {
		t.Fatalf("unexpected scholarship %+v", s)
	}

	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("concept:%08x-5b6f-4c3d-8e9f-0a1b2c3d4e5f", i)
	}
	cases := map[string]func(*domain.ScholarshipSpec){
		"no id":                 func(s *domain.ScholarshipSpec) { s.ID = "" },
		"no tenant":             func(s *domain.ScholarshipSpec) { s.TenantID = "" },
		"no name":               func(s *domain.ScholarshipSpec) { s.Name = "  " },
		"name too long":         func(s *domain.ScholarshipSpec) { s.Name = strings.Repeat("a", 121) },
		"unknown kind":          func(s *domain.ScholarshipSpec) { s.Kind = "gift" },
		"percent zero":          func(s *domain.ScholarshipSpec) { s.Value = 0 },
		"percent over 100":      func(s *domain.ScholarshipSpec) { s.Value = 10001 },
		"fixed off zero":        func(s *domain.ScholarshipSpec) { s.Kind, s.Value = domain.ScholarshipFixedOff, 0 },
		"fixed price negative":  func(s *domain.ScholarshipSpec) { s.Kind, s.Value = domain.ScholarshipFixedPrice, -1 },
		"amount over max":       func(s *domain.ScholarshipSpec) { s.Kind, s.Value = domain.ScholarshipFixedOff, domain.MaxCents+1 },
		"no targets":            func(s *domain.ScholarshipSpec) { s.Targets = nil },
		"unknown target":        func(s *domain.ScholarshipSpec) { s.Targets = []string{"uniforms"} },
		"concept without id":    func(s *domain.ScholarshipSpec) { s.Targets = []string{"concept:"} },
		"concept id with space": func(s *domain.ScholarshipSpec) { s.Targets = []string{"concept:a b"} },
		"activity without uuid": func(s *domain.ScholarshipSpec) { s.Targets = []string{"activity:"} },
		"duplicate target":      func(s *domain.ScholarshipSpec) { s.Targets = []string{"tuition", "tuition"} },
		"too many targets":      func(s *domain.ScholarshipSpec) { s.Targets = tooMany },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := valid
			spec.Targets = append([]string(nil), valid.Targets...)
			mutate(&spec)
			if _, err := domain.NewScholarship(spec); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v", err)
			}
		})
	}

	free, err := domain.NewScholarship(domain.ScholarshipSpec{
		ID: "s2", TenantID: "t1", Name: "Gratis", Kind: domain.ScholarshipFixedPrice, Value: 0, Targets: []string{"tuition"},
	})
	if err != nil || free.Value() != 0 {
		t.Fatalf("a fixed price of zero is a full scholarship: %v", err)
	}
}

func TestScholarshipDiscountOn(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		value int64
		base  int64
		want  int64
	}{
		{"percent half", domain.ScholarshipPercent, 5000, 300000, 150000},
		{"percent full", domain.ScholarshipPercent, 10000, 300000, 300000},
		{"percent rounds half up", domain.ScholarshipPercent, 5000, 333, 167},
		{"percent of zero", domain.ScholarshipPercent, 5000, 0, 0},
		{"percent near the cap does not overflow", domain.ScholarshipPercent, 3333, domain.MaxCents, 333300000000000},
		{"fixed off", domain.ScholarshipFixedOff, 50000, 300000, 50000},
		{"fixed off capped at the price", domain.ScholarshipFixedOff, 400000, 300000, 300000},
		{"fixed price", domain.ScholarshipFixedPrice, 100000, 300000, 200000},
		{"fixed price zero is free", domain.ScholarshipFixedPrice, 0, 300000, 300000},
		{"fixed price above the price never raises it", domain.ScholarshipFixedPrice, 400000, 300000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := scholarship(t, "s", c.kind, c.value, "tuition")
			if got := s.DiscountOn(domain.Cents(c.base)).Int64(); got != c.want {
				t.Fatalf("DiscountOn(%d) = %d, want %d", c.base, got, c.want)
			}
		})
	}
}

func TestScholarshipSharesTarget(t *testing.T) {
	a := scholarship(t, "a", domain.ScholarshipPercent, 5000, "tuition", "enrollment")
	b := scholarship(t, "b", domain.ScholarshipFixedOff, 100, "enrollment")
	c := scholarship(t, "c", domain.ScholarshipFixedOff, 100, "activity:"+activityUUID)
	if !a.SharesTargetWith(b) || a.SharesTargetWith(c) {
		t.Fatal("overlap is a shared target")
	}
}

// tuitionCharge is a $3,000 month due on 10 Sep 2026.
func tuitionCharge(t *testing.T) domain.Charge {
	t.Helper()
	c, err := domain.NewCharge("ch-1", "t1", "st-1", "cy-1", "co-tuition", "Colegiatura septiembre", 300000, 0, day(2026, 9, 10))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestApplyScholarship(t *testing.T) {
	half := scholarship(t, "half", domain.ScholarshipPercent, 5000, "tuition")
	off := scholarship(t, "off", domain.ScholarshipFixedOff, 100000, "tuition")
	price := scholarship(t, "price", domain.ScholarshipFixedPrice, 100000, "tuition")
	free := scholarship(t, "free", domain.ScholarshipFixedPrice, 0, "tuition")
	other := scholarship(t, "other", domain.ScholarshipPercent, 10000, "enrollment")
	inactive, _ := domain.NewScholarship(domain.ScholarshipSpec{
		ID: "off-duty", TenantID: "t1", Name: "Apagada", Kind: domain.ScholarshipPercent, Value: 10000, Targets: []string{"tuition"},
	})
	beforeDue := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	onDueAfternoon := time.Date(2026, 9, 10, 17, 30, 0, 0, time.UTC)
	afterDue := day(2026, 9, 11)

	levelDiscounted := func(t *testing.T) domain.Charge {
		c, err := tuitionCharge(t).WithDiscount(20000, 0, domain.ReasonLevelDiscount)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	cases := []struct {
		name          string
		charge        func(*testing.T) domain.Charge
		paid, byDue   int64
		becas         []domain.Scholarship
		on            time.Time
		wantID        string
		wantCents     int64
		wantEffective int64
	}{
		{"percent before due", tuitionCharge, 0, 0, []domain.Scholarship{half}, beforeDue, "half", 150000, 150000},
		{"the due day counts whole, whatever the hour", tuitionCharge, 0, 0, []domain.Scholarship{half}, onDueAfternoon, "half", 150000, 150000},
		{"after due and unpaid: lost", tuitionCharge, 0, 0, []domain.Scholarship{half}, afterDue, "", 0, 300000},
		{"on the price after the level discount", levelDiscounted, 0, 0, []domain.Scholarship{half}, beforeDue, "half", 140000, 140000},
		{"fixed off", tuitionCharge, 0, 0, []domain.Scholarship{off}, beforeDue, "off", 100000, 200000},
		{"fixed price", tuitionCharge, 0, 0, []domain.Scholarship{price}, beforeDue, "price", 200000, 100000},
		{"no stacking: the most favourable wins", tuitionCharge, 0, 0, []domain.Scholarship{half, off, price}, beforeDue, "price", 200000, 100000},
		{"a scholarship for another target is not this charge's", tuitionCharge, 0, 0, []domain.Scholarship{other}, beforeDue, "", 0, 300000},
		{"an inactive scholarship applies to nothing", tuitionCharge, 0, 0, []domain.Scholarship{inactive}, beforeDue, "", 0, 300000},
		{"part paid: the rest at the scholarship price", tuitionCharge, 100000, 100000, []domain.Scholarship{half}, beforeDue, "half", 150000, 150000},
		{"part paid beyond the scholarship price: nothing refunded", tuitionCharge, 200000, 200000, []domain.Scholarship{half}, beforeDue, "half", 100000, 200000},
		{"settled in time stays settled after the due date", tuitionCharge, 150000, 150000, []domain.Scholarship{half}, afterDue, "half", 150000, 150000},
		{"paid only after the due date: lost", tuitionCharge, 150000, 0, []domain.Scholarship{half}, afterDue, "", 0, 300000},
		{"a free month needs no money to be in time", tuitionCharge, 0, 0, []domain.Scholarship{free}, afterDue, "free", 300000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charge := c.charge(t).WithStatus(c.paid)
			got, err := domain.ApplyScholarship(charge, c.paid, c.byDue, c.becas, domain.TargetTuition, c.on)
			if err != nil {
				t.Fatal(err)
			}
			if got.ScholarshipID() != c.wantID || got.Scholarship().Int64() != c.wantCents || got.Effective().Int64() != c.wantEffective {
				t.Fatalf("got beca %q %d effective %d; want %q %d effective %d",
					got.ScholarshipID(), got.Scholarship(), got.Effective(), c.wantID, c.wantCents, c.wantEffective)
			}
		})
	}
}

func TestApplyScholarshipLeavesAloneWhatItMustNotTouch(t *testing.T) {
	half := scholarship(t, "half", domain.ScholarshipPercent, 5000, "tuition")
	on := day(2026, 9, 5)

	paid := tuitionCharge(t).WithStatus(300000)
	if got, _ := domain.ApplyScholarship(paid, 300000, 300000, []domain.Scholarship{half}, domain.TargetTuition, on); got.Scholarship() != 0 {
		t.Fatal("a paid charge never changes")
	}
	cancelled, _ := tuitionCharge(t).Cancel()
	if got, _ := domain.ApplyScholarship(cancelled, 0, 0, []domain.Scholarship{half}, domain.TargetTuition, on); got.Scholarship() != 0 {
		t.Fatal("a cancelled charge owes nothing to discount")
	}
	frozen, err := tuitionCharge(t).WithScholarship("old", 50000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := domain.ApplyScholarship(frozen, 0, 0, []domain.Scholarship{half}, domain.TargetTuition, on); got.ScholarshipID() != "old" || got.Scholarship() != 50000 {
		t.Fatal("a scholarship written on the charge is history and stays")
	}
	if got, _ := domain.ApplyScholarship(tuitionCharge(t), 0, 0, []domain.Scholarship{half}, "", on); got.Scholarship() != 0 {
		t.Fatal("a charge with no target (a surcharge) takes no scholarship")
	}
}

func TestChargeScholarshipInvariants(t *testing.T) {
	c := tuitionCharge(t)
	if _, err := c.WithScholarship("", 100, 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatal("a scholarship part names its scholarship")
	}
	withLevel, _ := c.WithDiscount(20000, 0, domain.ReasonLevelDiscount)
	if _, err := withLevel.WithScholarship("s", 290000, 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatal("discount plus scholarship cannot exceed the amount")
	}
	beca, err := withLevel.WithScholarship("s", 280000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beca.WithDiscount(30000, 0, "a mano"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatal("a discount must fit beside the scholarship")
	}
	if _, err := beca.WithAmount(250000); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatal("a re-price must keep both parts within the amount")
	}
	if beca.Effective() != 0 || beca.DeriveStatus(0) != domain.StatusPending {
		t.Fatalf("a full scholarship owes nothing and stays pending until money lands, got %d %s", beca.Effective(), beca.DeriveStatus(0))
	}
	if got := beca.WithoutScholarship(); got.Scholarship() != 0 || got.ScholarshipID() != "" || got.Effective() != 280000 {
		t.Fatal("WithoutScholarship returns the charge at its price")
	}
}

func TestNewScholarshipHolding(t *testing.T) {
	h, err := domain.NewScholarshipHolding("h1", "t1", "s1", "st1", day(2026, 10, 2))
	if err != nil || h.ScholarshipID != "s1" || h.StudentID != "st1" || !h.EndsOn.IsZero() {
		t.Fatalf("valid holding: %+v %v", h, err)
	}
	for name, args := range map[string][4]string{
		"no id": {"", "t1", "s1", "st1"}, "no tenant": {"h1", "", "s1", "st1"},
		"no scholarship": {"h1", "t1", "", "st1"}, "no student": {"h1", "t1", "s1", " "},
	} {
		if _, err := domain.NewScholarshipHolding(args[0], args[1], args[2], args[3], day(2026, 10, 2)); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Fatalf("%s: want KindInvalid, got %v", name, err)
		}
	}
	if _, err := domain.NewScholarshipHolding("h1", "t1", "s1", "st1", time.Time{}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatal("a holding starts on a day")
	}
}
