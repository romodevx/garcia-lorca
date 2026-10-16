package postgres

import (
	"encoding/json"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
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
	schedule, err := scheduleToDomain(m.Schedule)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	fees, err := feesToDomain(m.Fees)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	concepts, err := enrollmentConceptsToDomain(m.EnrollmentConcepts)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	var enrollmentDue, enrollmentStarts time.Time
	if m.EnrollmentDueOn != nil {
		enrollmentDue = *m.EnrollmentDueOn
	}
	if m.EnrollmentStartsOn != nil {
		enrollmentStarts = *m.EnrollmentStartsOn
	}
	return domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: m.ID, TenantID: m.TenantID, CycleID: m.CycleID, LevelID: m.LevelID,
		EnrollmentCents: m.EnrollmentCents, MonthlyCents: m.MonthlyCents,
		Months: m.Months, FirstBilledMonth: m.FirstBilledMonth,
		DueDay: m.DueDay, LateFeeCents: m.LateFeeCents, Active: m.Active,
		Schedule:             schedule,
		MonthlyDiscountCents: m.MonthlyDiscountCents,
		Fees:                 fees,
		EnrollmentDueOn:      enrollmentDue,
		EnrollmentStartsOn:   enrollmentStarts,
		EnrollmentConcepts:   concepts,
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
func fromDomainPlan(p domain.TuitionPlan) (tuitionPlanModel, error) {
	concepts, err := enrollmentConceptsFromDomain(p.EnrollmentConcepts())
	if err != nil {
		return tuitionPlanModel{}, err
	}
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
		Schedule:               scheduleFromDomain(p.Schedule()),
		MonthlyDiscountCents:   p.MonthlyDiscountCents(),
		Fees:                   feesFromDomain(p.Fees()),
		EnrollmentDueOn:        optionalDate(p.EnrollmentDueOn()),
		EnrollmentStartsOn:     optionalDate(p.EnrollmentStartsOn()),
		EnrollmentConcepts:     concepts,
	}, nil
}

// A price step's boundary is stored as the calendar day the school wrote down —
// dateLayout, the same one the rest of this package uses — and nothing else.
//
// Marshalling domain.PriceTier straight to JSON would store an INSTANT with an
// offset, and this service is already six hours out of step with the school it
// serves (see AmountOn). A boundary that could drift by a timezone is one that
// moves a family between $2,200 and $3,300 depending on how a row was written.
// priceTierJSON and enrollmentConceptJSON are the stored shape. They live here,
// in the adapter, because the storage format is not the domain's business — and
// because pinning it explicitly is what stops a field rename in the domain
// silently orphaning every price list already saved.
type priceTierJSON struct {
	EffectiveFrom string `json:"effective_from"`
	// Empty when the school set no closing date. Omitted from the stored JSON so
	// a row written before ranges existed reads back identically.
	EffectiveTo string `json:"effective_to,omitempty"`
	AmountCents int64  `json:"amount_cents"`
}

type enrollmentConceptJSON struct {
	Name  string          `json:"name"`
	Tiers []priceTierJSON `json:"tiers"`
}

// enrollmentConceptsToDomain reads the stored inscription. A row written before
// M18 holds "[]" and yields none, which is the single-amount inscription every
// plan had until now.
func enrollmentConceptsToDomain(raw []byte) ([]domain.EnrollmentConcept, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var stored []enrollmentConceptJSON
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the inscription concepts", err)
	}
	out := make([]domain.EnrollmentConcept, 0, len(stored))
	for _, c := range stored {
		tiers := make([]domain.PriceTier, 0, len(c.Tiers))
		for _, tier := range c.Tiers {
			day, err := time.Parse(dateLayout, tier.EffectiveFrom)
			if err != nil {
				// NOT degraded to "no ladder". A concept that lost its steps
				// would be priced at nothing and the school would find out in
				// June; refusing the read is how somebody finds out today.
				return nil, glerrors.Wrap(glerrors.KindInternal,
					"reading the date a price step starts", err)
			}
			var until time.Time
			if tier.EffectiveTo != "" {
				until, err = time.Parse(dateLayout, tier.EffectiveTo)
				if err != nil {
					return nil, glerrors.Wrap(glerrors.KindInternal,
						"reading the date a price step stops", err)
				}
			}
			tiers = append(tiers, domain.PriceTier{
				EffectiveFrom: day, EffectiveTo: until, AmountCents: tier.AmountCents,
			})
		}
		out = append(out, domain.EnrollmentConcept{Name: c.Name, Tiers: tiers})
	}
	return out, nil
}

