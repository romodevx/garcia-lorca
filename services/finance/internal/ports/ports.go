package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
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
// LoosePayment is a payment taken outside a deposit, with the description of
// the charge it paid, so a cash-up can say what the money was for.
type LoosePayment struct {
	Payment     domain.Payment
	Description string
}

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
	// StartsOn anchors the billing calendar. "Ten payments from September" is
	// meaningless without knowing which September, and a cycle that opens in
	// August 2026 reaches June of 2027 — the year is derived, never stated twice.
	StartsOn time.Time
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
	// EnrolledAt is the day the pupil took their place. The run needs it to know
	// which months are theirs: billing the whole year to somebody who arrived in
	// March charges them for months the school never taught them.
	//
	// Zero for every enrolment recorded before finance started reading it, which
	// the run treats as "was already there" — the whole year, exactly as today.
	EnrolledAt time.Time
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
	// ListByStudent returns one student's enrolments across every cycle. A
	// deposit needs it to find where the pupil sits: the group leads to the
	// level, and the level's tuition table carries the collection rule.
	ListByStudent(ctx context.Context, studentID string) ([]EnrollmentRef, error)
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

// ChargeSettledEvent announces that a charge became FULLY covered — the
// crossing into paid, not every collection along the way. It exists for the
// consumer that only cares whether the money is in (uniforms, before it releases
// goods): summing PaymentRegistered would make that consumer redo the ledger's
// arithmetic, depend on delivery order, and subscribe to every payment the
// school takes to discard most of them.
//
// ExternalRef echoes the charge's reference, so the consumer resolves its own
// row without calling back into finance.
type ChargeSettledEvent struct {
	ChargeID    string
	StudentID   string
	TenantID    string
	ExternalRef string
	AmountCents int64
	SettledAt   time.Time
}

// LevelRef is the finance-owned view of a config level: its identity, and the
// name the panel shows when reporting a level that has no tuition table yet.
type LevelRef struct {
	ID   string
	Name string
}

// GroupLevel maps a group of a cycle to the level it belongs to. It is the join
// finance cannot make on its own: an enrolment names a GROUP, a price is set per
// LEVEL, and the path between them (group → grade → level) lives in config.
type GroupLevel struct {
	GroupID string
	LevelID string
}

// CatalogReader reads the academic catalogue finance needs to price a cycle.
// Read-only and by RPC: config owns this data and finance never writes it.
type CatalogReader interface {
	// ListLevels returns the school's levels.
	ListLevels(ctx context.Context) ([]LevelRef, error)
	// GroupLevels resolves every group of a cycle to its level, in one call, so
	// the billing run does not make a round trip per enrolment.
	GroupLevels(ctx context.Context, cycleID string) ([]GroupLevel, error)
}

// CalendarSnapshot is the school calendar as finance saw it at one moment: the
// days the school is shut, plus a version to stamp on whatever was computed
// from it.
//
// Version exists so a recomputation months later can tell whether it is looking
// at the same calendar the cashier saw. Stale says the snapshot did not come
// fresh from config — either it was served from cache after a failed read, or
// config was unreachable and this is the weekends-only fallback. A number
// derived from a stale calendar is still a number a person has to defend, so it
// is recorded as such rather than passed off as current.
type CalendarSnapshot struct {
	Days    []time.Time
	Version string
	Stale   bool
}

// CalendarReader tells finance which days the school does not operate. config
// owns the calendar; finance only reads it, and only to work out deadlines
// counted in working days.
type CalendarReader interface {
	NonWorkingDays(ctx context.Context, cycleID string) (CalendarSnapshot, error)
}

// DepositWrite is everything one deposit turns into: the surcharge charges it
// makes real, the payments that split it across debts, and what is left over.
//
// It is produced by a callback the repository runs INSIDE the transaction, with
// the student's open charges already locked. Planning outside and writing inside
// would reopen the race the lock exists to close: two cashiers taking money from
// the same family at once would each plan against the same balance and both
// allocate it.
type DepositWrite struct {
	// Surcharges are late-fee charges materialised because this money is landing
	// on them. The daily rule is derived, not billed, so a surcharge row exists
	// only where there was movement.
	Surcharges []domain.Charge
	// Repriced are charges whose AMOUNT this deposit fixed: the inscription
	// ladder priced at the day the money arrived. Written before everything
	// else, because the discounts and the allocations are both worked out
	// against the new number — and that write is what freezes the ladder.
	Repriced []domain.Charge
	// Discounted are charges whose discount this deposit changed — the
	// early-payment reward, granted only where the money actually settles the
	// month. They are written before the allocations that assume the new amount.
	Discounted []domain.Charge
	// Allocations are the payments, each naming the charge it lands on.
	Allocations []domain.Payment
	// UnallocatedCents is the family's credit: money beyond what they owed.
	UnallocatedCents int64
}

