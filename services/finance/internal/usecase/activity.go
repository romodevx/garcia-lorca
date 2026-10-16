package usecase

import (
	"context"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// maxEnrollBatch bounds one enrol or unenrol request: a whole grade at once,
// and a ceiling on the cross-service calls one request makes.
const maxEnrollBatch = 200

// ActivityBiller issues the periods already billable for enrolments just made,
// so the office does not wait for the next run to collect (FinanceService).
type ActivityBiller interface {
	BillActivityEnrollments(ctx context.Context, a domain.Activity, es []domain.ActivityEnrollment) (int, error)
}

// ActivityService runs T17's activities: the catalog, the price options and the
// pupils enrolled (ADR 0031). Their charges are the billing run's; enrolling
// asks the biller for the periods already due.
type ActivityService struct {
	repo     ports.ActivityRepository
	students ports.StudentReader
	biller   ActivityBiller
	newID    func() string
	clock    SchoolClock
}

// NewActivityService wires the use case.
func NewActivityService(repo ports.ActivityRepository, students ports.StudentReader, biller ActivityBiller,
	newID func() string, clock SchoolClock,
) *ActivityService {
	return &ActivityService{repo: repo, students: students, biller: biller, newID: newID, clock: clock}
}

// ActivityInput is what the office types about an activity. An option with an
// empty ID is new.
type ActivityInput struct {
	Name            string
	Schedule        []domain.ActivitySlot
	Periodicity     string
	BillingStartsOn time.Time
	FirstDueOn      time.Time
	LateFeeCents    int64
	Active          bool
	Options         []domain.PriceOption
}

// ActivityView is an activity with how many pupils are enrolled today.
type ActivityView struct {
	Activity      domain.Activity
	EnrolledCount int
}

// EnrollItem is one pupil to enrol, with the price option chosen for them.
type EnrollItem struct {
	StudentID string
	OptionID  string
}

// EnrollResult is what an enrolment did: the enrolments written (a pupil
// already enrolled is not among them) and the charges issued at once.
type EnrollResult struct {
	Enrollments    []domain.ActivityEnrollment
	ChargesCreated int
}

// today is the school's day in Morelia (ADR 0035).
func (s *ActivityService) today() time.Time {
	return s.clock.Today()
}

// CreateActivity adds an activity. A name already used — ignoring case AND
// accents, as the run recognises lines (T1) — is KindConflict.
func (s *ActivityService) CreateActivity(ctx context.Context, in ActivityInput) (domain.Activity, error) {
	options := make([]domain.PriceOption, len(in.Options))
	for i, o := range in.Options {
		o.ID = s.newID()
		options[i] = o
	}
	a, err := domain.NewActivity(in.spec(s.newID(), options))
	if err != nil {
		return domain.Activity{}, err
	}
	if err := s.refuseTakenName(ctx, a); err != nil {
		return domain.Activity{}, err
	}
	return s.repo.CreateActivity(ctx, a)
}

// UpdateActivity replaces an activity's fields and options. An option sent
// with an id must be one of this activity's; one left out is retired, which
// the repository refuses while an enrolled pupil uses it.
func (s *ActivityService) UpdateActivity(ctx context.Context, id string, in ActivityInput) (domain.Activity, error) {
	current, err := s.repo.GetActivity(ctx, id)
	if err != nil {
		return domain.Activity{}, err
	}
	options := make([]domain.PriceOption, len(in.Options))
	for i, o := range in.Options {
		switch {
		case strings.TrimSpace(o.ID) == "":
			o.ID = s.newID()
		default:
			if _, ok := current.Option(o.ID); !ok {
				return domain.Activity{}, glerrors.New(glerrors.KindInvalid, "un esquema de cobro no pertenece a esta actividad")
			}
		}
		options[i] = o
	}
	spec := in.spec(id, options)
	spec.ResumedOn = s.resumedOn(current, in.Active)
	a, err := domain.NewActivity(spec)
	if err != nil {
		return domain.Activity{}, err
	}
	if err := s.refuseTakenName(ctx, a); err != nil {
		return domain.Activity{}, err
	}
	if err := s.refuseNewCalendar(ctx, current, a); err != nil {
		return domain.Activity{}, err
	}
	return s.repo.UpdateActivity(ctx, a, s.clock.Today())
}

// resumedOn is today when this edit switches the activity back on, so the
// months it was off are never billed; otherwise it keeps what it had.
func (s *ActivityService) resumedOn(current domain.Activity, active bool) time.Time {
	if !current.Active() && active {
		return s.today()
	}
	return current.ResumedOn()
}

// refuseNewCalendar keeps period refs stable: once anyone has been enrolled,
// the periodicity and the period of the first billing date are fixed —
// changing them would re-issue every period already billed under a new ref.
func (s *ActivityService) refuseNewCalendar(ctx context.Context, current, next domain.Activity) error {
	if current.SameBillingCalendar(next) {
		return nil
	}
	enrolled, err := s.repo.ListActivityEnrollments(ctx, current.ID())
	if err != nil {
		return err
	}
	if len(enrolled) > 0 {
		return glerrors.New(glerrors.KindConflict,
			"ya hay alumnos inscritos: la periodicidad y el periodo del primer cobro no se pueden cambiar (crea otra actividad)")
	}
	return nil
}

func (in ActivityInput) spec(id string, options []domain.PriceOption) domain.ActivitySpec {
	return domain.ActivitySpec{
		ID: id, Name: in.Name, Schedule: in.Schedule, Periodicity: in.Periodicity,
		BillingStartsOn: in.BillingStartsOn, FirstDueOn: in.FirstDueOn,
		LateFeeCents: in.LateFeeCents, Active: in.Active, Options: options,
	}
}

// refuseTakenName compares names by domain.LineKey: "Natación" and "NATACION"
// are one activity to the office, and two of them would be two lines a parent
// cannot tell apart.
func (s *ActivityService) refuseTakenName(ctx context.Context, a domain.Activity) error {
	all, err := s.repo.ListActivities(ctx, true)
	if err != nil {
		return err
	}
	key := domain.LineKey(a.Name())
	for _, other := range all {
		if other.ID() != a.ID() && domain.LineKey(other.Name()) == key {
			return glerrors.New(glerrors.KindConflict, "ya existe una actividad llamada "+other.Name())
		}
	}
	return nil
}

// GetActivity reads one activity with its enrolled count.
func (s *ActivityService) GetActivity(ctx context.Context, id string) (ActivityView, error) {
	a, err := s.repo.GetActivity(ctx, id)
	if err != nil {
		return ActivityView{}, err
	}
	counts, err := s.repo.CountActiveEnrollments(ctx, s.today())
	if err != nil {
		return ActivityView{}, err
	}
	return ActivityView{Activity: a, EnrolledCount: counts[a.ID()]}, nil
}

// ListActivities lists the activities with their enrolled counts.
func (s *ActivityService) ListActivities(ctx context.Context, includeInactive bool) ([]ActivityView, error) {
	all, err := s.repo.ListActivities(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountActiveEnrollments(ctx, s.today())
	if err != nil {
		return nil, err
	}
	out := make([]ActivityView, len(all))
	for i, a := range all {
		out[i] = ActivityView{Activity: a, EnrolledCount: counts[a.ID()]}
	}
	return out, nil
}

// ListEnrollments lists the pupils enrolled in an activity today.
func (s *ActivityService) ListEnrollments(ctx context.Context, activityID string) ([]domain.ActivityEnrollment, error) {
	if _, err := s.repo.GetActivity(ctx, activityID); err != nil {
		return nil, err
	}
	all, err := s.repo.ListActivityEnrollments(ctx, activityID)
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]domain.ActivityEnrollment, 0, len(all))
	for _, e := range all {
		if e.ActiveOn(today) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Enroll enrols pupils in an active activity, each with a live option of it,
// from startsOn (zero: today), and charges what is already billable. A pupil
// already enrolled is left as is.
func (s *ActivityService) Enroll(ctx context.Context, activityID string, startsOn time.Time, items []EnrollItem) (EnrollResult, error) {
	if len(items) == 0 || len(items) > maxEnrollBatch {
		return EnrollResult{}, glerrors.New(glerrors.KindInvalid, "inscribe de 1 a 200 alumnos a la vez")
	}
	a, err := s.repo.GetActivity(ctx, activityID)
	if err != nil {
		return EnrollResult{}, err
	}
	if !a.Active() {
		return EnrollResult{}, glerrors.New(glerrors.KindConflict, "la actividad está inactiva")
	}
	if startsOn.IsZero() {
		startsOn = s.today()
	}
	wanted, err := s.newEnrollments(ctx, a, startsOn, items)
	if err != nil {
		return EnrollResult{}, err
	}
	created, err := s.repo.CreateEnrollments(ctx, wanted)
	if err != nil {
		return EnrollResult{}, err
	}
	// Bill every requested pupil who is enrolled now, not only the new ones:
	// enrolling and billing are two writes, and when billing failed the first
	// time, enrolling the same pupils again is how the office gets the charges
	// (idempotent by ref) instead of nothing.
	open, err := s.openEnrollmentsOf(ctx, a.ID(), wanted)
	if err != nil {
		return EnrollResult{}, err
	}
	charged, err := s.biller.BillActivityEnrollments(ctx, a, open)
	if err != nil {
		return EnrollResult{}, err
	}
	return EnrollResult{Enrollments: created, ChargesCreated: charged}, nil
}

// openEnrollmentsOf is the running enrolment of each requested pupil.
func (s *ActivityService) openEnrollmentsOf(ctx context.Context, activityID string, wanted []domain.ActivityEnrollment) ([]domain.ActivityEnrollment, error) {
	requested := make(map[string]bool, len(wanted))
	for _, e := range wanted {
		requested[e.StudentID()] = true
	}
	all, err := s.repo.ListActivityEnrollments(ctx, activityID)
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]domain.ActivityEnrollment, 0, len(wanted))
	for _, e := range all {
		if requested[e.StudentID()] && e.ActiveOn(today) {
			out = append(out, e)
		}
	}
	return out, nil
}

