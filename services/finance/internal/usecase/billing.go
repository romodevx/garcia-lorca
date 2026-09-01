package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/schoolcal"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// The names the school's system concepts are born with. They are a starting
// point, not an identity: the school may rename any of them, and the run keeps
// finding them because it looks them up by KIND (migration 0004).
const (
	tuitionConceptName    = "Colegiatura"
	enrollmentConceptName = "Inscripción"
	lateFeeConceptName    = "Recargo por pago tardío"
)

const enrolmentStatusActive = "active"

// TuitionPlanInput is a school's statement of what one level costs in one cycle.
type TuitionPlanInput struct {
	CycleID          string
	LevelID          string
	EnrollmentCents  int64
	MonthlyCents     int64
	Months           int
	FirstBilledMonth int
	DueDay           int
	LateFeeCents     int64
	Active           bool
	// Policy is the M16 collection rule. Zero value keeps the original monthly
	// surcharge, so a caller written before M16 goes on meaning what it meant.
	Policy domain.CollectionPolicy
	// Schedule replaces the uniform monthly payment with named amounts per
	// month. Empty keeps the uniform year.
	Schedule []domain.Installment
	// MonthlyDiscountCents comes off every month of this level — the school's
	// arrangement for maternal, or for any level it prices differently.
	MonthlyDiscountCents int64
}

// BillingResult is what a run did. Skipped counts the charges that already
// existed: on a second run in the same day that is everything, which is what
// idempotent looks like from the outside.
//
// LevelsWithoutPlan carries NAMES, not ids, because its only reader is a person
// who has to go and price that level.
type BillingResult struct {
	TuitionCreated    int
	EnrollmentCreated int
	LateFeesCreated   int
	Skipped           int
	LevelsWithoutPlan []string
}

// UpsertTuitionPlan sets what one level of one cycle costs, replacing whatever
// was there. The cycle is validated against config first: a tuition table for a
// school year that does not exist is a typo, and finding out in the billing run
// three months later is finding out too late.
func (s *FinanceService) UpsertTuitionPlan(ctx context.Context, in TuitionPlanInput) (domain.TuitionPlan, error) {
	if err := s.requireCycle(ctx, in.CycleID); err != nil {
		return domain.TuitionPlan{}, err
	}
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: s.newID(), TenantID: s.tenantID, CycleID: in.CycleID, LevelID: in.LevelID,
		EnrollmentCents: in.EnrollmentCents, MonthlyCents: in.MonthlyCents,
		Months: in.Months, FirstBilledMonth: in.FirstBilledMonth,
		DueDay: in.DueDay, LateFeeCents: in.LateFeeCents, Active: in.Active,
		Policy:               in.Policy,
		Schedule:             in.Schedule,
		MonthlyDiscountCents: in.MonthlyDiscountCents,
	})
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	return s.repo.UpsertTuitionPlan(ctx, plan)
}

// ListTuitionPlans returns the tuition table of a cycle.
func (s *FinanceService) ListTuitionPlans(ctx context.Context, cycleID string) ([]domain.TuitionPlan, error) {
	if strings.TrimSpace(cycleID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "specify a cycle")
	}
	return s.repo.ListTuitionPlans(ctx, cycleID)
}

