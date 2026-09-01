package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/schoolcal"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// DepositInput is a deposit as the counter reports it: who paid, how much, how,
// and when. What it covers is not asked for — that is the whole point.
type DepositInput struct {
	StudentID      string
	AmountCents    int64
	Method         string
	Reference      string
	PaidAt         time.Time
	IdempotencyKey string
}

// DepositResult is what the money did, in the shape a receipt needs.
type DepositResult struct {
	Deposit domain.Deposit
	// Surcharges are the late-fee charges this deposit made real. The daily rule
	// is derived and not billed, so a row exists only where money landed on it.
	Surcharges []domain.Charge
	// Allocations are the payments the deposit split into, oldest debt first.
	Allocations []domain.Payment
	// UnallocatedCents is the family's credit: money beyond what they owed.
	UnallocatedCents int64
	// Replayed says the deposit already existed under this idempotency key and
	// nothing was collected a second time.
	Replayed bool
}

// RegisterDeposit takes money without being told what it pays for.
//
// A parent hands over an amount and says nothing else. The office is expected to
// know it covers the oldest month still owed, the surcharge that month earned,
// and then the next. Asking a cashier to split that by hand is how a family ends
// up with October paid and September still running $10 a day.
//
// The split is decided INSIDE the payment transaction, with the student's
// charges locked. Planning outside and writing inside would reopen the race the
// lock exists to close: two cashiers taking money from the same family at once
// would each plan against the same balance and both allocate it.
//
// The date matters as much as the amount. Every rule the school asked for is a
// function of when the family paid, so a cashier catching up on Monday can stamp
// Friday. A future date is refused: it is the classic counter typo (2027 for
// 2026) and it silently lifts the money out of every overdue calculation.
func (s *FinanceService) RegisterDeposit(ctx context.Context, in DepositInput) (DepositResult, error) {
	deposit, planner, err := s.prepareDeposit(ctx, in)
	if err != nil {
		return DepositResult{}, err
	}

	record, replayed, err := s.repo.RegisterDeposit(ctx, deposit, planner.plan)
	if err != nil {
		return DepositResult{}, err
	}
	if !replayed {
		s.announceDeposit(ctx, record)
	}
	return DepositResult{
		Deposit:          record.Deposit,
		Surcharges:       record.Surcharges,
		Allocations:      record.Allocations,
		UnallocatedCents: record.UnallocatedCents,
		Replayed:         replayed,
	}, nil
}

// PreviewDeposit says what an amount would cover, without taking it.
//
// A cashier has to be able to tell a parent what their money pays for BEFORE
// taking it. It runs the same split as the real thing — same rule, same order,
// same surcharge arithmetic — and writes nothing.
//
// It is a projection, not a promise. The charges are read without the lock the
// real call takes, so a payment landing between the preview and the deposit
// changes the answer. That is the correct trade: holding a row lock open while
// a person decides would block the counter.
func (s *FinanceService) PreviewDeposit(ctx context.Context, in DepositInput) (DepositResult, error) {
	deposit, planner, err := s.prepareDeposit(ctx, in)
	if err != nil {
		return DepositResult{}, err
	}
	open, err := s.repo.ListCharges(ctx, in.StudentID, "")
	if err != nil {
		return DepositResult{}, err
	}
	write, err := planner.plan(open)
	if err != nil {
		return DepositResult{}, err
	}
	return DepositResult{
		Deposit:          deposit,
		Surcharges:       write.Surcharges,
		Allocations:      write.Allocations,
		UnallocatedCents: write.UnallocatedCents,
	}, nil
}