// IncomeLine is what one concept earned in a period.
type IncomeLine struct {
	ConceptID   string
	Concept     string
	AmountCents int64
}

// ReceiptLine is one concept a deposit paid, as its receipt prints it (task 12).
type ReceiptLine struct {
	ChargeID    string
	Description string
	// AmountCents is what THIS deposit put on the charge.
	AmountCents int64
	// Surcharge marks a late fee (daily, monthly or typed by hand).
	Surcharge bool
	// DiscountCents is the charge's discount, printed beside it.
	DiscountCents int64
}

// DepositReceipt is a deposit and what it paid, for its receipt.
type DepositReceipt struct {
	Deposit          domain.Deposit
	Lines            []ReceiptLine
	UnallocatedCents int64
}

// DepositRecord is a stored deposit with what it did.
type DepositRecord struct {
	Deposit          domain.Deposit
	Surcharges       []domain.Charge
	Allocations      []domain.Payment
	UnallocatedCents int64
	// Settled names the charges this deposit closed. The caller announces them,
	// and only them: a consumer that releases goods on ChargeSettled must never
	// act on a partial payment.
	Settled []ChargeWithPaid
}

// The event types the outbox carries. They are the names the messaging boundary
// stamps, so a row read with psql says what it is without a lookup.
const (
	OutboxChargeSettled     = "ChargeSettled"
	OutboxPaymentRegistered = "PaymentRegistered"
)

// OutboxEvent is a domain event queued inside the transaction that produced it.
//
// It exists because publishing after a commit has no second chance: a broker
// that is down, or a container that dies in between, loses the event and nothing
// notices. Written here, either the payment and its event both land or neither
// does, and a publication that fails is simply tried again.
//
// Payload is JSON of the domain event, not proto bytes. What goes on the wire is
// the adapter's business; what belongs in a durable queue is the fact, in a form
// a person debugging a stuck row can read.
type OutboxEvent struct {
	Type    string
	Payload []byte
}

