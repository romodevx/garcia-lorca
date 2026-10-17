package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
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
	feeConceptName        = "Cuota"
)

// monthNamesES names the months the way the school says them out loud.
//
// "Colegiatura 2026-09" is a machine's idea of a month. These rows are read to
// parents over the phone and printed on statements, and nobody on the other end
// of that call is holding a calendar to decode 09.
//
// Spanish and not a locale lookup: this school is in Mexico and every other
// string the family reads is already Spanish. A translation layer here would be
// one indirection standing in front of one answer.
var monthNamesES = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// monthInWords is "septiembre 2026".
func monthInWords(t time.Time) string {
	return fmt.Sprintf("%s %d", monthNamesES[int(t.Month())-1], t.Year())
}

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
	// Fees are the extra amounts collected on top of the tuition, each split
	// across the months the school chose.
	Fees []domain.Fee
	// EnrollmentDueOn is the deadline the school set for the inscription. Zero
	// keeps the rule the run has always used.
	EnrollmentDueOn time.Time
	// EnrollmentStartsOn is the first day the inscription opens to families
	// (T5). Zero keeps it open from the day it is issued.
	EnrollmentStartsOn time.Time
	// EnrollmentConcepts breaks the inscription into the lines the school prints,
	// each with its own price ladder. Empty keeps EnrollmentCents.
	EnrollmentConcepts []domain.EnrollmentConcept
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
	FeesCreated       int
	// ActivitiesCreated counts the activity periods issued (T17). Their fixed
	// late fees count in LateFeesCreated.
	ActivitiesCreated int
	// EnrollmentMoved counts inscriptions whose deadline the run corrected to
	// match the tuition table. Reported because a run that silently moves four
	// hundred due dates is one nobody can audit.
	EnrollmentMoved int
	// EnrollmentRepriced counts unpaid inscriptions the run moved to the tier now
	// in force. Reported for the same reason as EnrollmentMoved, and it matters
	// more: this one moves money, not dates.
	EnrollmentRepriced int
	Skipped            int
	LevelsWithoutPlan  []string
}

// UpsertTuitionPlan sets what one level of one cycle costs, replacing whatever
// was there. The cycle is validated against config first: a tuition table for a
// school year that does not exist is a typo, and finding out in the billing run
// three months later is finding out too late.
func (s *FinanceService) UpsertTuitionPlan(ctx context.Context, in TuitionPlanInput) (domain.TuitionPlan, error) {
	if err := s.requireCycle(ctx, in.CycleID); err != nil {
		return domain.TuitionPlan{}, err
	}
	if err := s.refuseDeletingLevel(ctx, in.LevelID); err != nil {
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
		Fees:                 in.Fees,
		EnrollmentDueOn:      in.EnrollmentDueOn,
		EnrollmentStartsOn:   in.EnrollmentStartsOn,
		EnrollmentConcepts:   in.EnrollmentConcepts,
	})
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	if err := plan.SharedLineName(); err != nil {
		return domain.TuitionPlan{}, err
	}
	// Everything the re-dating needs is read BEFORE anything is written: a
	// failure there (students scaled to zero, an unreadable ledger) refuses the
	// save whole, instead of answering "no se pudo guardar" over a plan that was.
	moves, err := s.inscriptionMoves(ctx, plan)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	saved, err := s.repo.UpsertTuitionPlan(ctx, plan)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	// The one write left. Should it fail the plan is already stored, the error
	// says so to the caller, a retry is idempotent, and the next run re-dates
	// the same charges anyway (RunBilling reschedules every inscription).
	if _, err := s.repo.RescheduleUnpaid(ctx, moves); err != nil {
		return domain.TuitionPlan{}, err
	}
	return saved, nil
}

