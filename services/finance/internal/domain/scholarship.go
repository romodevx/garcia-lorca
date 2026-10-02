package domain

import (
	"math/bits"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// What a scholarship does to a price (T18, ADR 0022).
const (
	// ScholarshipPercent takes a share of the price off; its value is basis
	// points (5000 = 50 %).
	ScholarshipPercent = "percent"
	// ScholarshipFixedOff takes a fixed amount off, never below zero.
	ScholarshipFixedOff = "fixed_off"
	// ScholarshipFixedPrice makes the family pay a fixed amount, never more
	// than the price itself.
	ScholarshipFixedPrice = "fixed_price"
)

// What a scholarship can apply to. A concept or an activity is named by id, so
// renaming either keeps the scholarship on it.
const (
	TargetEnrollment = "enrollment"
	TargetTuition    = "tuition"
	// targetConceptPrefix + a catalog concept's id: its own concepts and the
	// plan's fees (one concept covers every fee).
	targetConceptPrefix = "concept:"
	// targetActivityPrefix + an activity's id (T17).
	targetActivityPrefix = "activity:"
)

const (
	// FullScholarshipBasisPoints is 100 %.
	FullScholarshipBasisPoints = 10_000
	// maxScholarshipTargets bounds what one scholarship may name.
	maxScholarshipTargets = 50
	// maxScholarshipName is what a statement line can print beside a charge.
	maxScholarshipName = 120
)

// targetIDShape is what an id in a target may look like: a token, no spaces or
// colons, at most 64 characters. The edge checks it is a uuid, and the use
// case that the concept or activity exists.
var targetIDShape = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// ConceptTarget is the target that names one catalog concept.
func ConceptTarget(conceptID string) string { return targetConceptPrefix + conceptID }

// ActivityTarget is the target that names one activity.
func ActivityTarget(activityID string) string { return targetActivityPrefix + activityID }

// validTarget reports whether a target is one of the four shapes.
func validTarget(t string) bool {
	switch t {
	case TargetEnrollment, TargetTuition:
		return true
	}
	for _, prefix := range []string{targetConceptPrefix, targetActivityPrefix} {
		if id, ok := strings.CutPrefix(t, prefix); ok {
			return targetIDShape.MatchString(id)
		}
	}
	return false
}

// ScholarshipSpec is what a scholarship is made of, as the director types it.
type ScholarshipSpec struct {
	ID       string
	TenantID string
	Name     string
	Kind     string
	// Value is basis points for a percent, cents for the two fixed kinds.
	Value   int64
	Targets []string
	Active  bool
}

// Scholarship is a standing reduction the school grants some families on some
// of what it charges. It is a rule, not money: what it takes off a charge is
// worked out when the charge is read or paid (ApplyScholarship), which is why
// assigning one reaches every charge still pending and removing it takes it
// back from them. Immutable.
type Scholarship struct {
	id       string
	tenantID string
	name     string
	kind     string
	value    int64
	targets  []string
	active   bool
}

// NewScholarship validates a spec. Every failure is KindInvalid.
func NewScholarship(spec ScholarshipSpec) (Scholarship, error) {
	if strings.TrimSpace(spec.ID) == "" {
		return Scholarship{}, glerrors.New(glerrors.KindInvalid, "scholarship id is required")
	}
	if strings.TrimSpace(spec.TenantID) == "" {
		return Scholarship{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Scholarship{}, glerrors.New(glerrors.KindInvalid, "el nombre de la beca es obligatorio")
	}
	if utf8.RuneCountInString(name) > maxScholarshipName {
		return Scholarship{}, glerrors.New(glerrors.KindInvalid, "el nombre de la beca es demasiado largo")
	}
	if err := validateScholarshipValue(spec.Kind, spec.Value); err != nil {
		return Scholarship{}, err
	}
	targets, err := validateTargets(spec.Targets)
	if err != nil {
		return Scholarship{}, err
	}
	return Scholarship{
		id: spec.ID, tenantID: spec.TenantID, name: name, kind: spec.Kind,
		value: spec.Value, targets: targets, active: spec.Active,
	}, nil
}

// validateScholarshipValue checks a value against its kind: a percent is
// 0 < bp ≤ 100 %, an amount off at least a centavo, a price to pay zero or more.
func validateScholarshipValue(kind string, value int64) error {
	switch kind {
	case ScholarshipPercent:
		if value <= 0 || value > FullScholarshipBasisPoints {
			return glerrors.New(glerrors.KindInvalid, "el porcentaje de la beca va de 0.01 % a 100 %")
		}
		return nil
	case ScholarshipFixedOff:
		if value <= 0 {
			return glerrors.New(glerrors.KindInvalid, "el descuento de la beca debe ser mayor que cero")
		}
	case ScholarshipFixedPrice:
		if value < 0 {
			return glerrors.New(glerrors.KindInvalid, "el monto a pagar no puede ser negativo")
		}
	default:
		return glerrors.New(glerrors.KindInvalid, "tipo de beca desconocido")
	}
	_, err := NewCents(value)
	return err
}

// validateTargets checks the list: 1–50, each one of the four shapes, no
// repeats. It returns a copy so the scholarship owns its slice.
func validateTargets(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "la beca debe aplicar a algo")
	}
	if len(in) > maxScholarshipTargets {
		return nil, glerrors.New(glerrors.KindInvalid, "la beca aplica a demasiados conceptos")
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		t := strings.TrimSpace(raw)
		if !validTarget(t) {
			return nil, glerrors.New(glerrors.KindInvalid, "concepto de beca no válido")
		}
		if slices.Contains(out, t) {
			return nil, glerrors.New(glerrors.KindInvalid, "un concepto aparece dos veces en la beca")
		}
		out = append(out, t)
	}
	return out, nil
}