func enrollmentConceptsFromDomain(in []domain.EnrollmentConcept) ([]byte, error) {
	if len(in) == 0 {
		return []byte("[]"), nil
	}
	stored := make([]enrollmentConceptJSON, 0, len(in))
	for _, c := range in {
		tiers := make([]priceTierJSON, 0, len(c.Tiers))
		for _, tier := range c.Tiers {
			stored := priceTierJSON{
				EffectiveFrom: tier.EffectiveFrom.Format(dateLayout),
				AmountCents:   tier.AmountCents,
			}
			if !tier.EffectiveTo.IsZero() {
				stored.EffectiveTo = tier.EffectiveTo.Format(dateLayout)
			}
			tiers = append(tiers, stored)
		}
		stored = append(stored, enrollmentConceptJSON{Name: c.Name, Tiers: tiers})
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		// Reported, never degraded. Unlike fees, an empty list here does not mean
		// "no extras" — it means "charge the single EnrollmentCents", which for a
		// plan that has none is charging the family nothing. And returning nil
		// into a NOT NULL column would surface as a not-null violation with the
		// real cause already thrown away.
		return nil, glerrors.Wrap(glerrors.KindInternal, "storing the inscription concepts", err)
	}
	return raw, nil
}

// feesToDomain reads the stored fees. A row written before fees existed holds
// "[]" and yields none, which is a level that collects only its tuition.
func feesToDomain(raw []byte) ([]domain.Fee, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []domain.Fee
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the level fees", err)
	}
	return out, nil
}

func feesFromDomain(in []domain.Fee) []byte {
	if len(in) == 0 {
		return []byte("[]")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		// Marshalling names and ints cannot fail. An empty list is still a
		// valid answer, so this degrades to "no fees" rather than losing the
		// whole tuition table.
		return []byte("[]")
	}
	return raw
}

// optionalDate keeps "no date set" as NULL — no deadline, no opening day.
// Writing the zero time instead would bill every inscription as due in year 1.
func optionalDate(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// scheduleToDomain reads the stored payment plan. A row written before the
// schedule existed holds "[]" and yields none, which is the uniform year.
func scheduleToDomain(raw []byte) ([]domain.Installment, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []domain.Installment
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the tuition schedule", err)
	}
	return out, nil
}

func scheduleFromDomain(in []domain.Installment) []byte {
	if len(in) == 0 {
		return []byte("[]")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		// Marshalling a slice of two ints cannot fail; an empty list is still
		// a valid schedule, so this degrades to the uniform year rather than
		// losing the write.
		return []byte("[]")
	}
	return []byte(raw)
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
	c, err := chargeOf(m)
	if err != nil {
		return domain.Charge{}, err
	}
	if m.DiscountCents > 0 {
		c, err = c.WithDiscount(m.DiscountCents, paidCents, m.DiscountReason)
		if err != nil {
			return domain.Charge{}, err
		}
	}
	if m.ScholarshipCents > 0 && m.ScholarshipID != nil {
		c, err = c.WithScholarship(*m.ScholarshipID, m.ScholarshipCents, paidCents)
		if err != nil {
			return domain.Charge{}, err
		}
	}
	if m.ExternalRef != "" && m.CycleID != nil {
		c, err = c.WithExternalRef(m.ExternalRef)
		if err != nil {
			return domain.Charge{}, err
		}
	}
	if m.StartsOn != nil {
		if c, err = c.WithStartsOn(*m.StartsOn); err != nil {
			return domain.Charge{}, err
		}
	}
	if m.Status == domain.StatusCancelled {
		cancelled, err := c.Cancel()
		if err != nil {
			return domain.Charge{}, err
		}
		return cancelled.WithCancellationReason(m.CancellationReason), nil
	}
	return c.WithStatus(paidCents), nil
}