// inscriptionMoves is the window each inscription of the plan's level should
// carry once the plan is saved (T5), so saving carries the dates to the charges
// still owing at once rather than at the next run: a director who moves a
// deadline expects the families to see the new date, not to learn that nothing
// happens until somebody presses "generar".
//
// Only dates move, and only through RescheduleUnpaid's guards — a settled
// charge is never touched. Nothing is issued here; charges not yet issued are
// issued by the run, with the dates in force when it runs.
func (s *FinanceService) inscriptionMoves(ctx context.Context, plan domain.TuitionPlan) ([]ports.ChargeReschedule, error) {
	if !plan.Active() {
		return nil, nil
	}
	cycle, err := s.cycles.ByID(ctx, plan.CycleID())
	if err != nil {
		return nil, err
	}
	levelOfGroup, err := s.levelOfGroup(ctx, plan.CycleID())
	if err != nil {
		return nil, err
	}
	enrollments, err := s.enrollments.ListByCycle(ctx, plan.CycleID(), "")
	if err != nil {
		return nil, err
	}
	concept, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindEnrollment, enrollmentConceptName)
	if err != nil {
		return nil, err
	}
	asOf, err := resolveAsOf("", s.clock.Today)
	if err != nil {
		return nil, err
	}
	wanted, err := s.wantedInscriptions(plan.CycleID(), enrollments,
		map[string]domain.TuitionPlan{plan.LevelID(): plan}, levelOfGroup,
		cycle.StartsOn, concept.ID(), asOf, s.closedDays(ctx, plan.CycleID()))
	if err != nil || len(wanted) == 0 {
		return nil, err
	}
	issuedRows, err := s.repo.ListIssuedLines(ctx, plan.CycleID(), "")
	if err != nil {
		return nil, err
	}
	wanted, _, err = newIssuedLines(issuedRows).adoptAll(wanted)
	if err != nil {
		return nil, err
	}
	return reschedules(wanted), nil
}

// reschedules is the window each wanted inscription should have.
func reschedules(charges []domain.Charge) []ports.ChargeReschedule {
	out := make([]ports.ChargeReschedule, 0, len(charges))
	for _, c := range charges {
		out = append(out, ports.ChargeReschedule{ExternalRef: c.ExternalRef(), StartsOn: c.StartsOn(), DueOn: c.DueDate()})
	}
	return out
}

// closedDays is the school's calendar for a cycle, so no charge falls due on one.
// A due date on a day the office was shut is a date nobody could have paid on,
// and with the surcharge counting every day it would charge a family for the
// school's own holiday. Best effort: a calendar that cannot be read leaves every
// day open, as it always has.
func (s *FinanceService) closedDays(ctx context.Context, cycleID string) *schoolcal.Calendar {
	if s.calendar != nil {
		if snap, err := s.calendar.NonWorkingDays(ctx, cycleID); err == nil {
			return schoolcal.New(snap.Days)
		}
	}
	return schoolcal.New(nil)
}

