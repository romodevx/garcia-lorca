package postgres

import (
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain.

// toDomainConcept rebuilds a domain ChargeConcept from its row.
func toDomainConcept(m chargeConceptModel) (domain.ChargeConcept, error) {
	c, err := domain.NewChargeConcept(m.ID, m.TenantID, m.Name, m.DefaultAmountCents, m.Recurrence, m.Active)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	// Rows written before migration 0004 carry the column default, so an empty
	// kind reads as custom rather than as a validation failure.
	if m.Kind == "" {
		return c, nil
	}
	return c.WithKind(m.Kind)
}

// toDomainPlan rebuilds a tuition table from its row.
func toDomainPlan(m tuitionPlanModel) (domain.TuitionPlan, error) {
	return domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: m.ID, TenantID: m.TenantID, CycleID: m.CycleID, LevelID: m.LevelID,
		EnrollmentCents: m.EnrollmentCents, MonthlyCents: m.MonthlyCents,
		Months: m.Months, FirstBilledMonth: m.FirstBilledMonth,
		DueDay: m.DueDay, LateFeeCents: m.LateFeeCents, Active: m.Active,
	})
}

// fromDomainPlan projects a tuition table into its GORM row.
func fromDomainPlan(p domain.TuitionPlan) tuitionPlanModel {
	return tuitionPlanModel{
		ID: p.ID(), TenantID: p.TenantID(), CycleID: p.CycleID(), LevelID: p.LevelID(),
		EnrollmentCents: p.EnrollmentCents(), MonthlyCents: p.MonthlyCents(),
		Months: p.Months(), FirstBilledMonth: p.FirstBilledMonth(),
		DueDay: p.DueDay(), LateFeeCents: p.LateFeeCents(), Active: p.Active(),
	}
}

// fromDomainConcept projects a domain ChargeConcept into its GORM row.
// CreatedAt/UpdatedAt are left zero so the DB assigns them via DEFAULT now().
func fromDomainConcept(c domain.ChargeConcept) chargeConceptModel {
	return chargeConceptModel{
		ID:                 c.ID(),
		TenantID:           c.TenantID(),
		Name:               c.Name(),
		DefaultAmountCents: c.DefaultAmount().Int64(),
		Recurrence:         c.Recurrence(),
		Active:             c.Active(),
		Kind:               c.Kind(),
	}
}

// toDomainCharge rebuilds a domain Charge from its row and the total collected
// against it.
//
// The domain deliberately exposes no setter that takes an arbitrary status
// string — status is derived, not assigned — so the stored status is
// reconstructed rather than injected: NewCharge yields a pending charge, and
// then either Cancel() (the one status that is NOT derivable, being terminal and
// set by the school) or WithStatus(paidCents) re-derives exactly what the writer
// derived when it stored the row. Given the same paid total, the result's
// Status() equals the stored status.
func toDomainCharge(m chargeModel, paidCents int64) (domain.Charge, error) {
	c, err := domain.NewCharge(
		m.ID, m.TenantID, m.StudentID, m.CycleID, m.ConceptID,
		m.Description, m.AmountCents, m.DiscountCents, m.DueDate,
	)
	if err != nil {
		return domain.Charge{}, err
	}
	if m.ExternalRef != "" {
		c, err = c.WithExternalRef(m.ExternalRef)
		if err != nil {
			return domain.Charge{}, err
		}
	}
	if m.Status == domain.StatusCancelled {
		return c.Cancel()
	}
	return c.WithStatus(paidCents), nil
}

// fromDomainCharge projects a domain Charge into its GORM row for insert.
// CreatedAt/UpdatedAt are left zero so the DB assigns them via DEFAULT now().
func fromDomainCharge(c domain.Charge) chargeModel {
	return chargeModel{
		ID:            c.ID(),
		TenantID:      c.TenantID(),
		StudentID:     c.StudentID(),
		CycleID:       c.CycleID(),
		ConceptID:     c.ConceptID(),
		Description:   c.Description(),
		AmountCents:   c.Amount().Int64(),
		DiscountCents: c.Discount().Int64(),
		DueDate:       c.DueDate(),
		Status:        c.Status(),
		ExternalRef:   c.ExternalRef(),
	}
}

// toDomainPayment rebuilds a domain Payment from its row. ReceivedBy is not part
// of the domain yet (reserved for cashier attribution).
func toDomainPayment(m paymentModel) (domain.Payment, error) {
	return domain.NewPayment(m.ID, m.TenantID, m.ChargeID, m.AmountCents, m.Method, m.Reference, m.PaidAt, m.IdempotencyKey)
}

// fromDomainPayment projects a domain Payment into its GORM row for insert.
// ReceivedBy is deliberately left nil and CreatedAt zero (DEFAULT now()).
func fromDomainPayment(p domain.Payment) paymentModel {
	return paymentModel{
		ID:             p.ID(),
		TenantID:       p.TenantID(),
		ChargeID:       p.ChargeID(),
		AmountCents:    p.Amount().Int64(),
		Method:         p.Method(),
		Reference:      p.Reference(),
		IdempotencyKey: p.IdempotencyKey(),
		PaidAt:         p.PaidAt(),
	}
}
