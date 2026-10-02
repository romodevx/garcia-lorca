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
	// EnrollmentStartsOn is the first day the inscription opens to families
	// (T5). NULL is open from issue; never set without EnrollmentDueOn
	// (tuition_plans_enrollment_window_ck, migration 0020).
	EnrollmentStartsOn *time.Time `gorm:"column:enrollment_starts_on"`
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
	ID        string `gorm:"column:id;primaryKey"`
	TenantID  string `gorm:"column:tenant_id"`
	StudentID string `gorm:"column:student_id"`
	// CycleID is nil for a sale to a pupil with no cycle yet (migration 0017),
	// which then answers to ExternalRef.
	CycleID       *string `gorm:"column:cycle_id"`
	ConceptID     string  `gorm:"column:concept_id"`
	Description   string  `gorm:"column:description"`
	AmountCents   int64   `gorm:"column:amount_cents"`
	DiscountCents int64   `gorm:"column:discount_cents"`
	// Why the discount exists. Empty on the ones given before there was
	// anywhere to record it, which is the truth about them.
	DiscountReason string    `gorm:"column:discount_reason"`
	DueDate        time.Time `gorm:"column:due_date"`
	// StartsOn is the first day the charge opens to the family (T5, migration
	// 0020). NULL is open from issue — every charge written before it existed.
	StartsOn *time.Time `gorm:"column:starts_on"`
	Status   string     `gorm:"column:status"`
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
	// CardType is debit or credit on a card payment (T8, migration 0022).
	CardType string `gorm:"column:card_type"`
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
	// Folio is the receipt number (task 12, migration 0018).
	Folio string `gorm:"column:folio"`
	// The correction link (T9, migration 0023): the deposit that replaced
	// this one, and the one this one corrects.
	SupersededAt      *time.Time `gorm:"column:superseded_at"`
	SupersededBy      *string    `gorm:"column:superseded_by"`
	CorrectsDepositID *string    `gorm:"column:corrects_deposit_id"`
}

func (depositModel) TableName() string { return "deposits" }

// depositCorrectionModel is the GORM row for `deposit_corrections` (T9,
// migration 0023): one correction of a deposit, before and after as JSON.
type depositCorrectionModel struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	TenantID           string    `gorm:"column:tenant_id"`
	StudentID          string    `gorm:"column:student_id"`
	OriginalDepositID  string    `gorm:"column:original_deposit_id"`
	CorrectedDepositID string    `gorm:"column:corrected_deposit_id"`
	Reason             string    `gorm:"column:reason"`
	Before             []byte    `gorm:"column:before;type:jsonb"`
	After              []byte    `gorm:"column:after;type:jsonb"`
	CorrectedBy        *string   `gorm:"column:corrected_by"`
	CorrectedByName    string    `gorm:"column:corrected_by_name"`
	CorrectedAt        time.Time `gorm:"column:corrected_at"`
}

func (depositCorrectionModel) TableName() string { return "deposit_corrections" }

// depositTenderModel is the GORM row for `deposit_tenders` (T8, migration
// 0021): one method a deposit came in by, in the order the cashier listed it.
// The table is the source of truth for "how was this paid"; deposits.method
// and deposits.card_type are a summary written beside it.
type depositTenderModel struct {
	// The database mints the id: a tender has no domain identity.
	ID          string `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	TenantID    string `gorm:"column:tenant_id"`
	DepositID   string `gorm:"column:deposit_id"`
	Position    int    `gorm:"column:position"`
	Method      string `gorm:"column:method"`
	AmountCents int64  `gorm:"column:amount_cents"`
	Reference   string `gorm:"column:reference"`
	CardType    string `gorm:"column:card_type"`
}

func (depositTenderModel) TableName() string { return "deposit_tenders" }

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

// --- T10: expenses and the cash cut (migration 0024) ------------------------

// expenseCategoryModel is the GORM row for `expense_categories`.
type expenseCategoryModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	Name      string    `gorm:"column:name"`
	Active    bool      `gorm:"column:active"`
	CreatedAt time.Time `gorm:"column:created_at;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:now()"`
}

func (expenseCategoryModel) TableName() string { return "expense_categories" }

// expenseModel is the GORM row for `expenses`. SpentOn maps a `date`.
type expenseModel struct {
	ID                 string     `gorm:"column:id;primaryKey"`
	TenantID           string     `gorm:"column:tenant_id"`
	SpentOn            time.Time  `gorm:"column:spent_on"`
	CategoryID         string     `gorm:"column:category_id"`
	AmountCents        int64      `gorm:"column:amount_cents"`
	Method             string     `gorm:"column:method"`
	Description        string     `gorm:"column:description"`
	Status             string     `gorm:"column:status"`
	CancelReason       string     `gorm:"column:cancel_reason"`
	CancelledAt        *time.Time `gorm:"column:cancelled_at"`
	ReceiptKey         *string    `gorm:"column:receipt_key"`
	ReceiptContentType *string    `gorm:"column:receipt_content_type"`
	ReceiptSizeBytes   *int       `gorm:"column:receipt_size_bytes"`
	CreatedBy          *string    `gorm:"column:created_by"`
	CreatedByName      string     `gorm:"column:created_by_name"`
	CreatedAt          time.Time  `gorm:"column:created_at;default:now()"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;default:now()"`
	// CategoryName is read with the expense (a join), never written.
	CategoryName string `gorm:"column:category_name;->"`
}