// AppliesTo reports whether the scholarship, active, names the target.
func (s Scholarship) AppliesTo(target string) bool {
	return s.active && target != "" && slices.Contains(s.targets, target)
}

// SharesTargetWith reports whether two scholarships name a common target —
// the overlap the assignment screen warns about, since they do not stack.
func (s Scholarship) SharesTargetWith(o Scholarship) bool {
	for _, t := range s.targets {
		if slices.Contains(o.targets, t) {
			return true
		}
	}
	return false
}

// DiscountOn is what the scholarship takes off a base price: never more than
// the base, never negative. A percent rounds half up to the centavo (the half
// centavo goes to the family).
func (s Scholarship) DiscountOn(base Cents) Cents {
	b := base.Int64()
	switch s.kind {
	case ScholarshipPercent:
		return Cents(percentOf(b, s.value))
	case ScholarshipFixedOff:
		return Cents(min(s.value, b))
	case ScholarshipFixedPrice:
		return Cents(max(0, b-s.value))
	default:
		return 0
	}
}

// percentOf is round-half-up(base × bp / 10000) without overflowing: base is
// at most MaxCents (1e15) and bp at most 1e4, a product past int64.
func percentOf(base, bp int64) int64 {
	hi, lo := bits.Mul64(uint64(base), uint64(bp))
	lo, carry := bits.Add64(lo, FullScholarshipBasisPoints/2, 0)
	hi += carry
	q, _ := bits.Div64(hi, lo, FullScholarshipBasisPoints)
	return int64(q)
}

// BestScholarship is the scholarship most favourable to the family on a
// target and a base: the largest discount (they never stack). Ties go to the
// lowest id, so the same charge always names the same one.
func BestScholarship(candidates []Scholarship, target string, base Cents) (Scholarship, Cents, bool) {
	var (
		best  Scholarship
		most  Cents
		found bool
	)
	for _, s := range candidates {
		if !s.AppliesTo(target) {
			continue
		}
		d := s.DiscountOn(base)
		if d <= 0 {
			continue
		}
		if !found || d > most || (d == most && s.id < best.id) {
			best, most, found = s, d, true
		}
	}
	return best, most, found
}

// ApplyScholarship is THE rule of T18 (ADR 0022): what a scholarship takes off
// a charge when it is read or paid on `on`. Every path that prices a debt — the
// counter's quote, the deposit split, the statement, the receipts, the overdue
// views — runs through it.
//
//   - Order: the price after the discount already on the charge (the level's,
//     most often) is the base; the scholarship most favourable to the family
//     comes off it; early payment, if any, comes after (the caller).
//   - Deadline: the charge's own due date, whole day. Up to it the scholarship
//     applies. After it, only if what was paid BY the due date already covers
//     the scholarship price — the charge was settled in time (a 100 % month
//     needs no money for that). Otherwise it is lost: full price, and the
//     surcharge runs on the full price as for anyone late.
//   - Part paid: the scholarship price is what the charge costs in all; money
//     already on it counts towards it and is never refunded, so the part taken
//     off is capped at what is still unpaid.
//   - Paid, cancelled, or already carrying a scholarship part written when it
//     was settled: unchanged. That part is history.
//
// paidByDue is what was paid on or before the due date; paid is everything.
func ApplyScholarship(c Charge, paid, paidByDue int64, candidates []Scholarship, target string, on time.Time) (Charge, error) {
	if target == "" || c.scholarshipID != "" || c.status == StatusCancelled || c.status == StatusPaid {
		return c, nil
	}
	base := c.amount - c.discount
	best, part, ok := BestScholarship(candidates, target, base)
	if !ok {
		return c, nil
	}
	price := (base - part).Int64()
	if dayOf(on).After(dayOf(c.dueDate)) && paidByDue < price {
		return c, nil
	}
	part = min(part, max(0, base-Cents(paid)))
	if part == 0 {
		return c, nil
	}
	return c.WithScholarship(best.id, part.Int64(), paid)
}

// dayOf is the calendar day of an instant, in UTC like every finance date.
func dayOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// Accessors.
func (s Scholarship) ID() string       { return s.id }
func (s Scholarship) TenantID() string { return s.tenantID }
func (s Scholarship) Name() string     { return s.name }
func (s Scholarship) Kind() string     { return s.kind }
func (s Scholarship) Value() int64     { return s.value }
func (s Scholarship) Active() bool     { return s.active }

// Targets returns a copy of what the scholarship applies to.
func (s Scholarship) Targets() []string { return slices.Clone(s.targets) }

// ScholarshipHolding is a pupil holding a scholarship from StartsOn until
// EndsOn (the first day no longer held; zero = still held).
type ScholarshipHolding struct {
	ID            string
	TenantID      string
	ScholarshipID string
	StudentID     string
	StartsOn      time.Time
	EndsOn        time.Time
}

// NewScholarshipHolding validates an open holding. Every failure is KindInvalid.
func NewScholarshipHolding(id, tenantID, scholarshipID, studentID string, startsOn time.Time) (ScholarshipHolding, error) {
	for _, v := range []string{id, tenantID, scholarshipID, studentID} {
		if strings.TrimSpace(v) == "" {
			return ScholarshipHolding{}, glerrors.New(glerrors.KindInvalid, "a scholarship holding needs its ids")
		}
	}
	if startsOn.IsZero() {
		return ScholarshipHolding{}, glerrors.New(glerrors.KindInvalid, "a scholarship holding starts on a day")
	}
	return ScholarshipHolding{
		ID: id, TenantID: tenantID, ScholarshipID: scholarshipID, StudentID: studentID, StartsOn: dayOf(startsOn),
	}, nil
}
