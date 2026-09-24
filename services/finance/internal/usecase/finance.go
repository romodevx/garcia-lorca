package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

const (
	// dueDateLayout is the wire format for a charge's due date: a plain calendar
	// day, no time zone — a charge is due on a date, not at an instant.
	dueDateLayout = "2006-01-02"
	// enrollmentStatusActive is the only enrollment status a bulk generation
	// issues charges for.
	enrollmentStatusActive = "active"
)

// AccountStatement is a student's whole financial standing: every charge with
// what has been collected against it, every payment received, and the total
// still owed. TotalBalanceCents sums only what is genuinely outstanding —
// cancelled charges are excluded and an over-collected charge contributes zero,
// never a negative that would silently pay off another debt.
type AccountStatement struct {
	Charges           []ports.ChargeWithPaid
	Payments          []domain.Payment
	TotalBalanceCents int64
	// UnallocatedCents is money the family has handed over that no charge has
	// claimed: an overpayment, or a deposit made before the month was billed.
	// Without it on the statement the office cannot tell a credit from a
	// keying mistake, and the parent is the only one who knows it exists.
	UnallocatedCents int64
	// DueTodayCents is what is being asked for NOW: the months that have come
	// due, carrying the surcharge the daily rule has earned and net of the
	// early-payment reward.
	//
	// TotalBalanceCents above stays the plain arithmetic of charge minus paid —
	// what the YEAR still owes. Before this field the parents' app showed that
	// figure and the counter showed this one, and the two disagreed by design.
	DueTodayCents int64
	// The parts of DueTodayCents, so a family asking "why that number?" is
	// answered by the statement rather than by a phone call.
	SurchargeCents     int64
	EarlyDiscountCents int64
	// AsOf is the day the three figures above were worked out for, YYYY-MM-DD.
	// They move on their own overnight, and a screen that cannot say which day it
	// is quoting cannot be trusted the morning after.
	AsOf string
}

// FinanceService runs the debt-and-payment ledger: the charge catalog, the
// charges issued from it, the payments posted against them, and the account
// statement and overdue views built on top. It orchestrates four seams — the
// local FinanceRepository, the cross-service StudentReader (students, by RPC),
// CycleReader (config, by RPC) and EnrollmentReader (students, by RPC) — plus a
// best-effort PaymentPublisher. It depends only on ports + domain.
//
// Every cross-service reference is validated before anything is written, and a
// reference that does not resolve is reported as KindInvalid (bad input), never
// as a NotFound leak from another context.
//
//	Payment: build domain.Payment ─> repo.RegisterPayment(payment, derive)
//	         └─ derive runs INSIDE the tx, under the charge's row lock, and
//	            vetoes overpayment or paying a cancelled charge.
type FinanceService struct {
	repo        ports.FinanceRepository
	students    ports.StudentReader
	cycles      ports.CycleReader
	enrollments ports.EnrollmentReader
	catalog     ports.CatalogReader
	// calendar tells the collection rules which days the school is shut. It may
	// be nil: a service wired without one counts weekends as non-working and
	// nothing else, which is what every caller got before M16.
	calendar  ports.CalendarReader
	publisher ports.PaymentPublisher
	tenantID  string
	newID     func() string
	now       func() time.Time
}

// NewFinanceService wires the use case. tenantID is the single-tenant scope for
// this phase; newID mints ids and now stamps payments and bounds the overdue
// query (both injectable for deterministic tests). publisher may be nil, in
// which case events are simply not emitted.
func NewFinanceService(
	repo ports.FinanceRepository,
	students ports.StudentReader,
	cycles ports.CycleReader,
	enrollments ports.EnrollmentReader,
	catalog ports.CatalogReader,
	calendar ports.CalendarReader,
	publisher ports.PaymentPublisher,
	tenantID string,
	newID func() string,
	now func() time.Time,
) *FinanceService {
	return &FinanceService{
		repo:        repo,
		students:    students,
		cycles:      cycles,
		enrollments: enrollments,
		catalog:     catalog,
		calendar:    calendar,
		publisher:   publisher,
		tenantID:    tenantID,
		newID:       newID,
		now:         now,
	}
}