// newEnrollments validates the batch — each option is the activity's, each
// pupil exists — and builds one enrolment per distinct pupil.
func (s *ActivityService) newEnrollments(ctx context.Context, a domain.Activity, startsOn time.Time, items []EnrollItem) ([]domain.ActivityEnrollment, error) {
	seen := make(map[string]bool, len(items))
	out := make([]domain.ActivityEnrollment, 0, len(items))
	for _, it := range items {
		if seen[it.StudentID] {
			continue
		}
		seen[it.StudentID] = true
		if _, ok := a.Option(it.OptionID); !ok {
			return nil, glerrors.New(glerrors.KindInvalid, "elige un esquema de cobro de esta actividad")
		}
		if err := s.requireStudent(ctx, it.StudentID); err != nil {
			return nil, err
		}
		e, err := domain.NewActivityEnrollment(s.newID(), a.ID(), it.StudentID, it.OptionID, startsOn)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// requireStudent reports an unknown pupil as bad input, never as a NotFound
// leaked from another service.
func (s *ActivityService) requireStudent(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if _, err := s.students.ByID(ctx, id); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "student not found")
		}
		return err
	}
	return nil
}

// Unenroll ends the pupils' enrolments on endsOn (zero: today). No period
// billed from that day on is charged; issued charges are never touched.
func (s *ActivityService) Unenroll(ctx context.Context, activityID string, studentIDs []string, endsOn time.Time) (int, error) {
	if len(studentIDs) == 0 || len(studentIDs) > maxEnrollBatch {
		return 0, glerrors.New(glerrors.KindInvalid, "da de baja de 1 a 200 alumnos a la vez")
	}
	if _, err := s.repo.GetActivity(ctx, activityID); err != nil {
		return 0, err
	}
	if endsOn.IsZero() {
		endsOn = s.today()
	}
	return s.repo.EndEnrollments(ctx, activityID, studentIDs, endsOn)
}

// ChangeOption moves an enrolled pupil to another live option of the activity,
// from the next period billed.
func (s *ActivityService) ChangeOption(ctx context.Context, activityID, studentID, optionID string) (domain.ActivityEnrollment, error) {
	a, err := s.repo.GetActivity(ctx, activityID)
	if err != nil {
		return domain.ActivityEnrollment{}, err
	}
	if _, ok := a.Option(optionID); !ok {
		return domain.ActivityEnrollment{}, glerrors.New(glerrors.KindInvalid, "elige un esquema de cobro de esta actividad")
	}
	return s.repo.ChangeEnrollmentOption(ctx, activityID, studentID, optionID, s.today())
}