// PaymentPublisher emits the finance domain events. Publishing is best-effort
// and happens after the payment is durably stored: a broker outage must never
// lose money that was already collected, so the use case ignores the returned
// error (and tolerates a nil publisher).
type PaymentPublisher interface {
	PublishPaymentRegistered(ctx context.Context, e PaymentRegisteredEvent) error
	// PublishChargeSettled fires only on the payment that closes the charge.
	PublishChargeSettled(ctx context.Context, e ChargeSettledEvent) error
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

	// CreateCharge issues one charge. A KEYLESS charge (no external ref) may
	// exist once per (student, cycle, concept) — a student owes tuition once a
	// cycle — and a second one is KindConflict. A charge carrying an external
	// ref is identified by that ref instead, so it may repeat the concept
	// freely while a retried creation still conflicts (migration 0003).
	CreateCharge(ctx context.Context, c domain.Charge) (domain.Charge, error)
	// CreateChargesBulk issues many charges in ONE statement, skipping the rows
	// whose (student, cycle, concept) key already exists (ON CONFLICT DO
	// NOTHING) instead of failing the batch. It returns how many rows were
	// actually written, which makes re-running a generation idempotent.
	CreateChargesBulk(ctx context.Context, cs []domain.Charge) (createdCount int, err error)
	// RescheduleUnpaid moves the due date of charges identified by external ref.
	//
	// It exists because the run never touches a charge it already issued, which
	// is what makes it idempotent — and that left the school unable to correct a
	// deadline it had already billed. Setting the inscription's due date did
	// nothing for pupils already charged, so the setting looked broken.
	//
	// It moves ONLY what is unpaid and only where the date actually differs. A
	// settled charge is history; re-dating it would rewrite when a family was
	// due to pay something they have already paid.
	RescheduleUnpaid(ctx context.Context, items []ChargeReschedule) (movedCount int, err error)
	// RepriceUnpaid sets the amount of charges identified by external ref.
	//
	// It is what makes the school's price ladder true after the charge exists:
	// an inscription issued in January at $2,200 is worth $3,300 in March, and
	// goes on rising until it is settled (ADR 0011). The run writes the tier in
	// force so the database agrees with what the family is being shown, rather
	// than leaving the truth derived at read time and absent from every report.
	//
	// It moves ONLY what has taken no money and carries no discount. The first
	// peso freezes the price — a family holding a receipt for a quoted amount
	// must never find the statement disagreeing with it — and a discount is a
	// price somebody decided by hand, which a price list must not overwrite.
	RepriceUnpaid(ctx context.Context, items []ChargeReprice) (repricedCount int, err error)
	// GetCharge loads one charge together with its collected total; a missing id
	// yields glerrors.ErrNotFound.
	GetCharge(ctx context.Context, id string) (ChargeWithPaid, error)
	// ListCharges returns charges with their collected totals, resolved in ONE
	// tenant-bound query rather than a payment lookup per charge (no N+1). An
	// empty studentID or cycleID skips that filter.
	ListCharges(ctx context.Context, studentID, cycleID string) ([]ChargeWithPaid, error)
	// RegisterDeposit stores money as it arrived and the payments it splits
	// into, in ONE transaction. plan is run inside that transaction with the
	// student's open charges locked and their collected totals already summed,
	// so the split is decided against a balance that cannot move under it.
	//
	// A repeat under the same idempotency key returns what was stored the first
	// time (replayed = true) and collects nothing: the retry a lost response
	// provokes must not cost a family twice.
	//
	// outbox runs inside the same transaction, after the allocations are
	// applied, and returns the events the deposit produced.
	RegisterDeposit(ctx context.Context, d domain.Deposit,
		plan func(open []ChargeWithPaid) (DepositWrite, error),
		outbox func(record DepositRecord) []OutboxEvent) (DepositRecord, bool, error)

	// UnallocatedCents is money a family has handed over that no charge has
	// claimed: the sum of their live deposits, less what those deposits paid.
	// Cancelled deposits do not count — that money went back.
	UnallocatedCents(ctx context.Context, studentID string) (int64, error)

	// IncomeLines sums what was collected between two dates, grouped by the
	// concept the money was charged for. Cancelled charges are excluded: money
	// against a voided charge is a correction to make, not income to report.
	IncomeLines(ctx context.Context, from, to time.Time) ([]IncomeLine, error)

	// DrainOutbox publishes the events waiting in the outbox, oldest first, and
	// marks the ones that went out.
	//
	// publish is called per event; an event it refuses stays unpublished, its
	// attempt count climbs, and the next drain tries it again. Rows are taken
	// under SKIP LOCKED so two instances draining at once never publish the same
	// event twice.
	DrainOutbox(ctx context.Context, limit int, publish func(OutboxEvent) error) (int, error)

	// GetDeposit returns one deposit by id, or KindNotFound.
	GetDeposit(ctx context.Context, id string) (domain.Deposit, error)
	// DepositReceipt reads a deposit and what it paid, for its receipt (task
	// 12), or KindNotFound.
	DepositReceipt(ctx context.Context, id string) (DepositReceipt, error)

	// ListDeposits returns a student's deposits, newest first.
	ListDeposits(ctx context.Context, studentID string) ([]domain.Deposit, error)

	// ListDepositsOn returns every deposit taken on one day, cancelled ones
	// included — the day's cash-up decides what to do with those, and a reader
	// that never saw them could not explain a gap.
	ListDepositsOn(ctx context.Context, day time.Time) ([]domain.Deposit, error)

	// ListLoosePaymentsOn returns the payments taken on one day outside any
	// deposit — a counter sale's, paid straight against its charge — with what
	// each one paid for. They are in the drawer as much as a deposit is.
	ListLoosePaymentsOn(ctx context.Context, day time.Time) ([]LoosePayment, error)

	// VoidDeposit cancels a deposit and releases its allocations, in one
	// transaction. The deposit row survives with its reason; the payments it
	// created do not, so the charges they settled go back to being owed.
	VoidDeposit(ctx context.Context, d domain.Deposit) error

	// CancelChargeIfUnpaid voids a charge only if nothing was collected against
	// it, deciding under the charge's row lock so the read cannot go stale
	// before the write. A charge with payments is KindConflict. Automated
	// callers use this; CancelCharge stays the school's unguarded override.
	CancelChargeIfUnpaid(ctx context.Context, id string) (ChargeWithPaid, error)
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
	//
	// outbox runs inside the SAME transaction, after the status is derived, and
	// returns the events this payment produced. Queuing them here rather than
	// publishing after the commit is what stops an event being lost when the
	// broker is down or the container dies in between.
	RegisterPayment(
		ctx context.Context,
		p domain.Payment,
		derive func(charge domain.Charge, newPaidCents int64) (status string, err error),
		outbox func(charge ChargeWithPaid) []OutboxEvent,
	) (stored domain.Payment, charge ChargeWithPaid, replayed bool, err error)
	// ListPayments returns every payment posted against the student's charges,
	// for the account statement.
	ListPayments(ctx context.Context, studentID string) ([]domain.Payment, error)
	// ListOverdue returns the charges past their due date that still owe money
	// (due_date < today, effective - paid > 0, not cancelled). An empty cycleID
	// spans every cycle. today is passed in, never read from a clock here, so
	// the query stays deterministic and testable.
	ListOverdue(ctx context.Context, cycleID string, today time.Time) ([]ChargeWithPaid, error)

	// ListOverduePage walks the overdue set in (due_date, id) order without a
	// cap, one page at a time, starting strictly after `after`.
	//
	// ListOverdue above is capped for the UI, which is correct there: nobody
	// reads five thousand rows on a screen. The BILLING RUN is different — it
	// must see every overdue charge or it silently under-bills the newest debt,
	// and a school with 400 pupils crosses that cap in its first cycle. Two
	// readers with genuinely different needs, so two methods rather than one
	// that is wrong for somebody.
	ListOverduePage(ctx context.Context, cycleID string, today time.Time, after OverdueCursor, limit int) ([]ChargeWithPaid, error)

	// EnsureConceptOfKind returns the tenant's concept for a system kind. The id
	// is supplied by the caller (ids are minted in the use case, not here) and is
	// used only when the concept has to be created.
	// (tuition, enrollment, late_fee), creating it with the given name if the
	// school has none yet. Looking a concept up by KIND rather than by name is
	// what lets a school rename "Colegiatura" without the monthly run losing
	// track of what it bills. At most one concept per kind exists — the partial
	// unique index in migration 0004 is the backstop.
	EnsureConceptOfKind(ctx context.Context, id, kind, name string) (domain.ChargeConcept, error)

	// UpsertTuitionPlan stores the tuition table for one (cycle, level),
	// replacing whatever was there. Replacing and not versioning: the table is a
	// statement of current price, and the charges already issued carry their own
	// amount, so correcting a typo must not require a new cycle.
	UpsertTuitionPlan(ctx context.Context, p domain.TuitionPlan) (domain.TuitionPlan, error)
	// ListTuitionPlans returns the whole tuition table of a cycle.
	ListTuitionPlans(ctx context.Context, cycleID string) ([]domain.TuitionPlan, error)
}

