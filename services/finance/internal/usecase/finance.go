package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/schoolcal"
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
// what has been collected against it, every payment received, and the balance.
type AccountStatement struct {
	// Charges are priced as of AsOf: a scholarship's part included (T18).
	Charges []ports.ChargeWithPaid
	// raw are the same charges as stored, for the ledger (T7), which prices
	// each one itself after the inscription ladder.
	raw []ports.ChargeWithPaid
	// deposits are the student's, read once for the balance and reused by the
	// statement's lines.
	deposits []domain.Deposit
	Payments []domain.Payment
	// TotalBalanceCents is THE balance (T5 · ISSUE-006), the statement's
	// Totals.BalanceCents to the cent: opened, non-cancelled charges at the
	// counter's price (inscription ladder, scholarship), plus the surcharge
	// the daily rule has earned up to AsOf, less discounts and every live
	// payment and credit note. Negative is money the school holds for the
	// family (unallocated credit, an over-collected charge).
	//
	// A charge that has not opened (T5 window) is NOT in it, for the office
	// as for the family: the office's Charges list it, the balance does not,
	// so the drawer, the parents' app, the PDF and the counter read one number.
	// Surcharges of a cancelled month owe nothing and are not in it either.
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
	// TotalBalanceCents above is what the YEAR still owes; this is the part of
	// it asked for today (net of the early-payment reward, which is not in the
	// balance until a payment earns it).
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
	// activities is T17's (WithActivities); nil bills no activity.
	activities ports.ActivityRepository
	// scholarships is T18's (WithScholarships); nil prices no scholarship.
	scholarships ports.ScholarshipRepository
	tenantID     string
	newID        func() string
	// clock is the one source of time (ADR 0035): Now stamps, Today decides.
	clock SchoolClock
}

// NewFinanceService wires the use case. tenantID is the single-tenant scope for
// this phase; newID mints ids and clock stamps payments (Now) and
// decides what is due on the school's day (Today) (both injectable for deterministic tests). publisher may be nil, in
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
	clock SchoolClock,
) *FinanceService {
	return &FinanceService{
		repo:        repo,
		students:    students,
		cycles:      cycles,
		enrollments: schoolDayEnrollments{inner: enrollments, clock: clock},
		catalog:     catalog,
		calendar:    calendar,
		publisher:   publisher,
		tenantID:    tenantID,
		newID:       newID,
		clock:       clock,
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
	// A sale to a pupil not yet placed in a group names no cycle (task 10); the
	// domain lets it through only with the sale's ref.
	cycleFree := strings.TrimSpace(in.CycleID) == "" && strings.TrimSpace(in.ExternalRef) != ""
	if !cycleFree {
		if err := s.requireCycle(ctx, in.CycleID); err != nil {
			return ports.ChargeWithPaid{}, err
		}
	}
	concept, err := s.requireActiveConcept(ctx, in.ConceptID)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	due, err := parseDueDate(in.DueDate)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}

	charge, err := s.newChargeFor(in, concept, due, cycleFree)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	created, err := s.repo.CreateCharge(ctx, charge)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return ports.ChargeWithPaid{Charge: created, PaidCents: 0}, nil
}

