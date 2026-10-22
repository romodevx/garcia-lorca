package usecase

import (
	"context"
	"fmt"
	"regexp"
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
	// CardType is debit or credit when the money came in on a card. The office
	// reconciles against the terminal at month end, and the terminal knows the
	// difference.
	CardType string
	// GrantEarlyPayment honours the early-payment reward even though the money
	// arrived after the window closed. It is the school saying yes to a parent
	// who is always punctual and slipped once — a decision, never a default,
	// and it is recorded as one.
	GrantEarlyPayment bool
	// ChargeIDs, when given, are the only charges the money may pay (task 8):
	// the cashier's choice at the counter, paid oldest first, each month with
	// its surcharge. What is left is the family's credit. Empty keeps the rule
	// as it was: the oldest debt first, across everything owed.
	ChargeIDs []string
	// Receiver is who took the money: the authenticated caller, set by the
	// gateway (task 7). Recorded on the deposit and on every payment it splits
	// into.
	Receiver domain.Receiver
	// Tenders, when given, are the methods the money came in by (T8): a credit
	// note completed with cash, say. They must add up to AmountCents, and they
	// replace Method, Reference and CardType. Empty keeps the single method.
	Tenders []TenderInput
	// VoucherID, when given, is the payment voucher this money is (ADR 0040):
	// the idempotency key is derived from it, so the voucher can be applied
	// once. Empty is a payment by hand, which links the pupil's only pending
	// voucher by itself.
	VoucherID string
	// DeclinedVoucherIDs are pending vouchers the office says this payment by
	// hand is NOT. Only without VoucherID.
	DeclinedVoucherIDs []string
}

// TenderInput is one method of a combined payment as the counter reports it.
type TenderInput struct {
	Method      string
	AmountCents int64
	Reference   string
	// CardType is debit or credit, only on a card.
	CardType string
}

// toTenders validates the counter's tenders one by one; whether they add up
// is the payment's rule (Deposit.WithTenders, tenderedPayments).
func toTenders(in []TenderInput) ([]domain.Tender, error) {
	out := make([]domain.Tender, 0, len(in))
	for _, t := range in {
		tender, err := domain.NewTender(t.Method, t.AmountCents, t.Reference, t.CardType)
		if err != nil {
			return nil, err
		}
		out = append(out, tender)
	}
	return out, nil
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
	// LinkedVoucher is the payment voucher this deposit paid, or nil.
	LinkedVoucher *domain.PaymentVoucher
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
	when, err := s.paymentDate(in.PaidAt)
	if err != nil {
		return DepositResult{}, err
	}
	in, choice, err := s.voucherChoice(ctx, in)
	if err != nil {
		return DepositResult{}, err
	}
	deposit, planner, err := s.prepareDeposit(ctx, in, when)
	if err != nil {
		return DepositResult{}, err
	}
	deposit = deposit.WithVoucherChoice(choice)

	record, replayed, err := s.repo.RegisterDeposit(ctx, deposit, planner.plan, s.depositEvents)
	if err != nil {
		return DepositResult{}, err
	}
	if !replayed {
		// The events are already durable; this only gets them out now rather
		// than on the next drain, and cannot fail the deposit.
		s.drainQuietly(ctx)
	}
	return DepositResult{
		Deposit:          record.Deposit,
		Surcharges:       record.Surcharges,
		Allocations:      record.Allocations,
		UnallocatedCents: record.UnallocatedCents,
		Replayed:         replayed,
		LinkedVoucher:    record.LinkedVoucher,
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
	when, err := s.paymentDate(in.PaidAt)
	if err != nil {
		return DepositResult{}, err
	}
	deposit, planner, err := s.prepareDeposit(ctx, in, when)
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
// concepts the split may have to reach for. when is the deposit's date,
// already resolved (paymentDate, or the original's on a correction): in.PaidAt
// is not read here.
func (s *FinanceService) prepareDeposit(ctx context.Context, in DepositInput, when paymentDay) (domain.Deposit, *depositPlanner, error) {
	student, err := s.students.ByID(ctx, in.StudentID)
	if err != nil {
		return domain.Deposit{}, nil, glerrors.New(glerrors.KindInvalid, "student not found")
	}

	calendar, calendarVersion := s.schoolCalendar(ctx, in.StudentID)
	plan, err := s.planFor(ctx, student.ID)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	lateFee, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindLateFee, lateFeeConceptName)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	enrolment, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindEnrollment, enrollmentConceptName)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	tuition, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindTuition, tuitionConceptName)
	if err != nil {
		return domain.Deposit{}, nil, err
	}

	deposit, err := s.newDeposit(in, when, calendarVersion)
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	deposit = deposit.WithReceiver(in.Receiver)
	pricer, err := s.pricerFor(ctx, []string{in.StudentID},
		systemConceptIDs{tuition: tuition.ID(), enrolment: enrolment.ID(), lateFee: lateFee.ID()})
	if err != nil {
		return domain.Deposit{}, nil, err
	}
	return deposit, &depositPlanner{
		scholarships: pricer,
		svc:          s,
		deposit:      deposit,
		tuition:      plan,
		policy:       plan.Policy(),
		calendar:     calendar,
		lateFeeID:    lateFee.ID(),
		tuitionID:    tuition.ID(),
		enrolmentID:  enrolment.ID(),
		paidOn:       when.on,
		description:  lateFeeConceptName,
		grantEarly:   in.GrantEarlyPayment,
		chosen:       chosenSet(in.ChargeIDs),
	}, nil
}

// newDeposit builds the deposit the counter described: by its tenders when it
// sent them (T8), by its single method otherwise. Every card part must say
// debit or credit (CC-205): this is money being taken, not read back.
func (s *FinanceService) newDeposit(in DepositInput, when paymentDay, calendarVersion string) (domain.Deposit, error) {
	deposit, err := s.describedDeposit(in, when, calendarVersion)
	if err != nil {
		return domain.Deposit{}, err
	}
	if err := deposit.RequireCardType(); err != nil {
		return domain.Deposit{}, err
	}
	return deposit, nil
}

