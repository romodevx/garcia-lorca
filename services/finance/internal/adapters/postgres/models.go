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
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (chargeConceptModel) TableName() string { return "charge_concepts" }

// chargeModel is the GORM row for `charges`: one debt a student owes for one
// concept in one cycle. StudentID and CycleID are cross-service UUIDs validated
// by RPC — plain columns, never FKs. DueDate maps the `date` column, so only its
// date part is meaningful. Status is a cached projection of the payments posted
// against the charge (plus the terminal cancelled state).
type chargeModel struct {
	ID            string    `gorm:"column:id;primaryKey"`
	TenantID      string    `gorm:"column:tenant_id"`
	StudentID     string    `gorm:"column:student_id"`
	CycleID       string    `gorm:"column:cycle_id"`
	ConceptID     string    `gorm:"column:concept_id"`
	Description   string    `gorm:"column:description"`
	AmountCents   int64     `gorm:"column:amount_cents"`
	DiscountCents int64     `gorm:"column:discount_cents"`
	DueDate       time.Time `gorm:"column:due_date"`
	Status        string    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
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
}

func (paymentModel) TableName() string { return "payments" }