// chargeOf builds the pending charge a row describes: a cycle-free sale charge
// (with its ref) or the school's own.
func chargeOf(m chargeModel) (domain.Charge, error) {
	if m.CycleID == nil {
		return domain.NewSaleCharge(m.ID, m.TenantID, m.StudentID, m.ConceptID,
			m.Description, m.AmountCents, m.DueDate, m.ExternalRef)
	}
	return domain.NewCharge(m.ID, m.TenantID, m.StudentID, *m.CycleID, m.ConceptID,
		m.Description, m.AmountCents, 0, m.DueDate)
}

// cycleColumn is the cycle as the column stores it: NULL when there is none.
func cycleColumn(cycleID string) *string { return optionalString(cycleID) }

// optionalString is a nullable text column: NULL for "".
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// fromDomainCharge projects a domain Charge into its GORM row for insert.
// CreatedAt/UpdatedAt are left zero so the DB assigns them via DEFAULT now().
func fromDomainCharge(c domain.Charge) chargeModel {
	return chargeModel{
		ID:                 c.ID(),
		TenantID:           c.TenantID(),
		StudentID:          c.StudentID(),
		CycleID:            cycleColumn(c.CycleID()),
		ConceptID:          c.ConceptID(),
		Description:        c.Description(),
		AmountCents:        c.Amount().Int64(),
		DiscountCents:      c.Discount().Int64(),
		DiscountReason:     c.DiscountReason(),
		CancellationReason: c.CancellationReason(),
		ScholarshipCents:   c.Scholarship().Int64(),
		ScholarshipID:      optionalString(c.ScholarshipID()),
		DueDate:            c.DueDate(),
		StartsOn:           optionalDate(c.StartsOn()),
		Status:             c.Status(),
		ExternalRef:        c.ExternalRef(),
	}
}

// toDomainPayment rebuilds a domain Payment from its row, with the account that
// took the money when it was recorded.
func toDomainPayment(m paymentModel) (domain.Payment, error) {
	p, err := domain.NewPayment(m.ID, m.TenantID, m.ChargeID, m.AmountCents, m.Method, m.Reference, m.PaidAt, m.IdempotencyKey)
	if err != nil {
		return p, err
	}
	if m.DepositID != nil {
		p = p.WithDepositID(*m.DepositID)
	}
	if m.ReceivedBy != nil {
		p = p.WithReceivedBy(*m.ReceivedBy)
	}
	return p.WithCardType(m.CardType)
}

// fromDomainPayment projects a domain Payment into its GORM row for insert.
// CreatedAt is left zero (DEFAULT now()).
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
		ReceivedBy:     nullableUUID(p.ReceivedBy()),
		CardType:       p.CardType(),
	}
}

// nullableUUID stores an empty account id as NULL, which is what the uuid
// column means by "not recorded".
func nullableUUID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// --- deposits (M16) --------------------------------------------------------