// describedDeposit is the deposit exactly as the counter described it.
func (s *FinanceService) describedDeposit(in DepositInput, when paymentDay, calendarVersion string) (domain.Deposit, error) {
	if len(in.Tenders) == 0 {
		deposit, err := domain.NewDeposit(s.newID(), s.tenantID, in.StudentID, in.AmountCents,
			in.Method, in.Reference, when.at, in.IdempotencyKey, calendarVersion)
		if err != nil {
			return domain.Deposit{}, err
		}
		return deposit.WithPaidOn(when.on).WithCardType(in.CardType)
	}
	tenders, err := toTenders(in.Tenders)
	if err != nil {
		return domain.Deposit{}, err
	}
	deposit, err := domain.NewDeposit(s.newID(), s.tenantID, in.StudentID, in.AmountCents,
		tenders[0].Method(), "", when.at, in.IdempotencyKey, calendarVersion)
	if err != nil {
		return domain.Deposit{}, err
	}
	return deposit.WithPaidOn(when.on).WithTenders(tenders)
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
	voided, err := d.Void(reason, s.clock.Now())
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
	// CreditNoteCents is what credit notes settled in the period (T8). It is
	// not income — no money came in — so it is in neither Lines nor TotalCents.
	CreditNoteCents int64
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
	income := Income{From: from, To: to, Lines: make([]ports.IncomeLine, 0, len(lines))}
	for _, l := range lines {
		if l.CreditNote {
			income.CreditNoteCents += l.AmountCents
			continue
		}
		income.Lines = append(income.Lines, l)
		income.TotalCents += l.AmountCents
	}
	sort.Slice(income.Lines, func(i, j int) bool { return income.Lines[i].AmountCents > income.Lines[j].AmountCents })
	return income, nil
}

// Expected is what a family must hand over today to be current: the months
// already due, what being late has cost, and what paying early saves.
//
// It is what the counter shows as "Monto esperado" and never lets anybody edit.
// The captured amount is compared against it, and the difference is recorded —
// which is the whole point: the school wants to know when a payment did not
// match, not to be stopped from taking it.
type Expected struct {
	AmountCents    int64
	SurchargeCents int64
	DiscountCents  int64
	// ScholarshipCents is what scholarships take off the lines asked for
	// (T18); already out of AmountCents.
	ScholarshipCents int64
	// AnnualBalanceCents is the whole year still owed — THE balance (T5 ·
	// ISSUE-006), the account's TotalBalanceCents and the statement's
	// Totals.BalanceCents, always as of today (whatever day is quoted):
	// accrued surcharge in, credit netted, charges not yet open out, and no
	// early-payment reward (a projection, not a debt). Shown beside the
	// expected figure so nobody confuses "what is due today" with "what is
	// left". Set by ExpectedToday; expectedFrom leaves it zero, having no
	// deposits to net.
	AnnualBalanceCents int64
	// OnTimeBalanceCents is AnnualBalanceCents if every month in it is paid in
	// time: less the early-payment rewards of the lines InBalance. A
	// projection, not a debt; zero wherever AnnualBalanceCents is.
	OnTimeBalanceCents int64
	// Lines are the same figures concept by concept (task 8), oldest due date
	// first: the counter lists them so the cashier can choose what the money
	// pays. The Due lines add up to AmountCents.
	Lines []ExpectedLine
}

// ExpectedLine is one charge still owed, as of the day being asked about.
type ExpectedLine struct {
	ChargeID    string
	Description string
	DueDate     time.Time
	// OwedCents is what is left of the charge itself, at the price that day.
	OwedCents int64
	// SurchargeCents is what being late has cost this charge: surcharges
	// already written down and still unpaid, plus what the daily rule has
	// earned since.
	SurchargeCents int64
	// DiscountCents is the early-payment reward settling it that day earns.
	DiscountCents int64
	// ScholarshipCents is the scholarship's part that day (T18), already out
	// of OwedCents; 0 when none applies or it was lost by paying late.
	ScholarshipCents int64
	TotalCents       int64
	// Due is whether the charge is asked for that day (see expectedBy).
	Due bool
	// InBalance is whether the ledger counts the charge today (inPlayOn):
	// false for one not yet open with nothing paid on it — listed so it can
	// be paid ahead, but not part of the debt.
	InBalance bool
}

// ExpectedToday works out what is due as of a date.
//
// Only the months whose date has arrived. The year is billed up front, but
// asking a family for all of it at the counter is not what that figure means.
func (s *FinanceService) ExpectedToday(ctx context.Context, studentID string, on time.Time) (Expected, error) {
	expected, q, err := s.quote(ctx, studentID, on)
	if err != nil {
		return Expected{}, err
	}
	payments, err := s.repo.ListPayments(ctx, studentID)
	if err != nil {
		return Expected{}, err
	}
	deposits, err := s.repo.ListDeposits(ctx, studentID)
	if err != nil {
		return Expected{}, err
	}
	// The statement's balance TODAY, never a second sum and never another day's:
	// a quote for a back-dated payment still shows the balance every other
	// surface shows (and the ladder is not walked down through a past date).
	_, totals, err := ledger(q.charges, payments, deposits, q.terms, s.clock.Today(), false)
	if err != nil {
		return Expected{}, err
	}
	expected.AnnualBalanceCents = totals.BalanceCents
	expected.OnTimeBalanceCents = totals.BalanceCents - rewardsInBalance(expected.Lines)
	return expected, nil
}

// ExpectedWithoutBalance is ExpectedToday without the year's balance
// (AnnualBalanceCents and OnTimeBalanceCents stay zero): for a list that shows
// only what is due, it spares the payments and deposits reads per row.
func (s *FinanceService) ExpectedWithoutBalance(ctx context.Context, studentID string, on time.Time) (Expected, error) {
	expected, _, err := s.quote(ctx, studentID, on)
	return expected, err
}

// quoteInputs is what a quote was worked out from, for the ledger to reuse.
type quoteInputs struct {
	charges []ports.ChargeWithPaid
	terms   collectionTerms
}

// quote is the counter's arithmetic over the student's charges, read once.
func (s *FinanceService) quote(ctx context.Context, studentID string, on time.Time) (Expected, quoteInputs, error) {
	when := on
	if when.IsZero() {
		when = s.clock.Today()
	}
	calendar, _ := s.schoolCalendar(ctx, studentID)
	// The whole table, not just its policy: the inscription's price is a
	// function of the day being asked about, and only the table knows the ladder.
	plan, err := s.planFor(ctx, studentID)
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	lateFee, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindLateFee, lateFeeConceptName)
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	enrolment, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindEnrollment, enrollmentConceptName)
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	tuition, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindTuition, tuitionConceptName)
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	charges, err := s.repo.ListCharges(ctx, studentID, "")
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	pricer, err := s.pricerFor(ctx, []string{studentID},
		systemConceptIDs{tuition: tuition.ID(), enrolment: enrolment.ID(), lateFee: lateFee.ID()})
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	expected, err := expectedFrom(charges, plan, calendar, lateFee.ID(), tuition.ID(), enrolment.ID(), when, s.clock.Today(), true, pricer)
	if err != nil {
		return Expected{}, quoteInputs{}, err
	}
	terms := collectionTerms{plan: plan, calendar: calendar, lateFeeID: lateFee.ID(), tuitionID: tuition.ID(),
		enrolmentID: enrolment.ID(), scholarships: pricer}
	return expected, quoteInputs{charges: charges, terms: terms}, nil
}