// newChargeFor builds the charge CreateCharge stores: a cycle-free sale charge,
// or the school's own, carrying the ref when there is one.
func (s *FinanceService) newChargeFor(in CreateChargeInput, concept domain.ChargeConcept, due time.Time, cycleFree bool) (domain.Charge, error) {
	description, amount := resolveDescription(concept, in.DescriptionOverride), resolveAmount(concept, in.AmountCentsOverride)
	if cycleFree {
		return domain.NewSaleCharge(s.newID(), s.tenantID, in.StudentID, in.ConceptID, description, amount, due, in.ExternalRef)
	}
	charge, err := domain.NewCharge(s.newID(), s.tenantID, in.StudentID, in.CycleID, in.ConceptID, description, amount, 0, due)
	if err != nil || in.ExternalRef == "" {
		return charge, err
	}
	return charge.WithExternalRef(in.ExternalRef)
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

// studentsOf is the one pupil asked about, or every pupil in the list.
func studentsOf(studentID string, charges []ports.ChargeWithPaid) []string {
	if studentID != "" {
		return []string{studentID}
	}
	seen := map[string]bool{}
	// Never nil: nil asks for every pupil's scholarships, and no charge means
	// no pupil to price.
	out := []string{}
	for _, c := range charges {
		if id := c.Charge.StudentID(); !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
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

// maxCancellationReasonLen bounds the free text a cancellation keeps.
const maxCancellationReasonLen = 500

// CancelCharge withdraws a charge, recording why. Cancellation is terminal:
// cancelling twice is KindConflict. The reason is required (CC-203): a debt
// that disappears without one cannot be explained to the family. Payments
// already collected are left on the ledger — settling them (refund or credit)
// belongs to a later phase, not to a status change.
func (s *FinanceService) CancelCharge(ctx context.Context, chargeID, reason string) (ports.ChargeWithPaid, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "a reason is required to cancel a charge")
	}
	if utf8.RuneCountInString(reason) > maxCancellationReasonLen {
		return ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "the cancellation reason is too long")
	}
	current, err := s.requireCharge(ctx, chargeID)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	cancelled, err := current.Charge.Cancel()
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	saved, err := s.repo.SaveCharge(ctx, cancelled.WithCancellationReason(reason))
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
	return s.RegisterPaymentAt(ctx, chargeID, amountCents, method, reference, idempotencyKey, time.Time{}, "")
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
//
// receivedBy is the account that took the money (task 7), set by the gateway
// from the authenticated caller; empty when unknown.
func (s *FinanceService) RegisterPaymentAt(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, receivedBy string) (domain.Payment, ports.ChargeWithPaid, error) {
	payments, charge, err := s.RegisterTenderedPayment(ctx, TenderedPaymentInput{
		ChargeID: chargeID, AmountCents: amountCents, IdempotencyKey: idempotencyKey,
		PaidAt: paidAt, ReceivedBy: receivedBy,
		Tenders: []TenderInput{{Method: method, AmountCents: amountCents, Reference: reference}},
	})
	if err != nil {
		return domain.Payment{}, ports.ChargeWithPaid{}, err
	}
	return payments[0], charge, nil
}

// TenderedPaymentInput is a payment against one charge that may combine
// methods (T8): a credit note completed with cash, say.
type TenderedPaymentInput struct {
	ChargeID    string
	AmountCents int64
	// Tenders must add up exactly to AmountCents — the school's rule: a payment
	// whose methods do not cover the amount to pay is not registered.
	Tenders        []TenderInput
	IdempotencyKey string
	// PaidAt is when the money changed hands; zero means now.
	PaidAt time.Time
	// ReceivedBy is the account that took the money (task 7).
	ReceivedBy string
}

// RegisterTenderedPayment posts one payment row per tender against a charge,
// all in ONE transaction under the charge's lock: the overpay veto sees their
// sum, and either every row lands or none does.
//
// Idempotency holds for the whole set. The first row carries the caller's key
// and each further row the key with "#2", "#3"… appended, so a retry finds the
// set already stored and returns it without collecting again.
func (s *FinanceService) RegisterTenderedPayment(ctx context.Context, in TenderedPaymentInput) ([]domain.Payment, ports.ChargeWithPaid, error) {
	when, err := s.paymentDate(in.PaidAt)
	if err != nil {
		return nil, ports.ChargeWithPaid{}, err
	}
	payments, err := s.tenderedPayments(in, when)
	if err != nil {
		return nil, ports.ChargeWithPaid{}, err
	}
	if err := s.refuseScholarshipCharge(ctx, in.ChargeID, when.on); err != nil {
		return nil, ports.ChargeWithPaid{}, err
	}

	stored, charge, replayed, err := s.repo.RegisterPayment(ctx, payments, derivePaidStatus,
		func(charge ports.ChargeWithPaid) []ports.OutboxEvent {
			return s.paymentEvents(payments, charge)
		})
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return nil, ports.ChargeWithPaid{}, glerrors.New(glerrors.KindInvalid, "charge not found")
		}
		return nil, ports.ChargeWithPaid{}, err
	}

	if !replayed {
		// The events are already durable; this only gets them out now instead of
		// on the next drain. It cannot fail the payment — that is the difference
		// between a queue and a best-effort send.
		s.drainQuietly(ctx)
	}
	return stored, charge, nil
}

// errFuturePayment refuses a payment dated after it was taken.
var errFuturePayment = glerrors.New(glerrors.KindInvalid, "la fecha del pago no puede estar en el futuro")

// earliestPaymentDay is the first day a payment may be dated. The folio
// counter only numbers years 2000..3000 (migration 0018), so a mistyped year
// at the counter (0202, 1999) must be refused here as invalid input instead of
// reaching the counter and failing as an internal error.
var earliestPaymentDay = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// errPaymentTooOld refuses a payment dated before earliestPaymentDay.
var errPaymentTooOld = glerrors.New(glerrors.KindInvalid, "la fecha del pago es anterior al año 2000; revisa el año")