// CreateConcept adds an entry to the tenant's charge catalog, active from the
// start. A duplicate name is reported as KindConflict by the repository.
func (s *FinanceService) CreateConcept(ctx context.Context, name string, defaultAmountCents int64, recurrence string) (domain.ChargeConcept, error) {
	concept, err := domain.NewChargeConcept(s.newID(), s.tenantID, name, defaultAmountCents, recurrence, true)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return s.repo.CreateConcept(ctx, concept)
}

// ListConcepts returns the catalog; deactivated entries appear only when
// includeInactive is set.
func (s *FinanceService) ListConcepts(ctx context.Context, includeInactive bool) ([]domain.ChargeConcept, error) {
	return s.repo.ListConcepts(ctx, includeInactive)
}

// UpdateConcept re-validates and persists the mutable attributes of a catalog
// entry, leaving its identity — and every charge already issued from it —
// untouched. Setting active to false is how a concept is retired; it is never
// deleted.
func (s *FinanceService) UpdateConcept(ctx context.Context, id, name string, defaultAmountCents int64, recurrence string, active bool) (domain.ChargeConcept, error) {
	concept, err := s.requireConcept(ctx, id)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	updated, err := concept.WithUpdate(name, defaultAmountCents, recurrence, active)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return s.repo.UpdateConcept(ctx, updated)
}

// CreateCharge issues one charge to one student. In order: the student must
// exist (students RPC), the cycle must exist (config RPC), and the concept must
// exist in the catalog AND still be active — a retired concept is out of
// billing, not merely hidden. The amount and description default to the concept's,
// which lets the caller override either without touching the catalog; both are
// snapshotted onto the charge, so editing the catalog later never rewrites an
// issued charge. A student may owe a given concept once per cycle — a repeat is
// KindConflict.
// CreateChargeInput is what it takes to issue one charge. It is a struct rather
// than a parameter list because the optional half — an amount that overrides the
// concept's, a description that overrides its name, a reference to the row in
// another module that caused the charge — is easy to mis-order positionally and
// each field is worth naming at the call site.
type CreateChargeInput struct {
	StudentID string
	CycleID   string
	ConceptID string
	DueDate   string
	// AmountCentsOverride replaces the concept's default amount; zero keeps it.
	AmountCentsOverride int64
	// DescriptionOverride replaces the concept's name; empty keeps it.
	DescriptionOverride string
	// ExternalRef ties the charge to the row that caused it in another module.
	// Empty means this is the school's own charge and the "one concept per
	// cycle" rule applies to it.
	ExternalRef string
}

func (s *FinanceService) CreateCharge(ctx context.Context, in CreateChargeInput) (ports.ChargeWithPaid, error) {
	if err := s.requireStudent(ctx, in.StudentID); err != nil {
		return ports.ChargeWithPaid{}, err
	}
	if err := s.requireCycle(ctx, in.CycleID); err != nil {
		return ports.ChargeWithPaid{}, err
	}
	concept, err := s.requireActiveConcept(ctx, in.ConceptID)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	due, err := parseDueDate(in.DueDate)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}

	charge, err := domain.NewCharge(s.newID(), s.tenantID, in.StudentID, in.CycleID, in.ConceptID,
		resolveDescription(concept, in.DescriptionOverride), resolveAmount(concept, in.AmountCentsOverride), 0, due)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	if in.ExternalRef != "" {
		charge, err = charge.WithExternalRef(in.ExternalRef)
		if err != nil {
			return ports.ChargeWithPaid{}, err
		}
	}
	created, err := s.repo.CreateCharge(ctx, charge)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return ports.ChargeWithPaid{Charge: created, PaidCents: 0}, nil
}

// GenerateCharges issues the same concept to every ACTIVE enrollment of a cycle
// (or of one group within it) — how a school bills a month of tuition in one
// action. The concept itself must still be active. The write is a single bulk insert that skips students who already owe
// this concept for the cycle, so re-running a generation is idempotent: created
// counts the rows written and skipped the ones already there.
func (s *FinanceService) GenerateCharges(ctx context.Context, conceptID, cycleID, groupID string, amountCentsOverride int64, dueDate string) (created, skipped int, err error) {
	if err := s.requireCycle(ctx, cycleID); err != nil {
		return 0, 0, err
	}
	concept, err := s.requireActiveConcept(ctx, conceptID)
	if err != nil {
		return 0, 0, err
	}
	due, err := parseDueDate(dueDate)
	if err != nil {
		return 0, 0, err
	}

	enrollments, err := s.enrollments.ListByCycle(ctx, cycleID, groupID)
	if err != nil {
		return 0, 0, err
	}

	amount := resolveAmount(concept, amountCentsOverride)
	description := resolveDescription(concept, "")
	charges := make([]domain.Charge, 0, len(enrollments))
	for _, enr := range enrollments {
		if enr.Status != enrollmentStatusActive {
			continue
		}
		charge, err := domain.NewCharge(s.newID(), s.tenantID, enr.StudentID, cycleID, conceptID, description, amount, 0, due)
		if err != nil {
			return 0, 0, err
		}
		charges = append(charges, charge)
	}

	created, err = s.repo.CreateChargesBulk(ctx, charges)
	if err != nil {
		return 0, 0, err
	}
	return created, len(charges) - created, nil
}