// rewardsInBalance is the early-payment reward of the lines the balance holds:
// a line not yet open is not owed, so its reward takes nothing off the debt.
func rewardsInBalance(lines []ExpectedLine) int64 {
	var sum int64
	for _, l := range lines {
		if l.InBalance {
			sum += l.DiscountCents
		}
	}
	return sum
}

// expectedFrom is the arithmetic of "what is due today", with NO I/O.
//
// Pulled out so GetAccount can reuse it against the charges it has ALREADY read.
// Before this, the parent app's most-hit endpoint read every charge twice, made
// three cross-service calls a second time, and — through EnsureConceptOfKind —
// performed two idempotent WRITES on the way through. A statement is a read.
func expectedFrom(
	charges []ports.ChargeWithPaid,
	plan domain.TuitionPlan,
	calendar *schoolcal.Calendar,
	lateFeeID, tuitionID, enrolmentID string,
	when, now time.Time,
	withLines bool,
	scholarships *scholarshipPricer,
) (Expected, error) {
	policy := plan.Policy()
	// Every charge as a payment on `when` would find it: with its scholarship
	// (T18). The surcharge rule reads these — a month the scholarship settled
	// in time owes nothing and earns nothing.
	priced, err := scholarships.priceAll(charges, when)
	if err != nil {
		return Expected{}, err
	}

	var out Expected
	// A surcharge whose month was cancelled is owed by nobody: a deposit never
	// pays it (its month is not collected), so the quote does not ask for it.
	cancelled := map[string]bool{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled {
			cancelled[c.Charge.ID()] = true
		}
	}
	accrued := accruedSurcharges(priced, policy, calendar, lateFeeID, when)
	// unpaidFees is the part of a charge's written-down surcharges still owed,
	// so its line can carry it (task 8).
	unpaidFees := map[string]int64{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled || c.Charge.ConceptID() != lateFeeID {
			continue
		}
		// Any written-down surcharge, daily or monthly, rides on its month.
		if parent, ok := feeParent(c.Charge.ExternalRef()); ok && !cancelled[parent] && owed(c) > 0 && expectedBy(c.Charge.DueDate(), when) {
			unpaidFees[parent] += owed(c)
		}
	}
	// The ledger's rule for what the balance holds, on the ledger's day.
	inBalance := inPlayOn(now)
	var lines map[string]*ExpectedLine
	if withLines {
		lines = map[string]*ExpectedLine{}
	}

	for i, raw := range charges {
		if raw.Charge.Status() == domain.StatusCancelled {
			continue
		}
		c := priced[i]
		if owed(c) <= 0 {
			// A month already paid whose surcharge is still open is still owed:
			// its line carries the surcharge, so it can be chosen and the due
			// lines add up to the amount.
			if fees := unpaidFees[c.Charge.ID()]; fees > 0 && lines != nil && c.Charge.ConceptID() != lateFeeID {
				lines[c.Charge.ID()] = &ExpectedLine{
					ChargeID: c.Charge.ID(), Description: c.Charge.Description(), DueDate: c.Charge.DueDate(),
					SurchargeCents: fees, Due: true, InBalance: true,
				}
			}
			continue
		}
		// The ladder, at the day being asked about. Quoting the stored amount
		// instead would show one number and collect another in the same
		// transaction — see ladderPrice.
		// Quoted as paying it in full on `when` would price it — the same rule
		// the deposit applies when the money settles the line (task 7).
		// The ladder prices the charge as stored; the scholarship comes off
		// the price the ladder gives.
		settled, change, err := inscriptionPaidOn(raw, plan, enrolmentID, when, now)
		if err != nil {
			return Expected{}, err
		}
		if change != priceUnchanged {
			raw.Charge = settled
			if c, err = scholarships.price(raw, when); err != nil {
				return Expected{}, err
			}
		}
		isFee := c.Charge.ConceptID() == lateFeeID
		if parent, ok := feeParent(c.Charge.ExternalRef()); isFee && ok && cancelled[parent] {
			continue
		}
		// A written-down surcharge rides on its month's line. One with no month
		// — a surcharge the office typed by hand — is a debt of its own: its
		// own line, chosen and paid like any other (partition makes it one).
		var line *ExpectedLine
		if _, ridesOnMonth := feeParent(c.Charge.ExternalRef()); !isFee || !ridesOnMonth {
			line = &ExpectedLine{ChargeID: c.Charge.ID(), Description: c.Charge.Description(), DueDate: c.Charge.DueDate(),
				OwedCents: owed(c), ScholarshipCents: c.Charge.Scholarship().Int64(), InBalance: inBalance(charges[i])}
			if lines != nil {
				lines[c.Charge.ID()] = line
			}
		}
		if !expectedBy(c.Charge.DueDate(), when) || !c.Charge.OpenOn(when) {
			// Billed, but not yet asked for — not due this month, or not open
			// yet (T5): a charge the family is not shown is not asked for. Its line still shows the reward
			// paying it ahead would earn — the deposit grants it (earlyPayment),
			// so the total the counter fills in is the total it takes.
			if line != nil && c.Charge.ConceptID() == tuitionID && c.Charge.Discount().Int64() == 0 {
				line.DiscountCents = min(earlyReward(policy, calendar, c.Charge.DueDate(), when), owed(c))
			}
			continue
		}
		out.AmountCents += owed(c)

		if isFee {
			out.SurchargeCents += owed(c)
			if line != nil {
				// A surcharge with no month to ride on is its own line.
				line.SurchargeCents, line.OwedCents = owed(c), 0
				line.Due = true
			}
			continue
		}
		line.Due = true
		out.ScholarshipCents += line.ScholarshipCents
		line.SurchargeCents += unpaidFees[c.Charge.ID()]
		// What the daily rule has earned and nobody has written down yet.
		if pending := accrued[c.Charge.ID()]; pending > 0 {
			out.AmountCents += pending
			out.SurchargeCents += pending
			line.SurchargeCents += pending
		}
		if c.Charge.ConceptID() == tuitionID && c.Charge.Discount().Int64() == 0 {
			if d := earlyReward(policy, calendar, c.Charge.DueDate(), when); d > 0 {
				// Never more than is left of the month: a part payment can leave
				// less owed than the reward, and a line must not go negative.
				d = min(d, owed(c))
				out.DiscountCents += d
				out.AmountCents -= d
				line.DiscountCents = d
			}
		}
	}
	if out.AmountCents < 0 {
		out.AmountCents = 0
	}
	if lines != nil {
		out.Lines = sortedLines(lines)
	}
	return out, nil
}

