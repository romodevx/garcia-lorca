package postgres

import (
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain.

// toDomainConcept rebuilds a domain ChargeConcept from its row.
func toDomainConcept(m chargeConceptModel) (domain.ChargeConcept, error) {
	return domain.NewChargeConcept(m.ID, m.TenantID, m.Name, m.DefaultAmountCents, m.Recurrence, m.Active)
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
	}
}

// toDomainPayment rebuilds a domain Payment from its row. ReceivedBy is not part
// of the domain yet (reserved for cashier attribution).
func toDomainPayment(m paymentModel) (domain.Payment, error) {
	return domain.NewPayment(m.ID, m.TenantID, m.ChargeID, m.AmountCents, m.Method, m.Reference, m.PaidAt)
}

// fromDomainPayment projects a domain Payment into its GORM row for insert.
// ReceivedBy is deliberately left nil and CreatedAt zero (DEFAULT now()).
func fromDomainPayment(p domain.Payment) paymentModel {
	return paymentModel{
		ID:          p.ID(),
		TenantID:    p.TenantID(),
		ChargeID:    p.ChargeID(),
		AmountCents: p.Amount().Int64(),
		Method:      p.Method(),
		Reference:   p.Reference(),
		PaidAt:      p.PaidAt(),
	}
}