func fromDomainDeposit(d domain.Deposit) depositModel {
	m := depositModel{
		Folio:           d.Folio(),
		ID:              d.ID(),
		TenantID:        d.TenantID(),
		StudentID:       d.StudentID(),
		AmountCents:     d.Amount().Int64(),
		Method:          d.Method(),
		Reference:       d.Reference(),
		PaidAt:          d.PaidAt(),
		IdempotencyKey:  d.IdempotencyKey(),
		CalendarVersion: d.CalendarVersion(),
		CardType:        d.CardType(),
		VoidReason:      d.VoidReason(),
		ReceivedBy:      nullableUUID(d.ReceivedBy().UserID),
		ReceivedByName:  d.ReceivedBy().Name,
		// Written with the deposit; the superseded pair is written by the
		// correction itself, under the original's row lock.
		CorrectsDepositID: nullableUUID(d.Corrects()),
	}
	if d.Voided() {
		at := d.VoidedAt()
		m.VoidedAt = &at
	}
	return m
}

// fromDomainTenders is a deposit's tenders as rows, numbered in order.
func fromDomainTenders(d domain.Deposit) []depositTenderModel {
	tenders := d.Tenders()
	out := make([]depositTenderModel, 0, len(tenders))
	for i, t := range tenders {
		out = append(out, depositTenderModel{
			TenantID: d.TenantID(), DepositID: d.ID(), Position: i,
			Method: t.Method(), AmountCents: t.Amount().Int64(),
			Reference: t.Reference(), CardType: t.CardType(),
		})
	}
	return out
}

// toDomainDeposit rebuilds a stored deposit from its row and its tenders
// (ordered by position). The tenders are the source of truth: the row's
// method and card type are their summary, re-derived here by WithTenders.
// A deposit with no tender rows (none since migration 0021 backfilled them)
// falls back to its own single method.
//
// folios resolves the receipt numbers of the deposits this one is linked to by
// a correction (T9): read for the whole page in one query by the caller.
func toDomainDeposit(m depositModel, tenderRows []depositTenderModel, folios map[string]string) (domain.Deposit, error) {
	method := m.Method
	if len(tenderRows) > 0 {
		method = tenderRows[0].Method
	}
	d, err := domain.NewDeposit(m.ID, m.TenantID, m.StudentID, m.AmountCents,
		method, m.Reference, m.PaidAt, m.IdempotencyKey, m.CalendarVersion)
	if err != nil {
		return domain.Deposit{}, err
	}
	d, err = withStoredTenders(d, m.CardType, tenderRows)
	if err != nil {
		return domain.Deposit{}, err
	}
	receiver := domain.Receiver{Name: m.ReceivedByName}
	if m.ReceivedBy != nil {
		receiver.UserID = *m.ReceivedBy
	}
	d = d.WithReceiver(receiver).WithCapturedAt(m.CreatedAt).WithFolio(m.Folio)
	if m.CorrectsDepositID != nil {
		d = d.WithCorrects(*m.CorrectsDepositID, folios[*m.CorrectsDepositID])
	}
	if m.SupersededAt != nil && m.SupersededBy != nil {
		d = d.RehydrateSupersession(*m.SupersededAt, *m.SupersededBy, folios[*m.SupersededBy])
	}
	if m.VoidedAt == nil {
		return d, nil
	}
	// Rehydrate rather than Void: the constructor is for new money and
	// deliberately cannot produce a cancelled deposit, but a stored one has to
	// come back exactly as it was written, reason included.
	return d.Rehydrate(*m.VoidedAt, m.VoidReason), nil
}

// withStoredTenders puts a stored deposit's tenders back on it, or its card
// type when it has none.
func withStoredTenders(d domain.Deposit, cardType string, rows []depositTenderModel) (domain.Deposit, error) {
	if len(rows) == 0 {
		return d.WithCardType(cardType)
	}
	tenders := make([]domain.Tender, 0, len(rows))
	for _, r := range rows {
		t, err := domain.NewTender(r.Method, r.AmountCents, r.Reference, r.CardType)
		if err != nil {
			return domain.Deposit{}, glerrors.Wrap(glerrors.KindInternal, "a stored deposit tender is invalid", err)
		}
		tenders = append(tenders, t)
	}
	return d.WithTenders(tenders)
}