// RunBilling generates everything the school is owed as of a date.
//
// Three things, in one pass: the monthly tuition whose due date has arrived, the
// inscription of anybody enrolled without one, and one fixed surcharge per
// completed overdue month on every unpaid charge.
//
// It is idempotent. Every charge it writes carries an external_ref unique per
// tenant (ADR 0008), so the bulk insert skips what already exists rather than
// failing, and running it twice on the same day charges nobody twice. That is
// the property that lets a button in the panel and a scheduler in the cloud both
// call it without coordinating.
//
// asOfDate is a date the run pretends it is, so a school can catch up a cycle it
// configured late — and so this is testable without waiting for a calendar. Empty
// means today.
func (s *FinanceService) RunBilling(ctx context.Context, cycleID, asOfDate string) (BillingResult, error) {
	cycle, err := s.cycles.ByID(ctx, cycleID)
	if err != nil {
		return BillingResult{}, err
	}
	asOf, err := resolveAsOf(asOfDate, s.now)
	if err != nil {
		return BillingResult{}, err
	}

	plans, err := s.repo.ListTuitionPlans(ctx, cycleID)
	if err != nil {
		return BillingResult{}, err
	}
	planByLevel := make(map[string]domain.TuitionPlan, len(plans))
	for _, p := range plans {
		if p.Active() {
			planByLevel[p.LevelID()] = p
		}
	}

	levelOfGroup, err := s.levelOfGroup(ctx, cycleID)
	if err != nil {
		return BillingResult{}, err
	}
	enrollments, err := s.enrollments.ListByCycle(ctx, cycleID, "")
	if err != nil {
		return BillingResult{}, err
	}

	concepts, err := s.systemConcepts(ctx)
	if err != nil {
		return BillingResult{}, err
	}

	// The school's closed days, so no charge falls due on one. A due date on a
	// day the office was shut is a date nobody could have paid on, and with the
	// surcharge counting every day it would charge a family for the school's own
	// holiday.
	calendar := schoolcal.New(nil)
	if s.calendar != nil {
		if snap, cerr := s.calendar.NonWorkingDays(ctx, cycleID); cerr == nil {
			calendar = schoolcal.New(snap.Days)
		}
	}

	// Kept apart so the counts reported back are exact. A single bulk insert
	// returns one number, and splitting it afterwards would be arithmetic
	// pretending to be knowledge.
	var wantedEnrollment, wantedTuition []domain.Charge
	for _, enr := range enrollments {
		if enr.Status != enrolmentStatusActive {
			continue
		}
		plan, ok := planByLevel[levelOfGroup[enr.GroupID]]
		if !ok {
			// No table for this pupil's level. Reported below by name, never
			// billed at zero: a family charged nothing is a problem a school
			// discovers in June.
			continue
		}

		enrolCharge, err := s.buildCharge(chargeSpec{
			studentID:   enr.StudentID,
			cycleID:     cycleID,
			conceptID:   concepts[domain.KindEnrollment].ID(),
			description: enrollmentConceptName,
			amount:      plan.EnrollmentCents(),
			due:         calendar.NextWorkingDay(enrollmentDueDate(cycle.StartsOn, plan)),
			ref:         fmt.Sprintf("enrollment:%s:%s", cycleID, enr.StudentID),
		})
		if err != nil {
			return BillingResult{}, err
		}
		wantedEnrollment = append(wantedEnrollment, enrolCharge)

		// The WHOLE year, not just the months that have arrived. The school asked
		// for "el monto total que debe pagar el año", and it is what makes a
		// deposit work: a family that hands over $6,000 in September has six
		// months marked paid, which cannot happen if those months are not
		// charges yet. Billing on arrival left the other five sitting as an
		// unexplained credit.
		//
		// A future month owes nothing extra: it is not overdue, so it accrues no
		// surcharge until its own due date passes.
		for _, item := range plan.Installments(cycle.StartsOn) {
			due := item.DueOn
			monthly, err := s.buildCharge(chargeSpec{
				studentID:   enr.StudentID,
				cycleID:     cycleID,
				conceptID:   concepts[domain.KindTuition].ID(),
				description: fmt.Sprintf("%s %s", tuitionConceptName, due.Format("2006-01")),
				// What THIS payment collects, which under a split schedule is
				// not the same every month.
				amount: item.AmountCents,
				// Moved off a closed day, but the ref below still names the
				// ORIGINAL month: the charge must stay the same charge even if
				// the calendar is edited afterwards.
				due: calendar.NextWorkingDay(due),
				ref: fmt.Sprintf("tuition:%s:%s:%s", cycleID, enr.StudentID, due.Format("2006-01")),
			})
			if err != nil {
				return BillingResult{}, err
			}
			if d := plan.MonthlyDiscountCents(); d > 0 {
				// The level's standing arrangement, applied by the run so nobody
				// types it per pupil. It says why, so a parent asking is
				// answered by the row itself.
				monthly, err = monthly.WithDiscount(d, 0, domain.ReasonLevelDiscount)
				if err != nil {
					return BillingResult{}, err
				}
			}
			wantedTuition = append(wantedTuition, monthly)
		}
	}

	enrollmentCreated, err := s.repo.CreateChargesBulk(ctx, wantedEnrollment)
	if err != nil {
		return BillingResult{}, err
	}
	tuitionCreated, err := s.repo.CreateChargesBulk(ctx, wantedTuition)
	if err != nil {
		return BillingResult{}, err
	}

	// Surcharges are computed AFTER the tuition is written, so a month that just
	// came due and is already overdue is caught on the same run rather than a
	// month later.
	lateFees, err := s.accrueLateFees(ctx, cycleID, asOf, planByLevel, levelOfGroup,
		enrollments, concepts[domain.KindLateFee].ID())
	if err != nil {
		return BillingResult{}, err
	}
	lateCreated, err := s.repo.CreateChargesBulk(ctx, lateFees)
	if err != nil {
		return BillingResult{}, err
	}

	wanted := len(wantedEnrollment) + len(wantedTuition) + len(lateFees)
	created := enrollmentCreated + tuitionCreated + lateCreated
	return BillingResult{
		TuitionCreated:    tuitionCreated,
		EnrollmentCreated: enrollmentCreated,
		LateFeesCreated:   lateCreated,
		Skipped:           wanted - created,
		LevelsWithoutPlan: s.levelsWithoutPlan(ctx, planByLevel),
	}, nil
}