// accruedSurcharges is, per charge, what the daily rule has earned up to `when`
// and nobody has written down yet: the surcharge a month asked for that day is
// carrying. ONE rule for the counter's quote (expectedFrom) and the account
// statement (T7), so the two can never quote different late fees.
//
// A month earns only while it is owed, due by `when` and open (T5). What a
// deposit already wrote down as a daily surcharge row is subtracted — that part
// is a charge of its own now.
func accruedSurcharges(
	charges []ports.ChargeWithPaid, policy domain.CollectionPolicy, calendar *schoolcal.Calendar,
	lateFeeID string, when time.Time,
) map[string]int64 {
	if !policy.DailyMode {
		return nil
	}
	materialised := map[string]int64{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled || c.Charge.ConceptID() != lateFeeID {
			continue
		}
		// Only daily surcharges are subtracted from what the daily rule earns.
		if parent, ok := surchargeParent(c.Charge.ExternalRef()); ok {
			materialised[parent] += c.Charge.Effective().Int64()
		}
	}
	out := map[string]int64{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled || c.Charge.ConceptID() == lateFeeID || owed(c) <= 0 {
			continue
		}
		if domain.IsActivityRef(c.Charge.ExternalRef()) {
			// An activity's only surcharge is its own fixed fee, written by the
			// run (T17); the tuition table's daily rule is not its rule.
			continue
		}
		if !expectedBy(c.Charge.DueDate(), when) || !c.Charge.OpenOn(when) {
			continue
		}
		if pending := domain.Surcharge(policy, calendar, c.Charge.DueDate(), when) - materialised[c.Charge.ID()]; pending > 0 {
			out[c.Charge.ID()] = pending
		}
	}
	return out
}

// earlyReward is the early-payment discount a month due on `due` earns when paid
// on `when` — the same rule earlyPayment applies to the deposit.
func earlyReward(policy domain.CollectionPolicy, calendar *schoolcal.Calendar, due, when time.Time) int64 {
	monthStart := time.Date(due.Year(), due.Month(), 1, 0, 0, 0, 0, due.Location())
	return domain.EarlyPaymentDiscount(policy, calendar, monthStart, when)
}

// sortedLines is the counter's list: oldest due date first, then by id so the
// order is the same on every read.
func sortedLines(byID map[string]*ExpectedLine) []ExpectedLine {
	out := make([]ExpectedLine, 0, len(byID))
	for _, l := range byID {
		l.TotalCents = l.OwedCents + l.SurchargeCents - l.DiscountCents
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].DueDate.Equal(out[j].DueDate) {
			return out[i].DueDate.Before(out[j].DueDate)
		}
		return out[i].ChargeID < out[j].ChargeID
	})
	return out
}

// expectedBy decides whether a charge is being asked for as of a date: it has
// fallen due, or it falls due later THIS MONTH.
//
// The second half is the point of the early-payment reward. In September a
// family pays September, and they pay it before the 10th precisely to earn the
// discount — a rule that only counted arrived due dates would show them nothing
// to pay and quietly deny the reward it exists to give.
func expectedBy(due, when time.Time) bool {
	if !due.After(when) {
		return true
	}
	return due.Year() == when.Year() && due.Month() == when.Month()
}

// CashUp is the day's takings: one number per method, the total, and the list
// behind them.
type CashUp struct {
	Day        time.Time
	Deposits   []domain.Deposit
	ByMethod   map[string]int64
	TotalCents int64
	// Payments are the day's payments taken outside a deposit (a counter
	// sale's). They are counted: the money is in the drawer.
	Payments []ports.LoosePayment
	// Cancelled are the day's voided deposits. They count for nothing and are
	// listed anyway: a cash-up that hid them could not explain why the drawer
	// holds less than the receipts say.
	Cancelled []domain.Deposit
	// Superseded are the day's deposits replaced by a correction (T9). They
	// count for nothing — their correction counts on the day it is dated — and
	// are listed so the day's receipts still add up.
	Superseded []domain.Deposit
	// CancelledSales are the day's counter-sale payments voided by a same-day
	// return (C2-015). Like Cancelled, they count for nothing and are listed.
	CancelledSales []ports.VoidedSalePayment
	// CreditNoteCents is what credit notes settled that day (T8). Not money:
	// it is in neither ByMethod nor TotalCents, and is reported on its own line.
	CreditNoteCents int64
}

// count adds one tender of the day to the cut: money by its method, a credit
// note on its own line.
func (c *CashUp) count(method string, cents int64) {
	if method == domain.MethodCreditNote {
		c.CreditNoteCents += cents
		return
	}
	c.ByMethod[method] += cents
	c.TotalCents += cents
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
// An unset day is the school's today (the clock's), never the server's.
func (s *FinanceService) DailyCashUp(ctx context.Context, day time.Time) (CashUp, error) {
	if day.IsZero() {
		day = s.clock.Today()
	}
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
		if d.Superseded() {
			cut.Superseded = append(cut.Superseded, d)
			continue
		}
		cut.Deposits = append(cut.Deposits, d)
		for _, t := range d.Tenders() {
			cut.count(t.Method(), t.Amount().Int64())
		}
	}
	loose, err := s.repo.ListLoosePaymentsOn(ctx, day)
	if err != nil {
		return CashUp{}, err
	}
	for _, p := range loose {
		cut.Payments = append(cut.Payments, p)
		cut.count(p.Payment.Method(), p.Payment.Amount().Int64())
	}
	voided, err := s.repo.ListVoidedSalePaymentsOn(ctx, day)
	if err != nil {
		return CashUp{}, err
	}
	cut.CancelledSales = voided
	return cut, nil
}

// DepositReceipt returns a deposit and what it paid, for its receipt (task 12).
// An id that cannot be a deposit's is "no such receipt" — never a query the
// database refuses (22P02) and reports as an internal error.
func (s *FinanceService) DepositReceipt(ctx context.Context, id string) (ports.DepositReceipt, error) {
	if !uuidShape.MatchString(id) {
		return ports.DepositReceipt{}, glerrors.ErrNotFound
	}
	receipt, err := s.repo.DepositReceipt(ctx, id)
	if err != nil {
		return ports.DepositReceipt{}, err
	}
	pending, err := s.pendingAfter(ctx, receipt)
	if err != nil {
		return ports.DepositReceipt{}, err
	}
	receipt.PendingCents = pending
	return receipt, nil
}