// --- corrections (T9) ------------------------------------------------------

// snapshotJSON is a deposit snapshot as deposit_corrections stores it: plain
// JSON a person reading the row with psql can follow.
type snapshotJSON struct {
	DepositID   string         `json:"depositId"`
	Folio       string         `json:"folio"`
	AmountCents int64          `json:"amountCents"`
	PaidAt      time.Time      `json:"paidAt"`
	Tenders     []tenderJSON   `json:"tenders"`
	Lines       []snapLineJSON `json:"lines"`
}

type tenderJSON struct {
	Method      string `json:"method"`
	AmountCents int64  `json:"amountCents"`
	Reference   string `json:"reference,omitempty"`
	CardType    string `json:"cardType,omitempty"`
}

type snapLineJSON struct {
	ChargeID    string `json:"chargeId"`
	AmountCents int64  `json:"amountCents"`
}

func toSnapshotJSON(s domain.DepositSnapshot) ([]byte, error) {
	out := snapshotJSON{DepositID: s.DepositID, Folio: s.Folio, AmountCents: s.AmountCents, PaidAt: s.PaidAt,
		Tenders: make([]tenderJSON, 0, len(s.Tenders)), Lines: make([]snapLineJSON, 0, len(s.Lines))}
	for _, t := range s.Tenders {
		out.Tenders = append(out.Tenders, tenderJSON(t))
	}
	for _, l := range s.Lines {
		out.Lines = append(out.Lines, snapLineJSON(l))
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "encoding a correction snapshot", err)
	}
	return raw, nil
}

func fromSnapshotJSON(raw []byte) (domain.DepositSnapshot, error) {
	var in snapshotJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return domain.DepositSnapshot{}, glerrors.Wrap(glerrors.KindInternal, "a stored correction snapshot is unreadable", err)
	}
	out := domain.DepositSnapshot{DepositID: in.DepositID, Folio: in.Folio, AmountCents: in.AmountCents, PaidAt: in.PaidAt}
	for _, t := range in.Tenders {
		out.Tenders = append(out.Tenders, domain.TenderSnapshot(t))
	}
	for _, l := range in.Lines {
		out.Lines = append(out.Lines, domain.SnapshotLine(l))
	}
	return out, nil
}

func fromDomainCorrection(c domain.DepositCorrection) (depositCorrectionModel, error) {
	before, err := toSnapshotJSON(c.Before())
	if err != nil {
		return depositCorrectionModel{}, err
	}
	after, err := toSnapshotJSON(c.After())
	if err != nil {
		return depositCorrectionModel{}, err
	}
	return depositCorrectionModel{
		ID: c.ID(), TenantID: c.TenantID(), StudentID: c.StudentID(),
		OriginalDepositID: c.OriginalID(), CorrectedDepositID: c.CorrectedID(), Reason: c.Reason(),
		Before: before, After: after,
		CorrectedBy: nullableUUID(c.By().UserID), CorrectedByName: c.By().Name, CorrectedAt: c.At(),
	}, nil
}

func toDomainCorrection(m depositCorrectionModel) (domain.DepositCorrection, error) {
	before, err := fromSnapshotJSON(m.Before)
	if err != nil {
		return domain.DepositCorrection{}, err
	}
	after, err := fromSnapshotJSON(m.After)
	if err != nil {
		return domain.DepositCorrection{}, err
	}
	by := domain.Receiver{Name: m.CorrectedByName}
	if m.CorrectedBy != nil {
		by.UserID = *m.CorrectedBy
	}
	c, err := domain.NewDepositCorrection(m.ID, m.TenantID, m.StudentID, m.OriginalDepositID, m.CorrectedDepositID,
		m.Reason, before, after, by, m.CorrectedAt)
	if err != nil {
		return domain.DepositCorrection{}, glerrors.Wrap(glerrors.KindInternal, "a stored correction is invalid", err)
	}
	return c, nil
}
