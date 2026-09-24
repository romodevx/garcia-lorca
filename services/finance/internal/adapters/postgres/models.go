package postgres

import "time"

// models.go holds the GORM rows for the finance ledger. They exist only so the
// domain entities never carry persistence tags (double model + mapper.go).
//
// Every monetary column is an integer amount of cents (bigint) — never a float.
// CreatedAt/UpdatedAt are DB-managed (DEFAULT now(); updates bump updated_at
// explicitly).

// chargeConceptModel is the GORM row for `charge_concepts`, the tenant's dynamic
// catalog of billable concepts. A concept is deactivated (active=false), never
// deleted, because issued charges keep pointing at it.
type chargeConceptModel struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	TenantID           string    `gorm:"column:tenant_id"`
	Name               string    `gorm:"column:name"`
	DefaultAmountCents int64     `gorm:"column:default_amount_cents"`
	Recurrence         string    `gorm:"column:recurrence"`
	Active             bool      `gorm:"column:active"`
	Kind               string    `gorm:"column:kind"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (chargeConceptModel) TableName() string { return "charge_concepts" }

// tuitionPlanModel is the GORM row for `tuition_plans`: what one level costs in
// one cycle. CycleID and LevelID are cross-service UUIDs validated by RPC, so
// they carry no FK here.
type tuitionPlanModel struct {
	ID               string `gorm:"column:id;primaryKey"`
	TenantID         string `gorm:"column:tenant_id"`
	CycleID          string `gorm:"column:cycle_id"`
	LevelID          string `gorm:"column:level_id"`
	EnrollmentCents  int64  `gorm:"column:enrollment_cents"`
	MonthlyCents     int64  `gorm:"column:monthly_cents"`
	Months           int    `gorm:"column:months"`
	FirstBilledMonth int    `gorm:"column:first_billed_month"`
	DueDay           int    `gorm:"column:due_day"`
	LateFeeCents     int64  `gorm:"column:late_fee_cents"`
	// M16 collection policy. late_fee_mode decides which of the two surcharge
	// rules applies; existing rows default to 'monthly' so nothing already billed
	// changes meaning.
	LateFeeDailyCents      int64  `gorm:"column:late_fee_daily_cents"`
	LateFeeMode            string `gorm:"column:late_fee_mode"`
	SurchargeCountsAllDays bool   `gorm:"column:surcharge_counts_all_days"`
	EarlyPaymentCents      int64  `gorm:"column:early_payment_cents"`
	EarlyPaymentDays       int    `gorm:"column:early_payment_days"`
	// Schedule is the plan's explicit payment plan as JSON, "[]" when the year
	// is divided into equal monthly parts. Raw bytes and not a JSON helper type:
	// one column does not justify pulling a driver for another database into a
	// Postgres service.
	Schedule []byte `gorm:"column:schedule;type:jsonb"`
	// Fees are the extra amounts collected on top of the tuition, as JSON, "[]"
	// when the level has none. Raw bytes for the same reason as Schedule.
	Fees []byte `gorm:"column:fees;type:jsonb"`
	// EnrollmentConcepts are the lines the inscription is broken into, each with
	// its price ladder, as JSON — "[]" when the plan carries the single
	// EnrollmentCents it always did. The two are mutually exclusive, enforced by
	// the domain and by tuition_plans_one_inscription_shape_ck (migration 0015).
	EnrollmentConcepts []byte `gorm:"column:enrollment_concepts;type:jsonb"`
	// EnrollmentDueOn is the deadline the school set for the inscription. NULL
	// when it set none, which is what makes the run fall back to the old rule
	// instead of billing everything due on the zero date.
	EnrollmentDueOn *time.Time `gorm:"column:enrollment_due_on"`
	// MonthlyDiscountCents comes off every month of this level.
	MonthlyDiscountCents int64     `gorm:"column:monthly_discount_cents"`
	Active               bool      `gorm:"column:active"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (tuitionPlanModel) TableName() string { return "tuition_plans" }