// systemConcepts resolves the school's tuition, inscription and surcharge
// concepts, creating any it has not got yet.
func (s *FinanceService) systemConcepts(ctx context.Context) (map[string]domain.ChargeConcept, error) {
	out := make(map[string]domain.ChargeConcept, 3)
	for kind, name := range map[string]string{
		domain.KindTuition:    tuitionConceptName,
		domain.KindEnrollment: enrollmentConceptName,
		domain.KindLateFee:    lateFeeConceptName,
	} {
		c, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), kind, name)
		if err != nil {
			return nil, err
		}
		out[kind] = c
	}
	return out, nil
}

// levelOfGroup resolves every group of the cycle to its level, in one call. An
// enrolment names a GROUP and a price is set per LEVEL; the path between them
// (group → grade → level) belongs to config, so finance asks rather than guesses.
func (s *FinanceService) levelOfGroup(ctx context.Context, cycleID string) (map[string]string, error) {
	pairs, err := s.catalog.GroupLevels(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p.GroupID] = p.LevelID
	}
	return out, nil
}

// levelsWithoutPlan names the levels the school has not priced. Names, because
// the only reader is a person who has to go and price them.
func (s *FinanceService) levelsWithoutPlan(ctx context.Context, planByLevel map[string]domain.TuitionPlan) []string {
	levels, err := s.catalog.ListLevels(ctx)
	if err != nil {
		// A run that billed correctly must not fail over a courtesy report.
		return nil
	}
	var out []string
	for _, l := range levels {
		if _, ok := planByLevel[l.ID]; !ok {
			out = append(out, l.Name)
		}
	}
	return out
}

// overduePageSize is how many overdue charges one page of the walk fetches. It
// exists so the run holds a bounded slice in memory per round trip, NOT to bound
// the total: allOverdue keeps going until the pages run out.
const overduePageSize = 500

// allOverdue walks every overdue charge of the cycle, not the first page of them.
//
// This used to call ListOverdue, which caps at 500 rows — right for a screen,
// wrong here. A school with 400 pupils and eleven monthly charges crosses that
// cap inside its first cycle, and past it the run stopped accruing surcharges on
// the newest debt without a word. The school under-billed and found out from a
// parent, if at all.
//
// Stops on an EMPTY page, never on a short one: the repository applies the
// still-owing filter after fetching a page, so a page can come back with fewer
// rows than requested while later pages still hold debt.
func (s *FinanceService) allOverdue(ctx context.Context, cycleID string, asOf time.Time) ([]ports.ChargeWithPaid, error) {
	var (
		out    []ports.ChargeWithPaid
		cursor ports.OverdueCursor
	)
	for {
		page, err := s.repo.ListOverduePage(ctx, cycleID, asOf, cursor, overduePageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return out, nil
		}
		out = append(out, page...)
		last := page[len(page)-1].Charge
		cursor = ports.OverdueCursor{DueDate: last.DueDate(), ID: last.ID()}
	}
}