// wantedInscriptions is every inscription line the active pupils of the priced
// levels owe, priced as of asOf and dated by their plan's window. Shared by the
// run and by the plan's save, so both reach the same rows under the same refs.
func (s *FinanceService) wantedInscriptions(
	cycleID string,
	enrollments []ports.EnrollmentRef,
	planByLevel map[string]domain.TuitionPlan,
	levelOfGroup map[string]string,
	cycleStart time.Time,
	conceptID string,
	asOf time.Time,
	calendar *schoolcal.Calendar,
) ([]domain.Charge, error) {
	var out []domain.Charge
	for _, enr := range enrollments {
		// An imported pupil was inscribed by the previous system (ADR 0033).
		if enr.Status != enrolmentStatusActive || enr.Imported() {
			continue
		}
		plan, ok := planByLevel[levelOfGroup[enr.GroupID]]
		if !ok {
			continue
		}
		// One charge per line the school prints, priced at the tier in force
		// TODAY. Asking the plan for EnrollmentCents here instead would issue a
		// charge for NOTHING on any plan that carries concepts — it is forced to
		// zero (ADR 0011), NewCents(0) is legal, and the bulk insert makes that
		// zero permanent: a family charged nothing, a problem a school
		// discovers in June.
		lines, err := s.enrollmentCharges(enr.StudentID, cycleID, conceptID,
			plan, asOf, inscriptionDueDate(cycleStart, plan, enr.EnrolledAt, calendar))
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
}

// refuseDeletingLevel refuses a plan for a level whose deletion runs
// (LEVEL_BEING_DELETED, ADR 0021). It reads config's level list — the one
// level lookup finance makes; a level the list does not name is left to the
// plan's own validation, as before.
func (s *FinanceService) refuseDeletingLevel(ctx context.Context, levelID string) error {
	levels, err := s.catalog.ListLevels(ctx)
	if err != nil {
		return err
	}
	for _, l := range levels {
		if l.ID == levelID && l.Deleting {
			return levelarchive.LevelBeingDeleted()
		}
	}
	return nil
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
	asOf, err := resolveAsOf(asOfDate, s.clock.Today)
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
	calendar := s.closedDays(ctx, cycleID)

	// Kept apart so the counts reported back are exact. A single bulk insert
	// returns one number, and splitting it afterwards would be arithmetic
	// pretending to be knowledge.
	wantedEnrollment, err := s.wantedInscriptions(cycleID, enrollments, planByLevel, levelOfGroup,
		cycle.StartsOn, concepts[domain.KindEnrollment].ID(), asOf, calendar)
	if err != nil {
		return BillingResult{}, err
	}
	var wantedTuition, wantedFees []domain.Charge
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

		// The WHOLE year, not just the months that have arrived. The school asked
		// for "el monto total que debe pagar el año", and it is what makes a
		// deposit work: a family that hands over $6,000 in September has six
		// months marked paid, which cannot happen if those months are not
		// charges yet. Billing on arrival left the other five sitting as an
		// unexplained credit.
		//
		// A future month owes nothing extra: it is not overdue, so it accrues no
		// surcharge until its own due date passes.
		// From the month they arrived, not from the month the year opened. A
		// pupil who joined in March was not taught September.
		for _, item := range plan.InstallmentsFrom(cycle.StartsOn, enr.EnrolledAt) {
			due := item.DueOn
			monthly, err := s.buildCharge(chargeSpec{
				studentID: enr.StudentID,
				cycleID:   cycleID,
				conceptID: concepts[domain.KindTuition].ID(),
				// In words, for the person reading it. The ref below stays
				// "2006-01": it is the charge's identity, and renaming a month
				// must never re-issue it.
				description: fmt.Sprintf("%s %s", tuitionConceptName, monthInWords(due)),
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

		// The extras the school collects on top of the tuition, each already
		// divided between the months it chose. They are separate charges and
		// not added into the month's tuition on purpose: a family that owes
		// $4,000 in September is owed an explanation of the $800, and a tuition
		// row that no longer matches the tabulador is one nobody can reconcile.
		for _, item := range feesOwed(plan, cycle.StartsOn, enr) {
			charge, err := s.buildCharge(chargeSpec{
				studentID:   enr.StudentID,
				cycleID:     cycleID,
				conceptID:   concepts[domain.KindFee].ID(),
				description: feeDescription(item),
				amount:      item.AmountCents,
				due:         calendar.NextWorkingDay(item.DueOn),
				ref: fmt.Sprintf("%s%s:%s:%s:%s", domain.FeeRefPrefix, cycleID, enr.StudentID,
					feeSlug(item.Name), item.DueOn.Format("2006-01")),
			})
			if err != nil {
				return BillingResult{}, err
			}
			wantedFees = append(wantedFees, charge)
		}
	}

	// What the ledger already holds under another reference is issued under
	// THAT reference — see issuedLines. One read for the whole cycle.
	issuedRows, err := s.repo.ListIssuedLines(ctx, cycleID, "")
	if err != nil {
		return BillingResult{}, err
	}
	issued := newIssuedLines(issuedRows)
	wantedEnrollment, priceableEnrollment, err := issued.adoptAll(wantedEnrollment)
	if err != nil {
		return BillingResult{}, err
	}
	if wantedFees, _, err = issued.adoptAll(wantedFees); err != nil {
		return BillingResult{}, err
	}

	enrollmentCreated, err := s.repo.CreateChargesBulk(ctx, wantedEnrollment)
	if err != nil {
		return BillingResult{}, err
	}
	// The inscriptions that already existed keep their original due date, and
	// the school may have changed the deadline since. Correcting it is the only
	// way the setting reaches a pupil who was billed before it was typed — the
	// bulk insert above skips them by design.
	//
	// Only the inscription, and only when it is unpaid. Tuition months are not
	// re-dated: moving eleven deadlines because somebody adjusted the due day is
	// a bigger thing than a school expects from pressing "generar".
	enrollmentMoved, err := s.repo.RescheduleUnpaid(ctx, reschedules(wantedEnrollment))
	if err != nil {
		return BillingResult{}, err
	}
	// The ladder goes on climbing after the charge exists. Written down rather
	// than left derived, so the database agrees with what the family is shown and
	// no report quotes a price nobody is charging. Only the unpaid and
	// un-negotiated ones move — see repriceable below.
	enrollmentRepriced, err := s.repo.RepriceUnpaid(ctx, repriceable(priceableEnrollment))
	if err != nil {
		return BillingResult{}, err
	}
	tuitionCreated, err := s.repo.CreateChargesBulk(ctx, wantedTuition)
	if err != nil {
		return BillingResult{}, err
	}
	feesCreated, err := s.repo.CreateChargesBulk(ctx, wantedFees)
	if err != nil {
		return BillingResult{}, err
	}

	// T17: every activity period whose billing date has arrived, for the pupils
	// active in this cycle. Same idempotency: one ref per pupil and period.
	activities, err := s.loadActivityBilling(ctx)
	if err != nil {
		return BillingResult{}, err
	}
	wantedActivities, err := s.wantedActivityCharges(ctx, activities, cycleID, activeStudents(enrollments), asOf, calendar)
	if err != nil {
		return BillingResult{}, err
	}
	activitiesCreated, err := s.repo.CreateChargesBulk(ctx, wantedActivities)
	if err != nil {
		return BillingResult{}, err
	}

	// Surcharges are computed AFTER the tuition is written, so a month that just
	// came due and is already overdue is caught on the same run rather than a
	// month later.
	lateFees, err := s.accrueLateFees(ctx, cycleID, asOf, planByLevel, levelOfGroup,
		enrollments, concepts[domain.KindLateFee].ID(), activities.lateFees)
	if err != nil {
		return BillingResult{}, err
	}
	lateCreated, err := s.repo.CreateChargesBulk(ctx, lateFees)
	if err != nil {
		return BillingResult{}, err
	}

	wanted := len(wantedEnrollment) + len(wantedTuition) + len(wantedFees) + len(wantedActivities) + len(lateFees)
	created := enrollmentCreated + tuitionCreated + feesCreated + activitiesCreated + lateCreated
	return BillingResult{
		TuitionCreated:     tuitionCreated,
		EnrollmentCreated:  enrollmentCreated,
		LateFeesCreated:    lateCreated,
		FeesCreated:        feesCreated,
		ActivitiesCreated:  activitiesCreated,
		EnrollmentMoved:    enrollmentMoved,
		EnrollmentRepriced: enrollmentRepriced,
		Skipped:            wanted - created,
		LevelsWithoutPlan:  s.levelsWithoutPlan(ctx, planByLevel),
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
		domain.KindFee:        feeConceptName,
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

// allOverdue walks every overdue charge of the cycle, not the first page of them.
//
// This used to call ListOverdue, which caps at 500 rows — right for a screen,
// wrong here. A school with 400 pupils and eleven monthly charges crosses that
// cap inside its first cycle, and past it the run stopped accruing surcharges on
// the newest debt without a word. The school under-billed and found out from a
// parent, if at all.
//
// Follows the repository's Next cursor, never the last row returned: the
// still-owing filter runs after a page is fetched, so a page of settled charges
// comes back empty while later pages still hold debt. Stopping on that empty
// page was the same silent under-billing again
// (TestRunBillingAccruesPastAPageOfSettledCharges).
//
// It is the same walk ListOverduePage serves the screen, scholarship pricing
// included: a month its scholarship settled in time is not late (T18).
func (s *FinanceService) allOverdue(ctx context.Context, cycleID string, asOf time.Time) ([]ports.ChargeWithPaid, error) {
	return walkPages(func(after ports.ChargeCursor) (ports.ChargePage, error) {
		return s.overduePage(ctx, cycleID, asOf, after, ports.MaxChargePageSize)
	})
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
	activityLateFees map[string]int64,
) ([]domain.Charge, error) {
	overdue, err := s.allOverdue(ctx, cycleID, asOf)
	if err != nil {
		return nil, err
	}
	// A pupil with two enrolments in the cycle (imported, withdrawn, enrolled
	// again by hand) is priced by the ACTIVE one, as planFor does: the import
	// grace belongs to the enrolment that carried it, whatever the list order.
	planOfStudent := make(map[string]domain.TuitionPlan, len(enrollments))
	activePlan := make(map[string]bool, len(enrollments))
	for _, enr := range enrollments {
		plan, ok := planByLevel[levelOfGroup[enr.GroupID]]
		active := enr.Status == enrolmentStatusActive
		if !ok || (activePlan[enr.StudentID] && !active) {
			continue
		}
		planOfStudent[enr.StudentID] = pricedFor(plan, enr)
		activePlan[enr.StudentID] = active
	}

	var out []domain.Charge
	for _, c := range overdue {
		if c.Charge.ConceptID() == lateFeeConceptID {
			continue
		}
		if domain.IsActivityRef(c.Charge.ExternalRef()) {
			// An activity answers to its own fixed fee (T17), never to the
			// tuition table's rule.
			fee, ok, err := s.activityLateFee(c.Charge, cycleID, lateFeeConceptID, activityLateFees)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, fee)
			}
			continue
		}
		plan, ok := planOfStudent[c.Charge.StudentID()]
		if !ok || plan.LateFeeCents() == 0 {
			continue
		}
		if plan.Policy().SurchargeFree(c.Charge.DueDate()) {
			// Due before the pupil was imported: normal price, no surcharge (T14).
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
				ref:         fmt.Sprintf("%s%s:%s", domain.LateFeeRefPrefix, c.Charge.ID(), accruedAt.Format("2006-01")),
			})
			if err != nil {
				return nil, err
			}
			out = append(out, fee)
		}
	}
	return out, nil
}

// activeStudents is the set of pupils actively enrolled in the cycle.
func activeStudents(enrollments []ports.EnrollmentRef) map[string]bool {
	out := make(map[string]bool, len(enrollments))
	for _, e := range enrollments {
		if e.Status == enrolmentStatusActive {
			out[e.StudentID] = true
		}
	}
	return out
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

// enrollmentCharges is what one pupil owes for joining, as of a day: one charge
// per line the school prints, each at the tier in force.
//
// Shared by the run and the enrolment consumer, so a pupil billed by the event
// and a pupil billed by the button get the same rows under the same references.
// Two implementations of this would be two price lists.
//
// The reference carries the concept's SLUG, so three lines are three identities
// and the run stays idempotent (ADR 0008 + 0011). A plan with no concepts keeps
// the reference it has always had, so nothing already issued is re-billed under
// a new name.
func (s *FinanceService) enrollmentCharges(
	studentID, cycleID, conceptID string,
	plan domain.TuitionPlan, asOf, due time.Time,
) ([]domain.Charge, error) {
	lines := plan.EnrollmentLinesOn(asOf)
	out := make([]domain.Charge, 0, len(lines))
	for _, line := range lines {
		description, ref := enrollmentConceptName, fmt.Sprintf("%s%s:%s", domain.EnrollmentRefPrefix, cycleID, studentID)
		if line.Name != "" {
			description = line.Name
			// The fallback Slugify's own doc says the caller supplies. A name of
			// nothing but punctuation would otherwise end the reference in a
			// colon and say nothing about which line it is.
			slug := domain.Slugify(line.Name)
			if slug == "" {
				slug = "concepto"
			}
			ref = fmt.Sprintf("%s%s:%s:%s", domain.EnrollmentRefPrefix, cycleID, studentID, slug)
		}
		charge, err := s.buildCharge(chargeSpec{
			studentID: studentID, cycleID: cycleID, conceptID: conceptID,
			description: description, amount: line.AmountCents, due: due, ref: ref,
		})
		if err != nil {
			return nil, err
		}
		// The window the school set (T5). The due date here is never before the
		// plan's own deadline — it only moves later, for a late joiner or off a
		// closed day — so the plan's start still precedes it.
		if charge, err = charge.WithStartsOn(plan.EnrollmentStartsOn()); err != nil {
			return nil, err
		}
		out = append(out, charge)
	}
	return out, nil
}

// repriceable turns the inscriptions the run WOULD issue into the re-pricing of
// the ones already there.
//
// The repository refuses to touch a charge that has taken money — the first peso
// freezes the price (ADR 0011) — and it also refuses one carrying a discount. A
// discount is a price a PERSON decided for one family, and a price list must
// never overwrite a decision somebody made by hand. The full scholarship is what
// proves the rule: a free inscription never receives a peso, so the payment
// guard never protects it, and raising the amount while the discount stands
// would turn a gift into a debt.
func repriceable(charges []domain.Charge) []ports.ChargeReprice {
	out := make([]ports.ChargeReprice, 0, len(charges))
	for _, c := range charges {
		out = append(out, ports.ChargeReprice{
			ExternalRef: c.ExternalRef(),
			AmountCents: c.Amount().Int64(),
		})
	}
	return out
}

// inscriptionDueDate is THE day an inscription falls due, for every path that
// issues one — the run and the StudentEnrolled consumer (T14-022): the plan's
// deadline or the join day, moved to the next day the school is open. When the
// consumer skipped the calendar, a pupil enrolled on a Saturday got a Saturday
// deadline and the next run "moved" it to Monday.
func inscriptionDueDate(cycleStart time.Time, plan domain.TuitionPlan, joinedOn time.Time, calendar *schoolcal.Calendar) time.Time {
	return calendar.NextWorkingDay(enrollmentDueDate(cycleStart, plan, joinedOn))
}

// enrollmentDueDate is the day the inscription falls due: the plan's due day in
// the month the school year opens. A family owes it from the start of the year,
// not from the first tuition.
func enrollmentDueDate(cycleStart time.Time, plan domain.TuitionPlan, joinedOn time.Time) time.Time {
	// The school's own deadline wins. The inscription is a separate charge with
	// a separate arrangement — collected in June for a year that opens in
	// August — and deriving it from the cycle made that impossible to say.
	due := plan.EnrollmentDueOn()
	if due.IsZero() {
		due = time.Date(cycleStart.Year(), cycleStart.Month(), plan.DueDay(), 0, 0, 0, 0, cycleStart.Location())
	}
	// A pupil who arrives in March is not six months late for it. The tuition
	// already refuses to charge them for months they were not a pupil
	// (InstallmentsFrom); the inscription was still handed to them dated in
	// September, overdue the day it was issued, and accrueLateFees does not spare
	// inscriptions — six surcharges on the first run.
	//
	// Unlike the tuition, this deadline may leave its calendar month: the
	// inscription's external_ref carries no month, so the charge's identity does
	// not move with it.
	if !joinedOn.IsZero() && joinedOn.After(due) {
		return joinedOn
	}
	return due
}

// feeDescription is what the family reads. A fee collected in one month is just
// the fee; a split one says which part it is, so two rows for the same fee are
// not mistaken for a double bill.
func feeDescription(i domain.FeeInstallment) string {
	if i.Of <= 1 {
		return i.Name
	}
	return fmt.Sprintf(feePartFormat, i.Name, i.Part, i.Of)
}

// feePartFormat is how feeDescription names a share of a split fee, and
// feePartSuffix reads it back. Kept side by side because feeNameOf depends on
// the wording: if it drifts, re-spelled fees are issued twice again
// (TestRespellingAFeeDoesNotIssueItAgain splits its fee to guard exactly that).
const feePartFormat = "%s (%d de %d)"

var feePartSuffix = regexp.MustCompile(`\s*\(\d+ de \d+\)$`)

// feeNameOf is the fee's name inside a description feeDescription wrote.
func feeNameOf(description string) string {
	return feePartSuffix.ReplaceAllString(description, "")
}

// feeSlug is the fee's stable identity inside a charge ref.
//
// It is derived from the NAME because that is all a fee has — the list lives in
// the plan's JSON and its entries carry no id. Lower-cased and stripped of
// everything but letters and digits so that fixing the capitalisation of "Cuota
// de Julio" does not re-issue the whole school's charges.
func feeSlug(name string) string {
	b := domain.Slugify(name)
	if b == "" {
		// A name of nothing but punctuation still needs an identity, or every
		// such fee would collide with every other.
		return "cuota"
	}
	return b
}

// resolveAsOf reads the date the run should pretend it is. An empty value means
// the school's today (ADR 0035); anything else must be a date, because a run
// that silently fell back to today after a typo would bill the wrong months
// without saying so.
func resolveAsOf(raw string, today func() time.Time) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return today(), nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "the date must be YYYY-MM-DD")
	}
	return parsed, nil
}

// feesOwed is the fee list a pupil owes. Everybody enrolled by hand owes the
// whole list, as before; a pupil brought over by the legacy import owes only
// what falls due from the month they arrived — the previous system collected
// the rest (owner decision O8, ADR 0033).
func feesOwed(plan domain.TuitionPlan, cycleStart time.Time, enr ports.EnrollmentRef) []domain.FeeInstallment {
	if enr.Imported() {
		return plan.FeeInstallmentsFrom(cycleStart, enr.EnrolledAt)
	}
	return plan.FeeInstallments(cycleStart)
}