// OverdueCursor is the keyset position of a page walk: the last (due_date, id)
// already consumed. Zero means "from the beginning". Keyset and not OFFSET
// because the billing run writes surcharges WHILE it reads, and OFFSET would
// skip or repeat rows as the result set shifts underneath it.
type OverdueCursor struct {
	DueDate time.Time
	ID      string
}

// IsZero reports the start-from-the-beginning position.
func (c OverdueCursor) IsZero() bool { return c.ID == "" && c.DueDate.IsZero() }

// ChargeReprice is one charge's new amount, named by its external ref.
type ChargeReprice struct {
	ExternalRef string
	AmountCents int64
}

// ChargeReschedule is one charge's new deadline, named by its external ref.
type ChargeReschedule struct {
	ExternalRef string
	DueOn       time.Time
}

// LevelArchiveRepository moves finance's rows of a deleted level — its tuition
// plans, and nothing else — into the service's deletion archive and back
// (ADR 0021). Money is never archived or altered: the preview COUNTS the
// level's pupils' charges and payments only so the director sees what stays.
// Archive and restore each run in ONE tenant-bound transaction and are
// idempotent per batch id; a restore skips — and reports — rows that now
// conflict.
type LevelArchiveRepository interface {
	PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error)
	ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error)
	PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
}