func (expenseModel) TableName() string { return "expenses" }

// expenseEventModel is the GORM row for `expense_events`: one change of an
// expense, before and after as JSON.
type expenseEventModel struct {
	ID              string    `gorm:"column:id;primaryKey"`
	TenantID        string    `gorm:"column:tenant_id"`
	ExpenseID       string    `gorm:"column:expense_id"`
	Kind            string    `gorm:"column:kind"`
	Before          []byte    `gorm:"column:before;type:jsonb"`
	After           []byte    `gorm:"column:after;type:jsonb"`
	Reason          string    `gorm:"column:reason"`
	SpentOn         time.Time `gorm:"column:spent_on"`
	PreviousSpentOn time.Time `gorm:"column:previous_spent_on"`
	ActorID         *string   `gorm:"column:actor_id"`
	ActorName       string    `gorm:"column:actor_name"`
	At              time.Time `gorm:"column:at"`
}

func (expenseEventModel) TableName() string { return "expense_events" }

// cashCutClosureModel is the GORM row for `cash_cut_closures`.
type cashCutClosureModel struct {
	ID                string     `gorm:"column:id;primaryKey"`
	TenantID          string     `gorm:"column:tenant_id"`
	Day               time.Time  `gorm:"column:day"`
	ExpectedCashCents int64      `gorm:"column:expected_cash_cents"`
	CountedCashCents  int64      `gorm:"column:counted_cash_cents"`
	DifferenceCents   int64      `gorm:"column:difference_cents"`
	Snapshot          []byte     `gorm:"column:snapshot;type:jsonb"`
	Note              string     `gorm:"column:note"`
	ClosedBy          *string    `gorm:"column:closed_by"`
	ClosedByName      string     `gorm:"column:closed_by_name"`
	ClosedAt          time.Time  `gorm:"column:closed_at"`
	ReopenedAt        *time.Time `gorm:"column:reopened_at"`
	ReopenedBy        *string    `gorm:"column:reopened_by"`
	ReopenedByName    string     `gorm:"column:reopened_by_name"`
	ReopenReason      string     `gorm:"column:reopen_reason"`
}

func (cashCutClosureModel) TableName() string { return "cash_cut_closures" }

// activityModel is the GORM row for `activities` (T17, migration 0025).
// Schedule is the JSONB [{weekday, start, end}]; the dates map `date`.
type activityModel struct {
	ID              string     `gorm:"column:id;primaryKey"`
	TenantID        string     `gorm:"column:tenant_id"`
	Name            string     `gorm:"column:name"`
	Schedule        []byte     `gorm:"column:schedule;type:jsonb"`
	Periodicity     string     `gorm:"column:periodicity"`
	BillingStartsOn time.Time  `gorm:"column:billing_starts_on"`
	FirstDueOn      time.Time  `gorm:"column:first_due_on"`
	LateFeeCents    int64      `gorm:"column:late_fee_cents"`
	Active          bool       `gorm:"column:active"`
	ResumedOn       *time.Time `gorm:"column:resumed_on"`
	CreatedAt       time.Time  `gorm:"column:created_at;default:now()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;default:now()"`
}

func (activityModel) TableName() string { return "activities" }

// activityOptionModel is the GORM row for `activity_price_options`.
// DaysPerWeek NULL is the flat fee.
type activityOptionModel struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TenantID    string    `gorm:"column:tenant_id"`
	ActivityID  string    `gorm:"column:activity_id"`
	DaysPerWeek *int      `gorm:"column:days_per_week"`
	AmountCents int64     `gorm:"column:amount_cents"`
	Retired     bool      `gorm:"column:retired"`
	CreatedAt   time.Time `gorm:"column:created_at;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updated_at;default:now()"`
}

func (activityOptionModel) TableName() string { return "activity_price_options" }

// activityEnrollmentModel is the GORM row for `activity_enrollments`.
type activityEnrollmentModel struct {
	ID         string     `gorm:"column:id;primaryKey"`
	TenantID   string     `gorm:"column:tenant_id"`
	ActivityID string     `gorm:"column:activity_id"`
	StudentID  string     `gorm:"column:student_id"`
	OptionID   string     `gorm:"column:option_id"`
	StartsOn   time.Time  `gorm:"column:starts_on"`
	EndsOn     *time.Time `gorm:"column:ends_on"`
	CreatedAt  time.Time  `gorm:"column:created_at;default:now()"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;default:now()"`
}

func (activityEnrollmentModel) TableName() string { return "activity_enrollments" }