// prepareDeposit builds the deposit and the planner both paths need: the date
// rules, the student, the calendar, the collection policy and the two system
// concepts the split may have to reach for.
func (s *FinanceService) prepareDeposit(ctx context.Context, in DepositInput) (domain.Deposit, *depositPlanner, error) {
	when := in.PaidAt
	if when.IsZero() {
		when = s.now()
	} else if when.After(s.now()) {
		return domain.Deposit{}, nil, glerrors.New(glerrors.KindInvalid,
			"la fecha del pago no puede estar en el futuro")
	}

	student, err := s.students.ByID(ctx, in.StudentID)
	if err != nil {
		return domain.Deposit{}, nil, glerrors.New(glerrors.KindInvalid, "student not found")
	}

	calendar, calendarVersion := s.schoolCalendar(ctx, in.StudentID)
	policy, err := s.policyFor(ctx, student.ID)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	lateFee, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindLateFee, lateFeeConceptName)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	tuition, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindTuition, tuitionConceptName)
	if err != nil {
		return domain.Deposit{}, nil, err
	}

	deposit, err := domain.NewDeposit(s.newID(), s.tenantID, in.StudentID, in.AmountCents,
		in.Method, in.Reference, when, in.IdempotencyKey, calendarVersion)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	return deposit, &depositPlanner{
		svc:         s,
		deposit:     deposit,
		policy:      policy,
		calendar:    calendar,
		lateFeeID:   lateFee.ID(),
		tuitionID:   tuition.ID(),
		paidOn:      when,
		description: lateFeeConceptName,
	}, nil
}

// VoidDeposit cancels a deposit and releases what it settled. The row survives
// with its reason: a school that deletes money rows cannot explain its own
// ledger, and the day's takings have to add up even when a cashier made a
// mistake.
func (s *FinanceService) VoidDeposit(ctx context.Context, depositID, reason string) error {
	d, err := s.repo.GetDeposit(ctx, depositID)
	if err != nil {
		return err
	}
	voided, err := d.Void(reason, s.now())
	if err != nil {
		return err
	}
	return s.repo.VoidDeposit(ctx, voided)
}

// Income is what the school earned in a period, by what it charged for.
type Income struct {
	From, To   time.Time
	Lines      []ports.IncomeLine
	TotalCents int64
}

// IncomeByConcept answers the question a director actually asks: how much came
// in for colegiaturas this month, and how much for everything else.
//
// It counts payments by the day the money changed hands, not the day the charge
// was issued — the two are months apart for anyone paying late, and reporting
// on the charge date would credit October with September's arrears.
//
// The lines come back biggest first. A report read on a phone is read from the
// top, and alphabetical order buries the number that matters.
func (s *FinanceService) IncomeByConcept(ctx context.Context, from, to time.Time) (Income, error) {
	lines, err := s.repo.IncomeLines(ctx, from, to)
	if err != nil {
		return Income{}, err
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].AmountCents > lines[j].AmountCents })
	income := Income{From: from, To: to, Lines: lines}
	for _, l := range lines {
		income.TotalCents += l.AmountCents
	}
	return income, nil
}

// CashUp is the day's takings: one number per method, the total, and the list
// behind them.
type CashUp struct {
	Day        time.Time
	Deposits   []domain.Deposit
	ByMethod   map[string]int64
	TotalCents int64
	// Cancelled are the day's voided deposits. They count for nothing and are
	// listed anyway: a cash-up that hid them could not explain why the drawer
	// holds less than the receipts say.
	Cancelled []domain.Deposit
}

// DailyCashUp adds up what came in on one day, and in what form.
//
// This is the artefact that replaces the notebook. At close of day the office
// can say what was collected, split by cash, card and transfer, without adding
// anything up by hand — and the list behind each number is right there when a
// figure is disputed.
//
// A cancelled deposit counts for nothing but is still shown. Hiding it would
// leave a gap between the receipts and the total that nobody could account for.
func (s *FinanceService) DailyCashUp(ctx context.Context, day time.Time) (CashUp, error) {
	deposits, err := s.repo.ListDepositsOn(ctx, day)
	if err != nil {
		return CashUp{}, err
	}
	cut := CashUp{Day: day, ByMethod: map[string]int64{}}
	for _, d := range deposits {
		if d.Voided() {
			cut.Cancelled = append(cut.Cancelled, d)
			continue
		}
		cut.Deposits = append(cut.Deposits, d)
		cut.ByMethod[d.Method()] += d.Amount().Int64()
		cut.TotalCents += d.Amount().Int64()
	}
	return cut, nil
}

// ListDeposits returns a student's deposits, newest first.
func (s *FinanceService) ListDeposits(ctx context.Context, studentID string) ([]domain.Deposit, error) {
	return s.repo.ListDeposits(ctx, studentID)
}