// pendingAfter is the receipt's "Saldo pendiente" (CC-201): what the charges a
// deposit paid still owed right after it, each priced by the rule every other
// surface reads (scholarshipPricer → domain.ApplyScholarship). A scholarship
// is READ until the charge settles (ADR 0022), so the stored row says $4,000
// while the statement, the counter and the app say $3,500 — pricing off the
// row printed $1,000 pending on a $3,000 abono where everybody else said $500.
//
// Priced on the deposit's own day, the day the planner priced the money on,
// so a reprint says what the paper said even after the due date has passed.
// A cancelled charge owes nothing.
func (s *FinanceService) pendingAfter(ctx context.Context, receipt ports.DepositReceipt) (int64, error) {
	if len(receipt.After) == 0 {
		return 0, nil
	}
	ids, err := s.conceptIDs(ctx)
	if err != nil {
		return 0, err
	}
	pricer, err := s.pricerFor(ctx, []string{receipt.Deposit.StudentID()}, ids)
	if err != nil {
		return 0, err
	}
	priced, err := pricer.priceAll(receipt.After, receipt.Deposit.PaidOn())
	if err != nil {
		return 0, err
	}
	var pending int64
	for _, c := range priced {
		if c.Charge.Status() == domain.StatusCancelled {
			continue
		}
		pending += max(0, owed(c))
	}
	return pending, nil
}

// IsDepositID reports whether id has the shape of a deposit's id. The edges
// check it so a malformed id is "no such deposit", never a query the database
// refuses and reports as an internal error.
func IsDepositID(id string) bool { return uuidShape.MatchString(id) }

// uuidShape is the canonical 8-4-4-4-12 form: a shape check, not a parse.
var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ListDeposits returns a student's deposits, newest first.
func (s *FinanceService) ListDeposits(ctx context.Context, studentID string) ([]domain.Deposit, error) {
	return s.repo.ListDeposits(ctx, studentID)
}

// depositPlanner holds what the split needs so the callback the repository runs
// stays a single expression of the rule rather than a closure over eight
// variables.
type depositPlanner struct {
	svc     *FinanceService
	deposit domain.Deposit
	// tuition is the student's tuition table. The planner needs the whole thing
	// and not just the policy now: the inscription's price is a function of the
	// day the money arrives, and only the table knows the ladder.
	//
	// Named tuition and not plan because plan() is what this type DOES.
	tuition     domain.TuitionPlan
	enrolmentID string
	policy      domain.CollectionPolicy
	calendar    *schoolcal.Calendar
	lateFeeID   string
	tuitionID   string
	// paidOn is the DAY the rules judge the money on (paymentDate): the
	// school's today for a deposit taken now, stored as paid_on. The instant
	// stays on the deposit as paid_at.
	paidOn      time.Time
	description string
	// grantEarly carries the school's decision to honour the reward late.
	grantEarly bool
	// chosen restricts the split to these charges; nil means every charge.
	chosen map[string]bool
	// scholarships prices each charge on the payment's date (T18); nil prices
	// none.
	scholarships *scholarshipPricer
}

// pendingScholarship is a charge priced with a scholarship this deposit may
// settle, and what it owed before the money came in.
type pendingScholarship struct {
	charge domain.Charge
	due    int64
}

// repriceInscription is the ladder applied at the counter: what this line of the
// inscription costs on the day the money arrived.
//
// It carries the SAME two guards as the repository's bulk re-price, and for the
// same reasons (ADR 0011). Nothing collected: the first peso freezes the price,
// so a family holding a receipt never finds the statement disagreeing with it.
// Nothing negotiated: a discount is a price a PERSON decided, and a price list
// must not overwrite a decision made by hand — the full scholarship is the case
// that proves it, because it never receives a peso and the payment guard alone
// would never protect it.
//
// Reports ok=false when nothing should move, so the caller writes no row for a
// price that did not change.
func (p *depositPlanner) repriceInscription(c ports.ChargeWithPaid) (domain.Charge, bool, error) {
	amount, ok := ladderPrice(c, p.tuition, p.enrolmentID, p.svc.clock.Today())
	if !ok {
		return domain.Charge{}, false, nil
	}
	moved, err := c.Charge.WithAmount(amount)
	if err != nil {
		return domain.Charge{}, false, err
	}
	return moved, true, nil
}

// ladderPrice is what one charge is worth on a day, and whether the ladder
// applies to it at all.
//
// It is a package function and not a method on the planner because the pricing
// must not live in one path only. When only the payment path knew this rule, the
// counter showed the amount last written down while the same transaction took
// the money at today's rung: the family left with a receipt for one number
// owing another, and the freeze rule fixed the price at the higher one. That is
// the outcome ADR 0011 says the rule exists to prevent. (The quote now prices
// through inscriptionPaidOn; the deposit uses this when a payment does not
// settle the line at the price of its date.)
//
// Reports ok=false when nothing should move, which is most charges: anything
// that is not an inscription line, anything with money on it (the first peso
// freezes the price), anything carrying a discount (a price a PERSON decided,
// which a price list must not overwrite), and a line already at its rung.
func ladderPrice(
	c ports.ChargeWithPaid, plan domain.TuitionPlan, enrolmentConceptID string, on time.Time,
) (int64, bool) {
	rung, ok := rungOn(c, plan, enrolmentConceptID, on)
	// The ladder ONLY EVER CLIMBS, and the guard lives here rather than only
	// in the plan's validation, because the plan is not the only thing that
	// can point downward:
	//
	//   - paidAt comes from the client and is bounded only in the future, so
	//     a peso dated before the first rung would price the inscription at
	//     the earliest one, freeze it there, and leave the difference with no
	//     discount row, no reason and no event.
	//   - a school that edits the table cheaper would walk every unpaid
	//     inscription down with it, below what the families were quoted.
	//
	// A charge already issued is a number a family has been told. It may rise
	// while it goes unpaid; it may never fall by itself. Lowering one is a
	// decision somebody makes by hand, and that is what a discount is for.
	// (Settling at the price of the payment date is the one lowering the
	// system makes, as a reasoned discount: inscriptionPaidOn.)
	if !ok || rung <= c.Charge.Amount().Int64() {
		return 0, false
	}
	return rung, true
}

// rungOn is the price the ladder gives an inscription line on a day, with no
// ratchet, and whether the line is one the ladder may touch at all: an
// inscription line with nothing paid, no discount a person decided, not
// cancelled.
func rungOn(c ports.ChargeWithPaid, plan domain.TuitionPlan, enrolmentConceptID string, on time.Time) (int64, bool) {
	if c.Charge.ConceptID() != enrolmentConceptID || c.PaidCents > 0 || c.Charge.Discount().Int64() > 0 {
		return 0, false
	}
	if c.Charge.Status() == domain.StatusCancelled {
		return 0, false
	}
	for _, line := range plan.EnrollmentLinesOn(on) {
		// Matched on the description, which is what enrollmentCharges wrote and
		// what the family reads. A plan with no concepts yields one unnamed line
		// and matches nothing here, so a school that never adopted the ladder is
		// untouched.
		if line.Name != "" && isLineOf(line.Name, c.Charge) {
			return line.AmountCents, true
		}
	}
	return 0, false
}