// ListCharges returns charges with what has been collected against each,
// narrowed by student and/or cycle (either filter alone is enough; an empty one
// means "any"). At least one is REQUIRED: an unscoped read would hand the caller
// the tenant's whole ledger, so it is refused as bad input rather than served.
// The collected total is resolved by the repository in one query, so listing a
// cycle never degenerates into a payment lookup per charge. The repository caps
// the result — see its documented bound.
func (s *FinanceService) ListCharges(ctx context.Context, studentID, cycleID string) ([]ports.ChargeWithPaid, error) {
	if studentID == "" && cycleID == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "specify a student or a cycle")
	}
	return s.repo.ListCharges(ctx, studentID, cycleID)
}

// ApplyDiscount grants a scholarship or courtesy on an issued charge. The
// discount replaces any previous one and may not exceed the gross amount; the
// status is re-derived from what has already been collected, so a discount that
// covers the remaining balance settles the charge outright. A cancelled charge
// owes nothing and cannot be discounted.
func (s *FinanceService) ApplyDiscount(ctx context.Context, chargeID string, discountCents int64, reason string) (ports.ChargeWithPaid, error) {
	current, err := s.requireCharge(ctx, chargeID)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	if current.Charge.Status() == domain.StatusCancelled {
		return ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "cannot discount a cancelled charge")
	}

	discounted, err := current.Charge.WithDiscount(discountCents, current.PaidCents, reason)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	saved, err := s.repo.SaveCharge(ctx, discounted)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return ports.ChargeWithPaid{Charge: saved, PaidCents: current.PaidCents}, nil
}

// CancelCharge withdraws a charge. Cancellation is terminal: cancelling twice
// is KindConflict. Payments already collected are left on the ledger — settling
// them (refund or credit) belongs to a later phase, not to a status change.
func (s *FinanceService) CancelCharge(ctx context.Context, chargeID string) (ports.ChargeWithPaid, error) {
	current, err := s.requireCharge(ctx, chargeID)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	cancelled, err := current.Charge.Cancel()
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	saved, err := s.repo.SaveCharge(ctx, cancelled)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return ports.ChargeWithPaid{Charge: saved, PaidCents: current.PaidCents}, nil
}

// CancelChargeIfUnpaid voids a charge only when nothing has been collected
// against it. The decision belongs to the repository, not here: it has to be
// made under the charge's row lock, and reading the paid total in the use case
// and writing afterwards would reopen the race it exists to close — the expiry
// sweep of another module voiding the charge a cashier is collecting right now.
func (s *FinanceService) CancelChargeIfUnpaid(ctx context.Context, chargeID string) (ports.ChargeWithPaid, error) {
	return s.repo.CancelChargeIfUnpaid(ctx, chargeID)
}

// RegisterPayment posts money against a charge. The rules that depend on what
// has already been collected — no overpayment, no paying a cancelled charge —
// are handed to the repository as a callback so they run inside the transaction
// under the charge's row lock; two cashiers taking the last peso at once cannot
// both succeed. The PaymentRegistered event is published afterwards and
// best-effort: the money is already banked, so a broker outage must not fail
// the call.
//
// idempotencyKey is the caller's optional handle on the attempt: a repeat under
// the same key returns the payment already stored (same response, no second
// charge) instead of collecting again — the retry a lost response provokes must
// not cost the family twice. Because such a replay collected nothing, it
// publishes nothing either: the event fired the first time round, and a second
// one would bill a second receipt for one payment. An empty key opts out.
func (s *FinanceService) RegisterPayment(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string) (domain.Payment, ports.ChargeWithPaid, error) {
	return s.RegisterPaymentAt(ctx, chargeID, amountCents, method, reference, idempotencyKey, time.Time{})
}