// depositPlanner holds what the split needs so the callback the repository runs
// stays a single expression of the rule rather than a closure over eight
// variables.
type depositPlanner struct {
	svc         *FinanceService
	deposit     domain.Deposit
	policy      domain.CollectionPolicy
	calendar    *schoolcal.Calendar
	lateFeeID   string
	tuitionID   string
	paidOn      time.Time
	description string
}

// plan turns the student's locked charges into the writes this deposit makes.
// It runs inside the transaction, which is why it takes the charges rather than
// reading them.
func (p *depositPlanner) plan(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
	base, feesByCharge := p.partition(open)
	sort.Slice(base, func(i, j int) bool {
		if !base[i].Charge.DueDate().Equal(base[j].Charge.DueDate()) {
			return base[i].Charge.DueDate().Before(base[j].Charge.DueDate())
		}
		return base[i].Charge.ID() < base[j].Charge.ID()
	})

	var (
		items      []domain.Payable
		newFees    = map[string]domain.Charge{}
		fresh      []domain.Charge
		discounted []domain.Charge
	)
	// budget walks alongside the items so the early-payment reward is only given
	// where the money in hand actually settles that month. Granting it on a part
	// payment would leave the month open at a price the school never set.
	budget := p.deposit.Amount().Int64()
	for _, c := range base {
		charge, due := c.Charge, owed(c)
		reduced, discountable, err := p.earlyPayment(c, budget)
		if err != nil {
			return ports.DepositWrite{}, err
		}
		if discountable {
			discounted = append(discounted, reduced)
			charge, due = reduced, reduced.Effective().Int64()-c.PaidCents
		}
		items = append(items, domain.Payable{
			ChargeID:  charge.ID(),
			DueOn:     charge.DueDate(),
			OwedCents: due,
		})
		budget -= min(budget, max(0, due))
		// Whatever surcharge this charge has already been given, in the order it
		// was given, comes before any new one: the older debt is the older debt.
		for _, fee := range feesByCharge[c.Charge.ID()] {
			items = append(items, domain.Payable{
				ChargeID:  fee.Charge.ID(),
				DueOn:     c.Charge.DueDate(),
				OwedCents: owed(fee),
				Surcharge: true,
			})
			budget -= min(budget, max(0, owed(fee)))
		}
		fee, ok, ferr := p.deriveSurcharge(c, feesByCharge[c.Charge.ID()])
		if ferr != nil {
			return ports.DepositWrite{}, ferr
		}
		if ok {
			newFees[fee.ID()] = fee
			items = append(items, domain.Payable{
				ChargeID:  fee.ID(),
				DueOn:     c.Charge.DueDate(),
				OwedCents: fee.Effective().Int64(),
				Surcharge: true,
			})
			budget -= min(budget, fee.Effective().Int64())
		}
	}

	allocations, unallocated := domain.Allocate(p.deposit.Amount().Int64(), items)

	payments := make([]domain.Payment, 0, len(allocations))
	for _, a := range allocations {
		if fee, isNew := newFees[a.ChargeID]; isNew {
			// A surcharge row exists only where money reached it. This is the
			// one place that decides that, so the ledger never fills with fees
			// nobody paid and nobody asked for.
			fresh = append(fresh, fee)
		}
		payment, err := domain.NewPayment(p.svc.newID(), p.svc.tenantID, a.ChargeID,
			a.AmountCents, p.deposit.Method(), p.deposit.Reference(), p.paidOn, "")
		if err != nil {
			return ports.DepositWrite{}, err
		}
		payments = append(payments, payment)
	}
	return ports.DepositWrite{
		Surcharges:       fresh,
		Discounted:       discounted,
		Allocations:      payments,
		UnallocatedCents: unallocated,
	}, nil
}