// isLineOf reports whether an issued inscription charge is the plan line of
// that name — by the same identity, and under the same rule, the run re-prices
// by (issuedLines), or the counter would show one number and collect another.
//
// A line whose name was RE-SPELLED after it was issued ("Inscripción" →
// "INSCRIPCION") is the same line. A single-amount inscription is not a rung
// of anybody's ladder, even when a concept carries its exact name: the run does
// not re-price it, so neither does the counter.
func isLineOf(name string, c domain.Charge) bool {
	return isConceptRef(c.ExternalRef()) && domain.LineKey(name) == domain.LineKey(c.Description())
}

// priceChange is what settling an inscription at its payment date does to it.
type priceChange int

const (
	// priceUnchanged: the payment date says nothing about this line.
	priceUnchanged priceChange = iota
	// priceHeld: it already stands at the price of the payment date, so a
	// payment that settles it keeps it there (today's rung does not apply).
	priceHeld
	priceRaised
	priceLowered
)

// inscriptionPaidOn is the inscription as a payment dated paidOn that SETTLES it
// would find it (task 7, the owner's decision): the price of the day the family
// paid, not of the day a cashier captured it.
//
// Two guards keep this from reopening SEC-2 (see ladderPrice):
//   - the caller applies it only when the money in hand settles the line at
//     that price, so a back-dated peso freezes nothing;
//   - a LOWERING is a discount with its own reason, never a silent change, and
//     it is capped by the climb the ladder shows between paidOn and today — so
//     a table edited cheaper does not walk the charge down through this door.
func inscriptionPaidOn(
	c ports.ChargeWithPaid, plan domain.TuitionPlan, enrolmentConceptID string, paidOn, now time.Time,
) (domain.Charge, priceChange, error) {
	// No floor beyond the table itself (the owner's decision): a date before
	// its first tier prices at that first tier — a price the school set for
	// this cycle — so a date typed years back reaches nothing cheaper. A cycle
	// or issue-date floor would charge the later price to a family that paid
	// the inscription early and was captured late.
	paidRung, ok := rungOn(c, plan, enrolmentConceptID, paidOn)
	if !ok {
		return domain.Charge{}, priceUnchanged, nil
	}
	current := c.Charge.Amount().Int64()
	if paidRung > current {
		raised, err := c.Charge.WithAmount(paidRung)
		return raised, priceRaised, err
	}
	if paidRung == current {
		return c.Charge, priceHeld, nil
	}
	nowRung, _ := rungOn(c, plan, enrolmentConceptID, now)
	climb := nowRung - paidRung
	if climb <= 0 {
		return domain.Charge{}, priceUnchanged, nil
	}
	lowered, err := c.Charge.WithDiscount(min(current-paidRung, climb), 0, domain.ReasonPriceAtPaymentDate)
	return lowered, priceLowered, err
}