// paymentDay is when a payment happened, in the two senses ADR 0035 keeps
// apart: the instant it was recorded (paid_at, audit only) and the
// school-local civil day it is judged on (paid_on, what every rule reads).
type paymentDay struct {
	at time.Time
	on time.Time
}

// paymentDate turns the caller's date into a paymentDay.
//
// Unset means now: recorded at the instant, dated the school's today — so a
// family paying at 19:00 in Morelia on the last day of the early window is
// dated that day, and stays dated that day in paid_by_due, the cash cut, the
// statement and the receipt. A date the caller gave is the day picked at the
// counter, which arrives as midnight UTC of that day: it is that day. The
// "no future date" rule compares DAYS with the school's today, so picking
// tomorrow is refused at any hour of the local evening; a day before
// earliestPaymentDay is refused too.
func (s *FinanceService) paymentDate(paidAt time.Time) (paymentDay, error) {
	today := s.clock.Today()
	if paidAt.IsZero() {
		return paymentDay{at: s.clock.Now(), on: today}, nil
	}
	day := domain.CalendarDay(paidAt)
	if day.After(today) {
		return paymentDay{}, errFuturePayment
	}
	if day.Before(earliestPaymentDay) {
		return paymentDay{}, errPaymentTooOld
	}
	return paymentDay{at: paidAt, on: day}, nil
}