// accrueLateFees issues one fixed surcharge per completed overdue month, per
// unpaid charge.
//
// Surcharges are excluded from their own input. A fee that earned fees would
// quietly turn a fixed monthly charge into compound interest — the school agreed
// to $150 a month, not to $150 that grows.
func (s *FinanceService) accrueLateFees(
	ctx context.Context,
	cycleID string,
	asOf time.Time,
	planByLevel map[string]domain.TuitionPlan,
	levelOfGroup map[string]string,
	enrollments []ports.EnrollmentRef,
	lateFeeConceptID string,
) ([]domain.Charge, error) {
	overdue, err := s.allOverdue(ctx, cycleID, asOf)
	if err != nil {
		return nil, err
	}
	planOfStudent := make(map[string]domain.TuitionPlan, len(enrollments))
	for _, enr := range enrollments {
		if plan, ok := planByLevel[levelOfGroup[enr.GroupID]]; ok {
			planOfStudent[enr.StudentID] = plan
		}
	}

	var out []domain.Charge
	for _, c := range overdue {
		if c.Charge.ConceptID() == lateFeeConceptID {
			continue
		}
		plan, ok := planOfStudent[c.Charge.StudentID()]
		if !ok || plan.LateFeeCents() == 0 {
			continue
		}
		if plan.Policy().DailyMode {
			// The daily rule is derived at the moment of payment, never written
			// down. What a family owes in surcharge changes every night: a charge
			// issued today would be a wrong number sitting in their balance
			// tomorrow, and the run would have to chase and correct it forever.
			continue
		}
		months := domain.OverdueMonths(c.Charge.DueDate(), asOf)
		for i := 1; i <= months; i++ {
			accruedAt := c.Charge.DueDate().AddDate(0, i, 0)
			fee, err := s.buildCharge(chargeSpec{
				studentID:   c.Charge.StudentID(),
				cycleID:     cycleID,
				conceptID:   lateFeeConceptID,
				description: fmt.Sprintf("%s · %s", lateFeeConceptName, c.Charge.Description()),
				amount:      plan.LateFeeCents(),
				due:         accruedAt,
				ref:         fmt.Sprintf("latefee:%s:%s", c.Charge.ID(), accruedAt.Format("2006-01")),
			})
			if err != nil {
				return nil, err
			}
			out = append(out, fee)
		}
	}
	return out, nil
}

// chargeSpec is what the run knows about a charge before the domain validates it.
type chargeSpec struct {
	studentID   string
	cycleID     string
	conceptID   string
	description string
	amount      int64
	due         time.Time
	ref         string
}

// buildCharge mints one generated charge, always with an external ref: the ref is
// what makes the run idempotent, so a generated charge without one would be a
// charge that can be issued twice.
func (s *FinanceService) buildCharge(spec chargeSpec) (domain.Charge, error) {
	charge, err := domain.NewCharge(s.newID(), s.tenantID, spec.studentID, spec.cycleID,
		spec.conceptID, spec.description, spec.amount, 0, spec.due)
	if err != nil {
		return domain.Charge{}, err
	}
	return charge.WithExternalRef(spec.ref)
}

// enrollmentDueDate is the day the inscription falls due: the plan's due day in
// the month the school year opens. A family owes it from the start of the year,
// not from the first tuition.
func enrollmentDueDate(cycleStart time.Time, plan domain.TuitionPlan) time.Time {
	return time.Date(cycleStart.Year(), cycleStart.Month(), plan.DueDay(), 0, 0, 0, 0, cycleStart.Location())
}

// resolveAsOf reads the date the run should pretend it is. An empty value means
// today; anything else must be a date, because a run that silently fell back to
// today after a typo would bill the wrong months without saying so.
func resolveAsOf(raw string, now func() time.Time) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		t := now().UTC()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "the date must be YYYY-MM-DD")
	}
	return parsed, nil
}