// RegisterPaymentAt is RegisterPayment with the date the money actually changed
// hands. A zero paidAt means "now", which is what every existing caller gets.
//
// The date matters more than it looks. Every rule the school asked for — the
// early-payment discount, the daily surcharge, which months a deposit covers —
// is a function of WHEN the family paid, and until now there was no way to say:
// the service stamped its own clock, so a cashier catching up on Monday stamped
// Monday on Friday's cash and every rule downstream used the wrong day.
//
// A future date is refused. It is the classic counter typo (2027 for 2026) and
// it silently lifts a payment out of every overdue calculation, which is a
// mistake nobody notices until a parent is wrongly told they are current.
func (s *FinanceService) RegisterPaymentAt(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time) (domain.Payment, ports.ChargeWithPaid, error) {
	when := paidAt
	if when.IsZero() {
		when = s.now()
	} else if when.After(s.now()) {
		return domain.Payment{}, ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid,
			"la fecha del pago no puede estar en el futuro")
	}

	payment, err := domain.NewPayment(s.newID(), s.tenantID, chargeID, amountCents, method, reference, when, idempotencyKey)
	if err != nil {
		return domain.Payment{}, ports.ChargeWithPaid{}, err
	}

	stored, charge, replayed, err := s.repo.RegisterPayment(ctx, payment, derivePaidStatus,
		func(charge ports.ChargeWithPaid) []ports.OutboxEvent {
			return s.paymentEvents(payment, charge)
		})
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.Payment{}, ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "charge not found")
		}
		return domain.Payment{}, ports.ChargeWithPaid{}, err
	}

	if !replayed {
		// The events are already durable; this only gets them out now instead of
		// on the next drain. It cannot fail the payment — that is the difference
		// between a queue and a best-effort send.
		s.drainQuietly(ctx)
	}
	return stored, charge, nil
}

// derivePaidStatus is the rule the repository runs inside the payment
// transaction: it vetoes a payment against a cancelled charge or one that would
// collect more than is owed, and otherwise reports the charge's new status.
func derivePaidStatus(charge domain.Charge, newPaidCents int64) (string, error) {
	if charge.Status() == domain.StatusCancelled {
		return "", glerrors.New(glerrors.KindInvalid, "cannot pay a cancelled charge")
	}
	if newPaidCents > charge.Effective().Int64() {
		return "", glerrors.New(glerrors.KindInvalid, "payment exceeds the remaining balance")
	}
	return charge.DeriveStatus(newPaidCents), nil
}

// paymentEvents is what a collected payment announces, queued INSIDE the
// payment transaction so it cannot be lost between the commit and the send.
//
// ChargeSettled fires on the collection that CLOSES the charge and on no other:
// a consumer acting on a partial payment would enrol a pupil whose inscription
// is half paid. The test is the charge's derived status, not the amount
// collected, which says nothing about what was owed.
func (s *FinanceService) paymentEvents(p domain.Payment, charge ports.ChargeWithPaid) []ports.OutboxEvent {
	events := []ports.OutboxEvent{
		outboxEvent(ports.OutboxPaymentRegistered, ports.PaymentRegisteredEvent{
			PaymentID:   p.ID(),
			StudentID:   charge.Charge.StudentID(),
			ChargeID:    p.ChargeID(),
			TenantID:    s.tenantID,
			Method:      p.Method(),
			AmountCents: p.Amount().Int64(),
			PaidAt:      p.PaidAt(),
		}),
	}
	if charge.Charge.Status() == domain.StatusPaid {
		events = append(events, outboxEvent(ports.OutboxChargeSettled, ports.ChargeSettledEvent{
			ChargeID:    p.ChargeID(),
			StudentID:   charge.Charge.StudentID(),
			TenantID:    s.tenantID,
			ExternalRef: charge.Charge.ExternalRef(),
			AmountCents: charge.Charge.Effective().Int64(),
			SettledAt:   p.PaidAt(),
		}))
	}
	return events
}

// outboxEvent serialises a domain event for the queue. A payload that cannot be
// marshalled would be a silently dropped event, so it becomes an empty one the
// drain refuses loudly rather than a row nobody notices.
func outboxEvent(eventType string, payload any) ports.OutboxEvent {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ports.OutboxEvent{Type: eventType}
	}
	return ports.OutboxEvent{Type: eventType, Payload: raw}
}

