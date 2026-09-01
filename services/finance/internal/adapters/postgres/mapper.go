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
		Policy: domain.CollectionPolicy{
			// A row written before M16 has mode 'monthly' by default, so it keeps
			// meaning exactly what it meant when it was billed.
			DailyMode:              m.LateFeeMode == "daily",
			LateFeeDailyCents:      m.LateFeeDailyCents,
			SurchargeCountsAllDays: m.SurchargeCountsAllDays,
			EarlyPaymentCents:      m.EarlyPaymentCents,
			EarlyPaymentDays:       m.EarlyPaymentDays,
		},
	})
}

// fromDomainPlan projects a tuition table into its GORM row.
func fromDomainPlan(p domain.TuitionPlan) tuitionPlanModel {
	return tuitionPlanModel{
		ID: p.ID(), TenantID: p.TenantID(), CycleID: p.CycleID(), LevelID: p.LevelID(),
		EnrollmentCents: p.EnrollmentCents(), MonthlyCents: p.MonthlyCents(),
		Months: p.Months(), FirstBilledMonth: p.FirstBilledMonth(),
		DueDay: p.DueDay(), LateFeeCents: p.LateFeeCents(), Active: p.Active(),
		LateFeeDailyCents:      p.Policy().LateFeeDailyCents,
		LateFeeMode:            lateFeeMode(p.Policy().DailyMode),
		SurchargeCountsAllDays: p.Policy().SurchargeCountsAllDays,
		EarlyPaymentCents:      p.Policy().EarlyPaymentCents,
		EarlyPaymentDays:       p.Policy().EarlyPaymentDays,
	}
}

// lateFeeMode is the column's vocabulary. Spelled out rather than stored as a
// boolean so a person reading the table sees which rule a plan is on.
func lateFeeMode(daily bool) string {
	if daily {
		return "daily"
	}
	return "monthly"
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
	if m.DiscountCents > 0 {
		c, err = c.WithDiscount(m.DiscountCents, paidCents, m.DiscountReason)
		if err != nil {
			return domain.Charge{}, err
		}
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
		ID:             c.ID(),
		TenantID:       c.TenantID(),
		StudentID:      c.StudentID(),
		CycleID:        c.CycleID(),
		ConceptID:      c.ConceptID(),
		Description:    c.Description(),
		AmountCents:    c.Amount().Int64(),
		DiscountCents:  c.Discount().Int64(),
		DiscountReason: c.DiscountReason(),
		DueDate:        c.DueDate(),
		Status:         c.Status(),
		ExternalRef:    c.ExternalRef(),
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

// --- deposits (M16) --------------------------------------------------------

func fromDomainDeposit(d domain.Deposit) depositModel {
	m := depositModel{
		ID:              d.ID(),
		TenantID:        d.TenantID(),
		StudentID:       d.StudentID(),
		AmountCents:     d.Amount().Int64(),
		Method:          d.Method(),
		Reference:       d.Reference(),
		PaidAt:          d.PaidAt(),
		IdempotencyKey:  d.IdempotencyKey(),
		CalendarVersion: d.CalendarVersion(),
		VoidReason:      d.VoidReason(),
	}
	if d.Voided() {
		at := d.VoidedAt()
		m.VoidedAt = &at
	}
	return m
}

func toDomainDeposit(m depositModel) (domain.Deposit, error) {
	d, err := domain.NewDeposit(m.ID, m.TenantID, m.StudentID, m.AmountCents,
		m.Method, m.Reference, m.PaidAt, m.IdempotencyKey, m.CalendarVersion)
	if err != nil {
		return domain.Deposit{}, err
	}
	if m.VoidedAt == nil {
		return d, nil
	}
	// Rehydrate rather than Void: the constructor is for new money and
	// deliberately cannot produce a cancelled deposit, but a stored one has to
	// come back exactly as it was written, reason included.
	return d.Rehydrate(*m.VoidedAt, m.VoidReason), nil
}
