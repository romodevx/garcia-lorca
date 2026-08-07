package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// This file declares the seams the finance use cases depend on. Interfaces live
// here (consumer side); adapters (students/config Connect clients, postgres,
// pubsub) implement them. The domain imports none of this — dependencies point
// inward.
//
// Finance owns the ledger and nothing else: who the student is lives in
// students, what a cycle is lives in config. Both are reached by RPC through
// minimal, read-only projections that carry only what the ledger needs to
// validate a reference — never PII.

// ChargeWithPaid pairs a charge with the total collected against it. The domain
// Charge deliberately holds no paid field (the payments are the source of
// truth), so every read and return path that needs the pair carries it here.
// What the student still owes is Charge.Effective() - PaidCents, floored at
// zero: an over-collected charge is a credit, never a negative debt.
type ChargeWithPaid struct {
	Charge    domain.Charge
	PaidCents int64
}

// StudentRef is the finance-owned view of a students record: identity and
// whether the enrolment is live. No name, no PII.
type StudentRef struct {
	ID     string
	Status string
}

// CycleRef is the finance-owned view of a config school cycle: its identity is
// all the ledger needs to stamp a charge.
type CycleRef struct {
	ID string
}

// EnrollmentRef is the finance-owned view of a students enrollment: enough to
// know which student, in which group of which cycle, and whether the enrollment
// is live. Bulk charge generation expands a cycle (or one of its groups) into
// these and issues one charge per active row.
type EnrollmentRef struct {
	ID        string
	StudentID string
	GroupID   string
	CycleID   string
	Status    string
}

// StudentReader validates student references against the students bounded
// context (by RPC). It is a consumer-defined seam: the use case depends on this
// interface and stays testable with a fake, while the real adapter is a Connect
// client. A missing student is reported as glerrors.ErrNotFound (KindNotFound);
// any other error propagates unchanged.
type StudentReader interface {
	ByID(ctx context.Context, id string) (StudentRef, error)
}

// CycleReader validates school-cycle references against the config bounded
// context (by RPC). A missing cycle is reported as glerrors.ErrNotFound
// (KindNotFound); any other error propagates unchanged.
type CycleReader interface {
	ByID(ctx context.Context, id string) (CycleRef, error)
}

// EnrollmentReader lists the enrollments a bulk charge generation expands over,
// read from the students bounded context (by RPC). An empty groupID means the
// whole cycle; a non-empty one narrows to that group. Filtering by status is the
// use case's job, so the caller sees exactly what students reported.
type EnrollmentReader interface {
	ListByCycle(ctx context.Context, cycleID, groupID string) ([]EnrollmentRef, error)
}

// PaymentRegisteredEvent announces that money was collected. Consumers (comms
// for the receipt notification, scholarships for its own accounting) react to
// it; finance does not wait on them.
type PaymentRegisteredEvent struct {
	PaymentID   string
	StudentID   string
	ChargeID    string
	TenantID    string
	Method      string
	AmountCents int64
	PaidAt      time.Time
}

// PaymentPublisher emits the PaymentRegistered domain event. Publishing is
// best-effort and happens after the payment is durably stored: a broker outage
// must never lose money that was already collected, so the use case ignores the
// returned error (and tolerates a nil publisher).
type PaymentPublisher interface {
	PublishPaymentRegistered(ctx context.Context, e PaymentRegisteredEvent) error
}

// FinanceRepository loads and persists the ledger: the charge catalog, the
// charges issued from it and the payments posted against them. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction,
// as in students/iam).
type FinanceRepository interface {
	// CreateConcept stores a new catalog entry. A duplicate name within the
	// tenant is reported as KindConflict.
	CreateConcept(ctx context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error)
	// GetConcept loads one catalog entry; a missing id yields glerrors.ErrNotFound.
	GetConcept(ctx context.Context, id string) (domain.ChargeConcept, error)
	// ListConcepts returns the catalog. Deactivated entries are hidden unless
	// includeInactive is set — they are never deleted, because issued charges
	// keep pointing at them.
	ListConcepts(ctx context.Context, includeInactive bool) ([]domain.ChargeConcept, error)
	// UpdateConcept persists a re-validated copy of an existing entry.
	UpdateConcept(ctx context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error)

	// CreateCharge issues one charge. A second charge for the same
	// (student, cycle, concept) is reported as KindConflict — a student owes a
	// given concept once per cycle.
	CreateCharge(ctx context.Context, c domain.Charge) (domain.Charge, error)
	// CreateChargesBulk issues many charges in ONE statement, skipping the rows
	// whose (student, cycle, concept) key already exists (ON CONFLICT DO
	// NOTHING) instead of failing the batch. It returns how many rows were
	// actually written, which makes re-running a generation idempotent.
	CreateChargesBulk(ctx context.Context, cs []domain.Charge) (createdCount int, err error)
	// GetCharge loads one charge together with its collected total; a missing id
	// yields glerrors.ErrNotFound.
	GetCharge(ctx context.Context, id string) (ChargeWithPaid, error)
	// ListCharges returns charges with their collected totals, resolved in ONE
	// tenant-bound query rather than a payment lookup per charge (no N+1). An
	// empty studentID or cycleID skips that filter.
	ListCharges(ctx context.Context, studentID, cycleID string) ([]ChargeWithPaid, error)
	// SaveCharge persists the mutable part of an existing charge (its discount
	// and derived status), as left by ApplyDiscount or CancelCharge.
	SaveCharge(ctx context.Context, c domain.Charge) (domain.Charge, error)

	// RegisterPayment posts a payment against a charge atomically, in ONE
	// tenant-bound transaction: it locks the charge row, sums the payments
	// already posted against it, and calls derive with the charge and the total
	// the new payment would bring it to. derive owns the decision — it returns
	// the charge's new status, or an error that ABORTS the transaction (the
	// overpay and cancelled-charge rules live there, so they are enforced under
	// the row lock rather than in a read-then-write race). On success the
	// payment is inserted and the charge's status and updated_at are written in
	// the same transaction. Returns the stored payment and the charge with its
	// new collected total. A missing charge yields glerrors.ErrNotFound.
	//
	// A payment carrying a non-empty IdempotencyKey is deduplicated inside that
	// same transaction: if the tenant already has a payment under the key, the
	// stored one is returned with replayed=true and NOTHING is written — no
	// second insert, no derive, no status change. An empty key opts out (every
	// call posts a payment) and always reports replayed=false. The caller uses
	// the flag to suppress the side effects of a retry, notably the event.
	RegisterPayment(
		ctx context.Context,
		p domain.Payment,
		derive func(charge domain.Charge, newPaidCents int64) (status string, err error),
	) (stored domain.Payment, charge ChargeWithPaid, replayed bool, err error)
	// ListPayments returns every payment posted against the student's charges,
	// for the account statement.
	ListPayments(ctx context.Context, studentID string) ([]domain.Payment, error)
	// ListOverdue returns the charges past their due date that still owe money
	// (due_date < today, effective - paid > 0, not cancelled). An empty cycleID
	// spans every cycle. today is passed in, never read from a clock here, so
	// the query stays deterministic and testable.
	ListOverdue(ctx context.Context, cycleID string, today time.Time) ([]ChargeWithPaid, error)
}