// chargeModel is the GORM row for `charges`: one debt a student owes for one
// concept in one cycle. StudentID and CycleID are cross-service UUIDs validated
// by RPC — plain columns, never FKs. DueDate maps the `date` column, so only its
// date part is meaningful. Status is a cached projection of the payments posted
// against the charge (plus the terminal cancelled state).
type chargeModel struct {
	ID            string `gorm:"column:id;primaryKey"`
	TenantID      string `gorm:"column:tenant_id"`
	StudentID     string `gorm:"column:student_id"`
	CycleID       string `gorm:"column:cycle_id"`
	ConceptID     string `gorm:"column:concept_id"`
	Description   string `gorm:"column:description"`
	AmountCents   int64  `gorm:"column:amount_cents"`
	DiscountCents int64  `gorm:"column:discount_cents"`
	// Why the discount exists. Empty on the ones given before there was
	// anywhere to record it, which is the truth about them.
	DiscountReason string    `gorm:"column:discount_reason"`
	DueDate        time.Time `gorm:"column:due_date"`
	Status         string    `gorm:"column:status"`
	// ExternalRef ties the charge to the row that caused it in another module
	// (a uniforms fulfillment, say). Empty means the school's own charge; the
	// empty string is what the "one concept per cycle" partial index covers, so
	// tuition keeps its idempotency while referenced charges may repeat
	// (migration 0003).
	ExternalRef string    `gorm:"column:external_ref"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (chargeModel) TableName() string { return "charges" }

// paymentModel is the GORM row for `payments`: money received against a charge.
// Append-only — there is no update path. ReceivedBy is a nullable column
// RESERVED for cashier attribution and is never written yet, so it maps to
// *string and stays NULL. IdempotencyKey is the caller's handle on the payment
// ATTEMPT; the empty string means "not deduplicated" and is excluded from the
// partial unique index (migration 0002).
type paymentModel struct {
	ID             string    `gorm:"column:id;primaryKey"`
	TenantID       string    `gorm:"column:tenant_id"`
	ChargeID       string    `gorm:"column:charge_id"`
	AmountCents    int64     `gorm:"column:amount_cents"`
	Method         string    `gorm:"column:method"`
	Reference      string    `gorm:"column:reference"`
	ReceivedBy     *string   `gorm:"column:received_by"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	PaidAt         time.Time `gorm:"column:paid_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	// DepositID is set when this payment is one slice of a deposit. NULL means a
	// payment made the old way, against one named charge.
	DepositID *string `gorm:"column:deposit_id"`
}

func (paymentModel) TableName() string { return "payments" }

// depositModel is the GORM row for `deposits` (M16): money as it arrived,
// before it was decided what it pays. Its allocations are payments carrying
// deposit_id, so there is still exactly one place that answers "what has been
// collected against this charge".
type depositModel struct {
	ID              string     `gorm:"column:id;primaryKey"`
	TenantID        string     `gorm:"column:tenant_id"`
	StudentID       string     `gorm:"column:student_id"`
	AmountCents     int64      `gorm:"column:amount_cents"`
	Method          string     `gorm:"column:method"`
	Reference       string     `gorm:"column:reference"`
	PaidAt          time.Time  `gorm:"column:paid_at"`
	IdempotencyKey  string     `gorm:"column:idempotency_key"`
	CalendarVersion string     `gorm:"column:calendar_version"`
	CardType        string     `gorm:"column:card_type"`
	VoidedAt        *time.Time `gorm:"column:voided_at"`
	VoidReason      string     `gorm:"column:void_reason"`
	// Who took the money (task 7); NULL / '' on deposits from before it was kept.
	ReceivedBy     *string   `gorm:"column:received_by"`
	ReceivedByName string    `gorm:"column:received_by_name"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (depositModel) TableName() string { return "deposits" }

// outboxModel is the GORM row for `finance_outbox`: a domain event queued in
// the same transaction that produced it, waiting to be published.
type outboxModel struct {
	// The database mints the id. An outbox row has no domain identity — it is a
	// queue entry — so nothing upstream has a name for it, and the `default` tag
	// is what stops GORM sending an empty string for a uuid column.
	ID          string     `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	TenantID    string     `gorm:"column:tenant_id"`
	EventType   string     `gorm:"column:event_type"`
	Payload     []byte     `gorm:"column:payload;type:jsonb"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	PublishedAt *time.Time `gorm:"column:published_at"`
	Attempts    int        `gorm:"column:attempts"`
}

func (outboxModel) TableName() string { return "finance_outbox" }