// partition splits the student's charges into the debts themselves and the
// surcharges already raised against each of them.
func (p *depositPlanner) partition(open []ports.ChargeWithPaid) ([]ports.ChargeWithPaid, map[string][]ports.ChargeWithPaid) {
	var base []ports.ChargeWithPaid
	fees := map[string][]ports.ChargeWithPaid{}
	for _, c := range open {
		if c.Charge.Status() == domain.StatusCancelled {
			continue
		}
		if c.Charge.ConceptID() != p.lateFeeID {
			base = append(base, c)
			continue
		}
		if parent, ok := surchargeParent(c.Charge.ExternalRef()); ok {
			fees[parent] = append(fees[parent], c)
		}
	}
	for parent := range fees {
		rows := fees[parent]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Charge.ID() < rows[j].Charge.ID() })
		fees[parent] = rows
	}
	return base, fees
}

// deriveSurcharge works out what a charge has earned in surcharge that has not
// been written down yet.
//
// The daily rule grows every night, so it is derived rather than billed — but
// the part already materialised by earlier deposits must be subtracted, or the
// same days get charged twice. Only the daily rule is derived here: under the
// monthly rule the billing run has already issued the fees, and they arrive as
// ordinary charges.
func (p *depositPlanner) deriveSurcharge(c ports.ChargeWithPaid, existing []ports.ChargeWithPaid) (domain.Charge, bool, error) {
	if !p.policy.DailyMode || owed(c) <= 0 {
		return domain.Charge{}, false, nil
	}
	total := domain.Surcharge(p.policy, p.calendar, c.Charge.DueDate(), p.paidOn)
	for _, fee := range existing {
		total -= fee.Charge.Effective().Int64()
	}
	if total <= 0 {
		return domain.Charge{}, false, nil
	}
	fee, err := domain.NewCharge(p.svc.newID(), p.svc.tenantID, c.Charge.StudentID(), c.Charge.CycleID(),
		p.lateFeeID, fmt.Sprintf("%s · %s", p.description, c.Charge.Description()),
		total, 0, c.Charge.DueDate())
	if err != nil {
		return domain.Charge{}, false, err
	}
	// The ref ties the fee to the debt that earned it, which is how the next
	// deposit knows what has already been charged.
	fee, err = fee.WithExternalRef(surchargeRef(c.Charge.ID(), p.deposit.ID()))
	if err != nil {
		return domain.Charge{}, false, err
	}
	return fee, true, nil
}

// earlyPayment grants the pronto pago on a monthly tuition charge, but only
// when the money in hand settles that month outright.
//
// The condition is the whole point. The reward is for paying a month early, so
// granting it on a part payment would leave the month open at a price the school
// never set — and the office would have to explain a balance that does not match
// any figure on the price list.
//
// It is given once: a charge that already carries a discount is left alone, so
// a second deposit in the same window cannot reprice a month twice.
func (p *depositPlanner) earlyPayment(c ports.ChargeWithPaid, budget int64) (domain.Charge, bool, error) {
	if p.policy.EarlyPaymentCents <= 0 || c.Charge.ConceptID() != p.tuitionID {
		return domain.Charge{}, false, nil
	}
	if c.Charge.Discount().Int64() > 0 || owed(c) <= 0 {
		return domain.Charge{}, false, nil
	}
	due := c.Charge.DueDate()
	monthStart := time.Date(due.Year(), due.Month(), 1, 0, 0, 0, 0, due.Location())
	discount := domain.EarlyPaymentDiscount(p.policy, p.calendar, monthStart, p.paidOn)
	if discount <= 0 || budget < owed(c)-discount {
		return domain.Charge{}, false, nil
	}
	reduced, err := c.Charge.WithDiscount(discount, c.PaidCents, domain.ReasonEarlyPayment)
	if err != nil {
		return domain.Charge{}, false, err
	}
	return reduced, true, nil
}

// surchargeRefPrefix is how a materialised daily surcharge names the charge it
// belongs to: "dailyfee:<chargeID>:<depositID>".
const surchargeRefPrefix = "dailyfee:"

func surchargeRef(chargeID, depositID string) string {
	return surchargeRefPrefix + chargeID + ":" + depositID
}