// tenderedPayments builds one validated payment per tender, after checking
// that the tenders add up to the amount being paid.
func (s *FinanceService) tenderedPayments(in TenderedPaymentInput, when paymentDay) ([]domain.Payment, error) {
	if in.AmountCents <= 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "payment amount must be positive")
	}
	tenders, err := toTenders(in.Tenders)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateTenders(tenders, in.AmountCents); err != nil {
		return nil, err
	}
	out := make([]domain.Payment, 0, len(tenders))
	for i, t := range tenders {
		payment, err := domain.NewPayment(s.newID(), s.tenantID, in.ChargeID, t.Amount().Int64(),
			t.Method(), t.Reference(), when.at, domain.TenderKey(in.IdempotencyKey, i))
		if err != nil {
			return nil, err
		}
		payment = payment.WithPaidOn(when.on)
		if payment, err = payment.WithCardType(t.CardType()); err != nil {
			return nil, err
		}
		out = append(out, payment.WithReceivedBy(in.ReceivedBy))
	}
	return out, nil
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
func (s *FinanceService) paymentEvents(payments []domain.Payment, charge ports.ChargeWithPaid) []ports.OutboxEvent {
	first := payments[0]
	var total int64
	method := first.Method()
	for _, p := range payments {
		total += p.Amount().Int64()
		if p.Method() != method {
			method = domain.MethodMixed
		}
	}
	// One announcement per payment the family made, whatever the methods it
	// combined: a parent is told once that $1,000 arrived, not once per tender.
	events := []ports.OutboxEvent{
		outboxEvent(ports.OutboxPaymentRegistered, ports.PaymentRegisteredEvent{
			PaymentID:   first.ID(),
			StudentID:   charge.Charge.StudentID(),
			ChargeID:    first.ChargeID(),
			TenantID:    s.tenantID,
			Method:      method,
			AmountCents: total,
			PaidAt:      first.PaidAt(),
		}),
	}
	if charge.Charge.Status() == domain.StatusPaid {
		events = append(events, outboxEvent(ports.OutboxChargeSettled, ports.ChargeSettledEvent{
			ChargeID:    first.ChargeID(),
			StudentID:   charge.Charge.StudentID(),
			TenantID:    s.tenantID,
			ExternalRef: charge.Charge.ExternalRef(),
			AmountCents: charge.Charge.Effective().Int64(),
			SettledAt:   first.PaidAt(),
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
	case ports.OutboxPaymentCorrected:
		var ev ports.PaymentCorrectedEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return err
		}
		return s.publisher.PublishPaymentCorrected(ctx, ev)
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
	return s.account(ctx, studentID, func(ports.ChargeWithPaid) bool { return true })
}

// GetFamilyAccount is the statement a family is shown (T5): the school's own,
// less the charges that have not opened yet. Balance and "due today" are worked
// out over what is left, so the app never asks for money it does not show.
//
// A charge that already has money on it stays, opened or not: the payment is in
// the family's history, and a payment whose charge is missing explains nothing.
func (s *FinanceService) GetFamilyAccount(ctx context.Context, studentID string) (AccountStatement, error) {
	return s.account(ctx, studentID, inPlayOn(s.clock.Today()))
}

// inPlayOn is the family's view of a charge on `now` (T5): opened, or carrying
// money already. The statement (T7) counts exactly these in the balance, for
// the office as for the family, so both read one number.
func inPlayOn(now time.Time) func(ports.ChargeWithPaid) bool {
	return func(c ports.ChargeWithPaid) bool {
		return c.PaidCents > 0 || c.Charge.OpenOn(now)
	}
}

// account is the statement over the charges `shown` keeps.
func (s *FinanceService) account(ctx context.Context, studentID string, shown func(ports.ChargeWithPaid) bool) (AccountStatement, error) {
	statement, _, err := s.accountWithTerms(ctx, studentID, shown, s.clock.Today())
	return statement, err
}

// accountWithTerms is account plus the collection terms it was worked out
// under, for the ledger (T7) to reuse without reading them twice.
func (s *FinanceService) accountWithTerms(
	ctx context.Context, studentID string, shown func(ports.ChargeWithPaid) bool, when time.Time,
) (AccountStatement, collectionTerms, error) {
	all, err := s.repo.ListCharges(ctx, studentID, "")
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	charges := make([]ports.ChargeWithPaid, 0, len(all))
	for _, c := range all {
		if shown(c) {
			charges = append(charges, c)
		}
	}
	payments, err := s.repo.ListPayments(ctx, studentID)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}

	credit, err := s.repo.UnallocatedCents(ctx, studentID)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	// The terms first: the scholarship's part depends on what each charge IS.
	terms, err := s.collectionTermsFor(ctx, studentID)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	pricer, err := s.pricerFor(ctx, []string{studentID}, terms.ids())
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	terms.scholarships = pricer
	priced, err := pricer.priceAll(charges, when)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}

	deposits, err := s.repo.ListDeposits(ctx, studentID)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	// The balance is the ledger's, never a second sum: see ledger.
	_, totals, err := ledger(charges, payments, deposits, terms, when, false)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	// What the counter would collect today, through the SAME arithmetic the
	// counter uses — but against the charges ALREADY read above, and without the
	// two idempotent writes EnsureConceptOfKind performs. A statement is a read,
	// and this is the endpoint the parents' app hits most.
	// The statement needs the totals, not the counter's list: no lines.
	expected, err := expectedFrom(charges, terms.plan, terms.calendar,
		terms.lateFeeID, terms.tuitionID, terms.enrolmentID, when, when, false, pricer)
	if err != nil {
		return AccountStatement{}, collectionTerms{}, err
	}
	return AccountStatement{
		raw:                charges,
		deposits:           deposits,
		Charges:            priced,
		Payments:           payments,
		TotalBalanceCents:  totals.BalanceCents,
		UnallocatedCents:   credit,
		DueTodayCents:      expected.AmountCents,
		SurchargeCents:     expected.SurchargeCents,
		EarlyDiscountCents: expected.DiscountCents,
		AsOf:               when.Format(dueDateLayout),
	}, terms, nil
}

// collectionTerms is what the counter's arithmetic needs besides the charges:
// the student's plan, the school calendar and the system concepts' ids.
type collectionTerms struct {
	plan                              domain.TuitionPlan
	calendar                          *schoolcal.Calendar
	lateFeeID, tuitionID, enrolmentID string
	// scholarships prices the pupil's charges (T18); nil prices none.
	scholarships *scholarshipPricer
}

// ids are the terms' system concepts.
func (t collectionTerms) ids() systemConceptIDs {
	return systemConceptIDs{tuition: t.tuitionID, enrolment: t.enrolmentID, lateFee: t.lateFeeID}
}

// collectionTermsFor reads the terms a statement is worked out under.
//
// It resolves the concepts by READING the catalog rather than ensuring them:
// a statement must not write, and by the time a family has charges at all, the
// concepts those charges point at necessarily exist. A school with no concepts
// yet has no charges either, and the arithmetic over an empty list is zero
// whichever id it was given.
func (s *FinanceService) collectionTermsFor(ctx context.Context, studentID string) (collectionTerms, error) {
	calendar, _ := s.schoolCalendar(ctx, studentID)
	plan, err := s.planFor(ctx, studentID)
	if err != nil {
		return collectionTerms{}, err
	}
	byKind, err := s.repo.ListConcepts(ctx, true)
	if err != nil {
		return collectionTerms{}, err
	}
	terms := collectionTerms{plan: plan, calendar: calendar}
	for _, c := range byKind {
		switch c.Kind() {
		case domain.KindLateFee:
			terms.lateFeeID = c.ID()
		case domain.KindTuition:
			terms.tuitionID = c.ID()
		case domain.KindEnrollment:
			terms.enrolmentID = c.ID()
		}
	}
	return terms, nil
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
