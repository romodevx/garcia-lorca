package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// How often a concept is charged. The catalog is dynamic — each school defines
// its own concepts — but the recurrence vocabulary is closed so the future
// charge-generation job has a finite set of rules to implement.
const (
	// RecurrenceOnce is charged a single time (inscription, a trip, a uniform).
	RecurrenceOnce = "once"
	// RecurrenceMonthly is charged every month of the cycle (tuition).
	RecurrenceMonthly = "monthly"
)

// validateRecurrence enforces the closed recurrence vocabulary.
func validateRecurrence(recurrence string) error {
	switch recurrence {
	case RecurrenceOnce, RecurrenceMonthly:
		return nil
	default:
		return glerrors.New(glerrors.KindInvalid, "invalid recurrence")
	}
}

// ChargeConcept is one entry of a tenant's charge catalog: the template a
// Charge is created from (tuition, inscription, a field trip). defaultAmount is
// only a default — the resulting Charge carries its own amount, so editing the
// catalog never rewrites history.
//
// A concept is deactivated rather than deleted: charges already issued keep
// pointing at it, so the row must survive. The entity is immutable; WithUpdate
// returns a re-validated copy.
type ChargeConcept struct {
	id            string
	tenantID      string
	name          string
	defaultAmount Cents
	recurrence    string
	active        bool
}

// NewChargeConcept builds a validated ChargeConcept. Identity, tenant and name
// are required, the default amount must be non-negative, and the recurrence must
// belong to the closed vocabulary above. Every failure is KindInvalid.
func NewChargeConcept(id, tenantID, name string, defaultAmountCents int64, recurrence string, active bool) (ChargeConcept, error) {
	if strings.TrimSpace(id) == "" {
		return ChargeConcept{}, glerrors.New(glerrors.KindInvalid, "charge concept id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return ChargeConcept{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(name) == "" {
		return ChargeConcept{}, glerrors.New(glerrors.KindInvalid, "charge concept name is required")
	}
	amount, err := NewCents(defaultAmountCents)
	if err != nil {
		return ChargeConcept{}, err
	}
	if err := validateRecurrence(recurrence); err != nil {
		return ChargeConcept{}, err
	}
	return ChargeConcept{
		id:            id,
		tenantID:      tenantID,
		name:          name,
		defaultAmount: amount,
		recurrence:    recurrence,
		active:        active,
	}, nil
}

// WithUpdate returns a re-validated copy carrying the new mutable attributes,
// leaving the receiver and the concept's identity untouched.
func (c ChargeConcept) WithUpdate(name string, defaultAmountCents int64, recurrence string, active bool) (ChargeConcept, error) {
	return NewChargeConcept(c.id, c.tenantID, name, defaultAmountCents, recurrence, active)
}

// Accessors.
func (c ChargeConcept) ID() string           { return c.id }
func (c ChargeConcept) TenantID() string     { return c.tenantID }
func (c ChargeConcept) Name() string         { return c.name }
func (c ChargeConcept) DefaultAmount() Cents { return c.defaultAmount }
func (c ChargeConcept) Recurrence() string   { return c.recurrence }
func (c ChargeConcept) Active() bool         { return c.active }