// surchargeParent reads back the charge a materialised surcharge belongs to.
func surchargeParent(ref string) (string, bool) {
	if !strings.HasPrefix(ref, surchargeRefPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(ref, surchargeRefPrefix)
	id, _, found := strings.Cut(rest, ":")
	if !found || id == "" {
		return "", false
	}
	return id, true
}

// owed is what is still to collect on a charge.
func owed(c ports.ChargeWithPaid) int64 {
	return c.Charge.Effective().Int64() - c.PaidCents
}

// schoolCalendar reads the closed days for the student's cycle, with its
// version so the deposit can record which calendar it was split against.
//
// It never fails the deposit: the reader already degrades to what it last held,
// or to weekends only, and a family standing at the counter must not be turned
// away because config is slow.
func (s *FinanceService) schoolCalendar(ctx context.Context, studentID string) (*schoolcal.Calendar, string) {
	if s.calendar == nil {
		return schoolcal.New(nil), ""
	}
	cycleID, err := s.activeCycleOf(ctx, studentID)
	if err != nil || cycleID == "" {
		return schoolcal.New(nil), ""
	}
	snap, err := s.calendar.NonWorkingDays(ctx, cycleID)
	if err != nil {
		return schoolcal.New(nil), ""
	}
	return schoolcal.New(snap.Days), snap.Version
}

// policyFor resolves the collection rule that applies to a student: the tuition
// plan of the level they are enrolled in. A student with no plan collects under
// the zero policy, which charges nothing extra — the school not having priced a
// level must never invent a surcharge.
func (s *FinanceService) policyFor(ctx context.Context, studentID string) (domain.CollectionPolicy, error) {
	enrollment, err := s.activeEnrollment(ctx, studentID)
	if err != nil || enrollment.CycleID == "" {
		return domain.CollectionPolicy{}, nil
	}
	levels, err := s.levelOfGroup(ctx, enrollment.CycleID)
	if err != nil {
		return domain.CollectionPolicy{}, nil
	}
	plans, err := s.repo.ListTuitionPlans(ctx, enrollment.CycleID)
	if err != nil {
		return domain.CollectionPolicy{}, err
	}
	for _, p := range plans {
		if p.LevelID() == levels[enrollment.GroupID] {
			return p.Policy(), nil
		}
	}
	return domain.CollectionPolicy{}, nil
}

// activeEnrollment finds where a student currently sits. Everything about how
// they are charged hangs off it: the cycle, the group, and through the group the
// level whose tuition table carries the collection rule.
func (s *FinanceService) activeEnrollment(ctx context.Context, studentID string) (ports.EnrollmentRef, error) {
	rows, err := s.enrollments.ListByStudent(ctx, studentID)
	if err != nil {
		return ports.EnrollmentRef{}, err
	}
	for _, e := range rows {
		if e.Status == enrolmentStatusActive {
			return e, nil
		}
	}
	return ports.EnrollmentRef{}, nil
}

// activeCycleOf is the cycle the student is enrolled in, or "" when they are not.
func (s *FinanceService) activeCycleOf(ctx context.Context, studentID string) (string, error) {
	e, err := s.activeEnrollment(ctx, studentID)
	if err != nil {
		return "", err
	}
	return e.CycleID, nil
}

// announceDeposit publishes one PaymentRegistered for the deposit — not one per
// allocation. A family that pays six months at once gets one receipt and one
// notification; six would read as six payments and the phone would buzz six
// times for one act.
func (s *FinanceService) announceDeposit(ctx context.Context, record ports.DepositRecord) {
	if s.publisher == nil {
		return
	}
	_ = s.publisher.PublishPaymentRegistered(ctx, ports.PaymentRegisteredEvent{
		PaymentID:   record.Deposit.ID(),
		StudentID:   record.Deposit.StudentID(),
		TenantID:    s.tenantID,
		Method:      record.Deposit.Method(),
		AmountCents: record.Deposit.Amount().Int64(),
		PaidAt:      record.Deposit.PaidAt(),
	})
	for _, settled := range record.Settled {
		_ = s.publisher.PublishChargeSettled(ctx, ports.ChargeSettledEvent{
			ChargeID:    settled.Charge.ID(),
			StudentID:   settled.Charge.StudentID(),
			TenantID:    s.tenantID,
			ExternalRef: settled.Charge.ExternalRef(),
			AmountCents: settled.Charge.Effective().Int64(),
			SettledAt:   record.Deposit.PaidAt(),
		})
	}
}