// DrainOutbox publishes what is waiting. It is called right after every payment
// so the ordinary case is instant, and by a scheduler so nothing stays stuck
// when the broker was down or the process died.
func (s *FinanceService) DrainOutbox(ctx context.Context, limit int) (int, error) {
	if s.publisher == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = drainBatch
	}
	return s.repo.DrainOutbox(ctx, limit, func(e ports.OutboxEvent) error {
		return s.publishWithin(ctx, e)
	})
}

// publishWithin turns one queued row back into a typed publication.
//
// An unknown type is refused rather than dropped: the row stays, and an attempt
// count that climbs is how somebody finds out this service was deployed emitting
// something nothing consumes.
func (s *FinanceService) publishWithin(ctx context.Context, e ports.OutboxEvent) error {
	switch e.Type {
	case ports.OutboxPaymentRegistered:
		var ev ports.PaymentRegisteredEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return err
		}
		return s.publisher.PublishPaymentRegistered(ctx, ev)
	case ports.OutboxChargeSettled:
		var ev ports.ChargeSettledEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return err
		}
		return s.publisher.PublishChargeSettled(ctx, ev)
	default:
		return glerrors.New(glerrors.KindInternal, "unknown outbox event: "+e.Type)
	}
}

// drainQuietly gets the just-queued events out now, under a hard deadline.
//
// The deadline is the whole point. A broker that is down must never turn into a
// payment that was refused — and it must not turn into a payment that HANGS
// either, which is what happened the first time this was written without one:
// the Pub/Sub client retried a dead emulator and the counter froze with a parent
// standing there. A second is plenty for a healthy publish and invisible when it
// fails; whatever misses the deadline is still queued, and the sweep gets it.
//
// Detached from the caller's context so a client that hangs up does not cancel
// a publication already in flight.
func (s *FinanceService) drainQuietly(ctx context.Context) {
	if s.publisher == nil {
		return
	}
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainDeadline)
	defer cancel()
	_, _ = s.repo.DrainOutbox(drainCtx, drainBatch, func(e ports.OutboxEvent) error {
		return s.publishWithin(drainCtx, e)
	})
}

// drainDeadline bounds the drain that runs on the payment path. See above: a
// healthy publish takes milliseconds, and a dead broker must cost the cashier a
// blink rather than a wait.
const drainDeadline = time.Second

// drainBatch is how many events one drain publishes. Small enough that a
// counter-side drain never becomes a wait, large enough that a backlog clears in
// a few passes.
const drainBatch = 50

// GetAccount builds a student's account statement: every charge across every
// cycle with what has been collected against it, every payment received, and
// the total still owed.
func (s *FinanceService) GetAccount(ctx context.Context, studentID string) (AccountStatement, error) {
	charges, err := s.repo.ListCharges(ctx, studentID, "")
	if err != nil {
		return AccountStatement{}, err
	}
	payments, err := s.repo.ListPayments(ctx, studentID)
	if err != nil {
		return AccountStatement{}, err
	}

	credit, err := s.repo.UnallocatedCents(ctx, studentID)
	if err != nil {
		return AccountStatement{}, err
	}

	var balance int64
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled {
			continue
		}
		// Floored at zero: an over-collected charge is a credit to reconcile,
		// not a discount on the student's other debts.
		balance += max(0, c.Charge.Effective().Int64()-c.PaidCents)
	}
	// What the counter would collect today, through the SAME arithmetic the
	// counter uses — but against the charges ALREADY read above, and without the
	// two idempotent writes EnsureConceptOfKind performs. A statement is a read,
	// and this is the endpoint the parents' app hits most.
	when := s.now()
	expected, err := s.expectedForStatement(ctx, studentID, charges, when)
	if err != nil {
		return AccountStatement{}, err
	}
	return AccountStatement{
		Charges:            charges,
		Payments:           payments,
		TotalBalanceCents:  balance,
		UnallocatedCents:   credit,
		DueTodayCents:      expected.AmountCents,
		SurchargeCents:     expected.SurchargeCents,
		EarlyDiscountCents: expected.DiscountCents,
		AsOf:               when.Format(dueDateLayout),
	}, nil
}