// plan turns the student's locked charges into the writes this deposit makes.
// It runs inside the transaction, which is why it takes the charges rather than
// reading them.
func (p *depositPlanner) plan(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
	base, feesByCharge := p.partition(open)
	base, err := p.onlyChosen(base)
	if err != nil {
		return ports.DepositWrite{}, err
	}
	sort.Slice(base, func(i, j int) bool {
		if !base[i].Charge.DueDate().Equal(base[j].Charge.DueDate()) {
			return base[i].Charge.DueDate().Before(base[j].Charge.DueDate())
		}
		return base[i].Charge.ID() < base[j].Charge.ID()
	})

	var (
		items   []domain.Payable
		newFees = map[string]domain.Charge{}
		// feeOwedBy pairs each derived surcharge with its month and what the
		// month owed before this deposit, to tell afterwards whether it settled.
		feeOwedBy  []derivedFee
		fresh      []domain.Charge
		discounted []domain.Charge
		repriced   []domain.Charge
		// becas are the charges priced with a scholarship on the payment's
		// date; those the money settles get it written (T18).
		becas []pendingScholarship
	)
	// budget walks alongside the items so the early-payment reward is only given
	// where the money in hand actually settles that month. Granting it on a part
	// payment would leave the month open at a price the school never set.
	budget := p.deposit.Amount().Int64()
	for i, c := range base {
		// The ladder. There is NO nightly run: RunBilling fires only when
		// somebody presses "generar", so between one press and the next this path
		// and the quote (ExpectedToday) are the only two things that price it.
		//
		// A payment that settles the line pays the price of its own date
		// (inscriptionPaidOn, task 7) — which is what the quote for that date
		// shows, since the quote is the amount that settles it. A part payment is
		// priced at TODAY, which keeps a back-dated peso from freezing the
		// earliest rung (SEC-2); for it the quote (a full settlement) and the
		// collection differ by design, and the preview shows the collection.
		settled, change, err := inscriptionPaidOn(c, p.tuition, p.enrolmentID, p.paidOn, p.svc.clock.Today())
		if err != nil {
			return ports.DepositWrite{}, err
		}
		// Whether the money settles it is asked at the price it would pay,
		// scholarship included (T18).
		settledPriced, err := p.scholarships.price(ports.ChargeWithPaid{
			Charge: settled, PaidCents: c.PaidCents, PaidByDueCents: c.PaidByDueCents}, p.paidOn)
		if err != nil {
			return ports.DepositWrite{}, err
		}
		switch {
		case change != priceUnchanged && budget >= settledPriced.Charge.Effective().Int64():
			// This payment settles the line at the price of its date.
			switch change {
			case priceRaised:
				repriced = append(repriced, settled)
			case priceLowered:
				discounted = append(discounted, settled)
			}
			c.Charge = settled
			base[i] = c
		default:
			if moved, ok, err := p.repriceInscription(c); err != nil {
				return ports.DepositWrite{}, err
			} else if ok {
				repriced = append(repriced, moved)
				c.Charge = moved
				base[i] = c
			}
		}

		// The scholarship on the price the ladder left, then early payment on
		// what the scholarship left (ADR 0022).
		priced, err := p.scholarships.price(c, p.paidOn)
		if err != nil {
			return ports.DepositWrite{}, err
		}
		charge, due := priced.Charge, owed(priced)
		reduced, discountable, err := p.earlyPayment(priced, budget)
		if err != nil {
			return ports.DepositWrite{}, err
		}
		if discountable {
			discounted = append(discounted, reduced)
			charge, due = reduced, reduced.Effective().Int64()-c.PaidCents
		}
		if charge.ScholarshipID() != "" && c.Charge.ScholarshipID() == "" {
			becas = append(becas, pendingScholarship{charge: charge, due: due})
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
			// A surcharge not yet accrued on the day the family paid was not
			// theirs to pay then — the quote for that day leaves it out too.
			if !expectedBy(fee.Charge.DueDate(), p.paidOn) {
				continue
			}
			items = append(items, domain.Payable{
				ChargeID:  fee.Charge.ID(),
				DueOn:     c.Charge.DueDate(),
				OwedCents: owed(fee),
				Surcharge: true,
			})
			budget -= min(budget, max(0, owed(fee)))
		}
		// Priced: a month its scholarship settled in time earns no surcharge.
		fee, ok, ferr := p.deriveSurcharge(priced, feesByCharge[c.Charge.ID()])
		if ferr != nil {
			return ports.DepositWrite{}, ferr
		}
		if ok {
			newFees[fee.ID()] = fee
			feeOwedBy = append(feeOwedBy, derivedFee{fee: fee, parentID: charge.ID(), parentDue: due})
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
	// Planned under the charges' lock, so a second cashier who finds the
	// month already paid is refused here, inside the transaction: no
	// deposit, no folio, no credit nobody applies (CC-202).
	if err := domain.RequireFullyApplied(p.deposit.Amount().Int64(), unallocated, p.chosen != nil); err != nil {
		return ports.DepositWrite{}, err
	}
	fresh = append(fresh, settledWithoutFee(feeOwedBy, allocations)...)
	scholarships := settledScholarships(becas, allocations)

	for _, a := range allocations {
		if fee, isNew := newFees[a.ChargeID]; isNew && !written(fresh, fee.ID()) {
			// A surcharge row exists where money reached it, or where its month
			// was settled (settledWithoutFee): an open month's fee is derived
			// again next time, so the ledger never fills with fees nobody asked for.
			fresh = append(fresh, fee)
		}
	}
	// Each allocation is cut where one tender ends and the next begins (T8), so
	// every payment row carries exactly one method.
	tendered := domain.SplitByTender(allocations, p.deposit.Tenders())
	payments := make([]domain.Payment, 0, len(tendered))
	for _, a := range tendered {
		payment, err := domain.NewPayment(p.svc.newID(), p.svc.tenantID, a.ChargeID,
			a.AmountCents, a.Tender.Method(), a.Tender.Reference(), p.deposit.PaidAt(), "")
		if err != nil {
			return ports.DepositWrite{}, err
		}
		if payment, err = payment.WithPaidOn(p.deposit.PaidOn()).WithCardType(a.Tender.CardType()); err != nil {
			return ports.DepositWrite{}, err
		}
		payments = append(payments, payment.WithReceivedBy(p.deposit.ReceivedBy().UserID))
	}
	return ports.DepositWrite{
		Repriced:         repriced,
		Surcharges:       fresh,
		Discounted:       discounted,
		Scholarships:     scholarships,
		Allocations:      payments,
		UnallocatedCents: unallocated,
	}, nil
}

// derivedFee is a daily surcharge worked out for this deposit, with the month it
// rides on and what that month owed before the money came in.
type derivedFee struct {
	fee       domain.Charge
	parentID  string
	parentDue int64
}

// settledWithoutFee returns the surcharges of the months this deposit settles
// whether or not money reached them (task 9). A settled month stops earning and
// is never derived again, so its surcharge must be written now: left out, it
// would be forgiven with no discount, no reason and no trace. A month left open
// keeps earning and is derived again on its next payment, so it writes nothing.
func settledWithoutFee(derived []derivedFee, allocations []domain.Allocation) []domain.Charge {
	paid := make(map[string]int64, len(allocations))
	for _, a := range allocations {
		paid[a.ChargeID] += a.AmountCents
	}
	var out []domain.Charge
	for _, d := range derived {
		// parentDue <= 0: the reward alone closed the month, no money needed.
		if paid[d.parentID] >= d.parentDue {
			out = append(out, d.fee)
		}
	}
	return out
}

// settledScholarships are the charges this deposit's money settles under a
// scholarship: their part is written now (T18). Money must reach the charge —
// a month the scholarship alone leaves owing nothing is settled by its due date,
// not by an unrelated payment months before (FreezeSettledScholarships), so a
// scholarship taken away before then still gives it back its price.
func settledScholarships(becas []pendingScholarship, allocations []domain.Allocation) []domain.Charge {
	paid := make(map[string]int64, len(allocations))
	for _, a := range allocations {
		paid[a.ChargeID] += a.AmountCents
	}
	var out []domain.Charge
	for _, b := range becas {
		if got := paid[b.charge.ID()]; got > 0 && got >= b.due {
			out = append(out, b.charge)
		}
	}
	return out
}

// written reports whether a surcharge is already among those to write.
func written(fees []domain.Charge, id string) bool {
	for _, f := range fees {
		if f.ID() == id {
			return true
		}
	}
	return false
}

// chosenSet is the cashier's selection as a set; nil when nothing was chosen.
func chosenSet(ids []string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// onlyChosen keeps the charges the cashier chose (task 8). A chosen id that is
// not one of this student's charges is refused rather than ignored: money must
// never be taken against a list the office did not mean.
func (p *depositPlanner) onlyChosen(base []ports.ChargeWithPaid) ([]ports.ChargeWithPaid, error) {
	if p.chosen == nil {
		return base, nil
	}
	kept := make([]ports.ChargeWithPaid, 0, len(p.chosen))
	for _, c := range base {
		if p.chosen[c.Charge.ID()] {
			kept = append(kept, c)
		}
	}
	if len(kept) != len(p.chosen) {
		return nil, glerrors.New(glerrors.KindInvalid, "a chosen charge is not one of this student's open charges")
	}
	return kept, nil
}

// partition splits the student's charges into the debts themselves and the
// surcharges already raised against each of them — daily or monthly, each
// riding on its month.
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
		if parent, ok := feeParent(c.Charge.ExternalRef()); ok {
			fees[parent] = append(fees[parent], c)
			continue
		}
		// A surcharge with no month to ride on (typed by hand) is a debt of
		// its own; left out, no deposit could ever pay it.
		base = append(base, c)
	}
	for parent := range fees {
		rows := fees[parent]
		// Oldest first, as every debt is paid; the id only breaks ties.
		sort.Slice(rows, func(i, j int) bool {
			if !rows[i].Charge.DueDate().Equal(rows[j].Charge.DueDate()) {
				return rows[i].Charge.DueDate().Before(rows[j].Charge.DueDate())
			}
			return rows[i].Charge.ID() < rows[j].Charge.ID()
		})
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
	// A surcharge never earns one of its own (a hand-typed one is a debt in
	// the list): that is how a fixed fee turns into compound interest.
	if !p.policy.DailyMode || owed(c) <= 0 || c.Charge.ConceptID() == p.lateFeeID {
		return domain.Charge{}, false, nil
	}
	// An activity answers to its own fixed fee (T17), never to the daily rule.
	if domain.IsActivityRef(c.Charge.ExternalRef()) {
		return domain.Charge{}, false, nil
	}
	total := domain.Surcharge(p.policy, p.calendar, c.Charge.DueDate(), p.paidOn)
	for _, fee := range existing {
		// Only what the DAILY rule already wrote down: a monthly surcharge on
		// the same month (a school that switched policy) is a debt of its own,
		// as the quote counts it (expectedFrom's materialised).
		if _, daily := surchargeParent(fee.Charge.ExternalRef()); daily {
			total -= fee.Charge.Effective().Int64()
		}
	}
	if total <= 0 {
		return domain.Charge{}, false, nil
	}
	// The ref ties the fee to the debt that earned it, which is how the next
	// deposit knows what has already been charged.
	ref := surchargeRef(c.Charge.ID(), p.deposit.ID())
	description := fmt.Sprintf("%s · %s", p.description, c.Charge.Description())
	if c.Charge.CycleID() == "" {
		// A sale to a pupil with no group yet (task 10) has no cycle, and
		// neither has the fee it earns; the ref is what it answers to.
		fee, err := domain.NewSaleCharge(p.svc.newID(), p.svc.tenantID, c.Charge.StudentID(),
			p.lateFeeID, description, total, c.Charge.DueDate(), ref)
		return fee, err == nil, err
	}
	fee, err := domain.NewCharge(p.svc.newID(), p.svc.tenantID, c.Charge.StudentID(), c.Charge.CycleID(),
		p.lateFeeID, description, total, 0, c.Charge.DueDate())
	if err != nil {
		return domain.Charge{}, false, err
	}
	fee, err = fee.WithExternalRef(ref)
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
	reason := domain.ReasonEarlyPayment
	if discount <= 0 && p.grantEarly {
		// Outside the window, but the school said yes. Same amount, different
		// reason, so a report can always tell the rule from the decision.
		discount, reason = p.policy.EarlyPaymentCents, domain.ReasonEarlyPaymentGranted
	}
	// Never more than the month still owes, as the quote caps it: a reward
	// larger than the balance would push the charge below what was paid.
	discount = min(discount, owed(c))
	if discount <= 0 || budget < owed(c)-discount {
		return domain.Charge{}, false, nil
	}
	reduced, err := c.Charge.WithDiscount(discount, c.PaidCents, reason)
	if err != nil {
		return domain.Charge{}, false, err
	}
	return reduced, true, nil
}

// surchargeRefPrefix is how a materialised daily surcharge names the charge it
// belongs to (domain.DailyFeeRefPrefix).
const surchargeRefPrefix = domain.DailyFeeRefPrefix

func surchargeRef(chargeID, depositID string) string {
	return surchargeRefPrefix + chargeID + ":" + depositID
}

// lateFeeRefPrefix is how the billing run names a monthly surcharge
// (domain.LateFeeRefPrefix).
const lateFeeRefPrefix = domain.LateFeeRefPrefix

// feeParent is the month a written-down surcharge belongs to, daily or monthly.
func feeParent(ref string) (string, bool) {
	if id, ok := surchargeParent(ref); ok {
		return id, true
	}
	if !strings.HasPrefix(ref, lateFeeRefPrefix) {
		return "", false
	}
	id, _, found := strings.Cut(strings.TrimPrefix(ref, lateFeeRefPrefix), ":")
	return id, found && id != ""
}

// surchargeParent reads back the charge a materialised DAILY surcharge belongs
// to.
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
	plan, err := s.planFor(ctx, studentID)
	if err != nil {
		return domain.CollectionPolicy{}, err
	}
	return plan.Policy(), nil
}

// planFor is the tuition table the student is priced by: through their active
// enrolment to the group, through the group to the level, and the level's table.
//
// A zero plan means the student sits nowhere the school has priced. Every caller
// treats that as "no special rule", which is what the deposit path did before
// there was a plan to find.
func (s *FinanceService) planFor(ctx context.Context, studentID string) (domain.TuitionPlan, error) {
	enrollment, err := s.activeEnrollment(ctx, studentID)
	if err != nil || enrollment.CycleID == "" {
		return domain.TuitionPlan{}, nil
	}
	levels, err := s.levelOfGroup(ctx, enrollment.CycleID)
	if err != nil {
		return domain.TuitionPlan{}, nil
	}
	plans, err := s.repo.ListTuitionPlans(ctx, enrollment.CycleID)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	for _, p := range plans {
		if p.LevelID() == levels[enrollment.GroupID] {
			return pricedFor(p, enrollment), nil
		}
	}
	return domain.TuitionPlan{}, nil
}

// pricedFor is the table as it prices this enrolment: for a pupil the legacy
// import brought in, the months that fell due before the import day accrue no
// surcharge (owner decision 2026-10-17, T14). The ONE place the grace is
// attached — planFor (statement, family account, counter quote, deposit) and
// the monthly run both go through it.
func pricedFor(plan domain.TuitionPlan, enr ports.EnrollmentRef) domain.TuitionPlan {
	if !enr.Imported() || enr.ImportedOn.IsZero() {
		return plan
	}
	return plan.WithSurchargeFreeBefore(enr.ImportedOn)
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

// depositEvents is what a deposit announces, queued INSIDE its transaction.
//
// ONE PaymentRegistered for the deposit, not one per month settled: a family
// that pays six months at once performed one act, and six notifications would
// read as six payments and buzz the phone six times.
//
// One ChargeSettled per charge the money CLOSED, and none for the ones it merely
// touched. That is what students listens to in order to turn a pupil from "en
// trámite" into "inscrito", and it is exactly the event that used to be lost
// when the broker was down — leaving a family paid and a pupil unenrolled with
// nothing in the system able to notice.
func (s *FinanceService) depositEvents(record ports.DepositRecord) []ports.OutboxEvent {
	events := []ports.OutboxEvent{
		outboxEvent(ports.OutboxPaymentRegistered, ports.PaymentRegisteredEvent{
			PaymentID:   record.Deposit.ID(),
			StudentID:   record.Deposit.StudentID(),
			TenantID:    s.tenantID,
			Method:      record.Deposit.Method(),
			AmountCents: record.Deposit.Amount().Int64(),
			PaidAt:      record.Deposit.PaidAt(),
		}),
	}
	for _, settled := range record.Settled {
		events = append(events, outboxEvent(ports.OutboxChargeSettled, ports.ChargeSettledEvent{
			ChargeID:    settled.Charge.ID(),
			StudentID:   settled.Charge.StudentID(),
			TenantID:    s.tenantID,
			ExternalRef: settled.Charge.ExternalRef(),
			AmountCents: settled.Charge.Effective().Int64(),
			SettledAt:   record.Deposit.PaidAt(),
		}))
	}
	return events
}