// expectedForStatement is what the counter would collect today, worked out from
// charges the caller has already loaded.
//
// It resolves the concepts by READING the catalog rather than ensuring them:
// a statement must not write, and by the time a family has charges at all, the
// concepts those charges point at necessarily exist. A school with no concepts
// yet has no charges either, and the arithmetic over an empty list is zero
// whichever id it was given.
func (s *FinanceService) expectedForStatement(
	ctx context.Context, studentID string, charges []ports.ChargeWithPaid, when time.Time,
) (Expected, error) {
	calendar, _ := s.schoolCalendar(ctx, studentID)
	plan, err := s.planFor(ctx, studentID)
	if err != nil {
		return Expected{}, err
	}
	byKind, err := s.repo.ListConcepts(ctx, true)
	if err != nil {
		return Expected{}, err
	}
	var lateFeeID, tuitionID, enrolmentID string
	for _, c := range byKind {
		switch c.Kind() {
		case domain.KindLateFee:
			lateFeeID = c.ID()
		case domain.KindTuition:
			tuitionID = c.ID()
		case domain.KindEnrollment:
			enrolmentID = c.ID()
		}
	}
	return expectedFrom(charges, plan, calendar, lateFeeID, tuitionID, enrolmentID, when, when)
}

// ListOverdue returns the charges past due that still owe money, as of the
// injected clock. An empty cycleID spans every cycle.
func (s *FinanceService) ListOverdue(ctx context.Context, cycleID string) ([]ports.ChargeWithPaid, error) {
	return s.repo.ListOverdue(ctx, cycleID, s.now())
}

// requireStudent asserts the student exists, mapping a NotFound to KindInvalid
// (a bad client reference) and propagating any other error.
func (s *FinanceService) requireStudent(ctx context.Context, studentID string) error {
	if _, err := s.students.ByID(ctx, studentID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "student not found")
		}
		return err
	}
	return nil
}

// requireCycle asserts the school cycle exists, mapping a NotFound to KindInvalid.
func (s *FinanceService) requireCycle(ctx context.Context, cycleID string) error {
	if _, err := s.cycles.ByID(ctx, cycleID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "cycle not found")
		}
		return err
	}
	return nil
}

// requireConcept loads a catalog entry, mapping a NotFound to KindInvalid.
func (s *FinanceService) requireConcept(ctx context.Context, conceptID string) (domain.ChargeConcept, error) {
	concept, err := s.repo.GetConcept(ctx, conceptID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.ChargeConcept{}, glerrors.New(glerrors.KindInvalid, "concept not found")
		}
		return domain.ChargeConcept{}, err
	}
	return concept, nil
}

// requireActiveConcept loads a catalog entry that may still issue charges. The
// active check lives here rather than in requireConcept because retirement must
// stay reversible: UpdateConcept has to load an inactive concept to revive it.
func (s *FinanceService) requireActiveConcept(ctx context.Context, conceptID string) (domain.ChargeConcept, error) {
	concept, err := s.requireConcept(ctx, conceptID)
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	if !concept.Active() {
		return domain.ChargeConcept{}, glerrors.New(glerrors.KindInvalid, "concept is inactive")
	}
	return concept, nil
}

// requireCharge loads a charge with its collected total, mapping a NotFound to
// KindInvalid.
func (s *FinanceService) requireCharge(ctx context.Context, chargeID string) (ports.ChargeWithPaid, error) {
	charge, err := s.repo.GetCharge(ctx, chargeID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "charge not found")
		}
		return ports.ChargeWithPaid{}, err
	}
	return charge, nil
}

// parseDueDate reads the wire format for a due date, reporting anything else as
// KindInvalid rather than leaking the parser's message.
func parseDueDate(s string) (time.Time, error) {
	due, err := time.Parse(dueDateLayout, s)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "invalid due date (want YYYY-MM-DD)")
	}
	return due, nil
}

// resolveAmount picks the caller's override when there is one, else the
// concept's default. A charge for no money is expressed by a zero-default
// concept, never by an override, so a non-positive override means "no override".
func resolveAmount(c domain.ChargeConcept, override int64) int64 {
	if override > 0 {
		return override
	}
	return c.DefaultAmount().Int64()
}

// resolveDescription picks the caller's override when there is one, else the
// concept's name.
func resolveDescription(c domain.ChargeConcept, override string) string {
	if strings.TrimSpace(override) == "" {
		return c.Name()
	}
	return override
}
