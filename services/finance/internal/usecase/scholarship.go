package usecase

import (
	"context"
	"sort"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// maxScholarshipBatch bounds one assignment request, like an enrolment batch.
const maxScholarshipBatch = 200

// ScholarshipLedger is what the scholarships need from the ledger: to write
// down what a scholarship has already settled before it is taken away or
// changed. FinanceService is it.
type ScholarshipLedger interface {
	FreezeSettledScholarships(ctx context.Context, studentIDs []string) error
}

// ConceptLister reads the charge catalog (FinanceRepository is one).
type ConceptLister interface {
	ListConcepts(ctx context.Context, includeInactive bool) ([]domain.ChargeConcept, error)
}

// ActivityLister reads the activities (ActivityRepository is one).
type ActivityLister interface {
	ListActivities(ctx context.Context, includeInactive bool) ([]domain.Activity, error)
}

// ScholarshipService manages the scholarships of T18 (ADR 0022) and who holds
// them. It never prices a charge: FinanceService does, through
// domain.ApplyScholarship, on every read and payment.
type ScholarshipService struct {
	repo       ports.ScholarshipRepository
	ledger     ScholarshipLedger
	concepts   ConceptLister
	activities ActivityLister
	students   ports.StudentReader
	tenantID   string
	newID      func() string
	clock      SchoolClock
}

// NewScholarshipService wires the use case. concepts and activities are read
// to offer, and check, what a scholarship can apply to.
func NewScholarshipService(repo ports.ScholarshipRepository, ledger ScholarshipLedger, concepts ConceptLister,
	activities ActivityLister, students ports.StudentReader, tenantID string, newID func() string, clock SchoolClock,
) *ScholarshipService {
	return &ScholarshipService{
		repo: repo, ledger: ledger, concepts: concepts, activities: activities, students: students,
		tenantID: tenantID, newID: newID, clock: clock,
	}
}

// ScholarshipInput is a scholarship as the director types it.
type ScholarshipInput struct {
	Name    string
	Kind    string
	Value   int64
	Targets []string
	Active  bool
}

// ScholarshipView is a scholarship with how many pupils hold it now.
type ScholarshipView struct {
	Scholarship   domain.Scholarship
	AssignedCount int
}

// ScholarshipTarget is one thing a scholarship can apply to, in words.
type ScholarshipTarget struct {
	Key   string
	Label string
	// Group is enrollment, tuition, concept or activity.
	Group string
}

// The groups the targets are offered in.
const (
	targetGroupEnrollment = "enrollment"
	targetGroupTuition    = "tuition"
	targetGroupConcept    = "concept"
	targetGroupActivity   = "activity"
)

// ScholarshipRef names a scholarship.
type ScholarshipRef struct {
	ID   string
	Name string
}

// ScholarshipHolder is a pupil holding a scholarship, with the other
// scholarships they hold that share a target with it: those do not stack, the
// most favourable applies, and the screen says so.
type ScholarshipHolder struct {
	StudentID string
	StartsOn  time.Time
	Overlaps  []ScholarshipRef
}

func (in ScholarshipInput) spec(id, tenantID string) domain.ScholarshipSpec {
	return domain.ScholarshipSpec{
		ID: id, TenantID: tenantID, Name: in.Name, Kind: in.Kind, Value: in.Value, Targets: in.Targets, Active: in.Active,
	}
}

// CreateScholarship adds a scholarship. A name already used — ignoring case
// and accents — is KindConflict; a target naming no concept or activity of
// the school is KindInvalid.
func (s *ScholarshipService) CreateScholarship(ctx context.Context, in ScholarshipInput) (domain.Scholarship, error) {
	sch, err := domain.NewScholarship(in.spec(s.newID(), s.tenantID))
	if err != nil {
		return domain.Scholarship{}, err
	}
	if err := s.refuse(ctx, sch); err != nil {
		return domain.Scholarship{}, err
	}
	return s.repo.CreateScholarship(ctx, sch)
}

// UpdateScholarship replaces a scholarship's terms. What it already settled
// is written down first, so the change reaches only what is still pending.
func (s *ScholarshipService) UpdateScholarship(ctx context.Context, id string, in ScholarshipInput) (domain.Scholarship, error) {
	if _, err := s.repo.GetScholarship(ctx, id); err != nil {
		return domain.Scholarship{}, err
	}
	sch, err := domain.NewScholarship(in.spec(id, s.tenantID))
	if err != nil {
		return domain.Scholarship{}, err
	}
	if err := s.refuse(ctx, sch); err != nil {
		return domain.Scholarship{}, err
	}
	holders, err := s.repo.ListHolders(ctx, id)
	if err != nil {
		return domain.Scholarship{}, err
	}
	if err := s.ledger.FreezeSettledScholarships(ctx, studentsHolding(holders)); err != nil {
		return domain.Scholarship{}, err
	}
	return s.repo.UpdateScholarship(ctx, sch)
}

// refuse runs the checks that need the store: the name, and that every
// concept and activity named exists.
func (s *ScholarshipService) refuse(ctx context.Context, sch domain.Scholarship) error {
	all, err := s.repo.ListScholarships(ctx, true)
	if err != nil {
		return err
	}
	key := domain.LineKey(sch.Name())
	for _, other := range all {
		if other.ID() != sch.ID() && domain.LineKey(other.Name()) == key {
			return glerrors.New(glerrors.KindConflict, "ya existe una beca llamada "+other.Name())
		}
	}
	known, err := s.knownTargets(ctx, true)
	if err != nil {
		return err
	}
	for _, t := range sch.Targets() {
		if _, ok := known[t]; !ok {
			return glerrors.New(glerrors.KindInvalid, "la beca nombra un concepto o una actividad que no existe")
		}
	}
	return nil
}

// knownTargets is every target the school has, by key: active ones only when
// offering, any when checking a scholarship that may name a retired one.
func (s *ScholarshipService) knownTargets(ctx context.Context, includeInactive bool) (map[string]ScholarshipTarget, error) {
	out := map[string]ScholarshipTarget{
		domain.TargetEnrollment: {Key: domain.TargetEnrollment, Label: "Inscripción", Group: targetGroupEnrollment},
		domain.TargetTuition:    {Key: domain.TargetTuition, Label: "Colegiatura mensual", Group: targetGroupTuition},
	}
	concepts, err := s.concepts.ListConcepts(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	for _, c := range concepts {
		if c.Kind() != domain.KindCustom && c.Kind() != domain.KindFee {
			continue
		}
		key := domain.ConceptTarget(c.ID())
		out[key] = ScholarshipTarget{Key: key, Label: c.Name(), Group: targetGroupConcept}
	}
	if s.activities == nil {
		return out, nil
	}
	activities, err := s.activities.ListActivities(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	for _, a := range activities {
		key := domain.ActivityTarget(a.ID())
		out[key] = ScholarshipTarget{Key: key, Label: a.Name(), Group: targetGroupActivity}
	}
	return out, nil
}

// ListTargets is what a scholarship can apply to today: the inscription, the
// tuition, the school's active concepts (and the plan's fees) and its active
// activities, grouped in that order and by name within each group.
func (s *ScholarshipService) ListTargets(ctx context.Context) ([]ScholarshipTarget, error) {
	known, err := s.knownTargets(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]ScholarshipTarget, 0, len(known))
	for _, t := range known {
		out = append(out, t)
	}
	order := map[string]int{targetGroupEnrollment: 0, targetGroupTuition: 1, targetGroupConcept: 2, targetGroupActivity: 3}
	sortTargets(out, order)
	return out, nil
}

// GetScholarship reads one with its holder count.
func (s *ScholarshipService) GetScholarship(ctx context.Context, id string) (ScholarshipView, error) {
	sch, err := s.repo.GetScholarship(ctx, id)
	if err != nil {
		return ScholarshipView{}, err
	}
	counts, err := s.repo.CountHolders(ctx)
	if err != nil {
		return ScholarshipView{}, err
	}
	return ScholarshipView{Scholarship: sch, AssignedCount: counts[id]}, nil
}

// ListScholarships lists them by name with their holder counts.
func (s *ScholarshipService) ListScholarships(ctx context.Context, includeInactive bool) ([]ScholarshipView, error) {
	list, err := s.repo.ListScholarships(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountHolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ScholarshipView, 0, len(list))
	for _, sch := range list {
		out = append(out, ScholarshipView{Scholarship: sch, AssignedCount: counts[sch.ID()]})
	}
	return out, nil
}

// ListHolders lists who holds a scholarship now, with their overlaps.
func (s *ScholarshipService) ListHolders(ctx context.Context, id string) ([]ScholarshipHolder, error) {
	sch, err := s.repo.GetScholarship(ctx, id)
	if err != nil {
		return nil, err
	}
	holdings, err := s.repo.ListHolders(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.withOverlaps(ctx, sch, holdings)
}

// Assign gives a scholarship to up to 200 pupils; one who already holds it is
// left as is. It reaches their pending charges at once, because a charge is
// priced when it is read. An inactive scholarship is KindConflict; a pupil
// the school does not have is KindInvalid.
func (s *ScholarshipService) Assign(ctx context.Context, id string, studentIDs []string) ([]ScholarshipHolder, error) {
	if err := validBatch(studentIDs); err != nil {
		return nil, err
	}
	sch, err := s.repo.GetScholarship(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sch.Active() {
		return nil, glerrors.New(glerrors.KindConflict, "la beca está inactiva")
	}
	today := s.clock.Today()
	wanted := make([]domain.ScholarshipHolding, 0, len(studentIDs))
	for _, student := range studentIDs {
		if _, err := s.students.ByID(ctx, student); err != nil {
			if glerrors.KindOf(err) == glerrors.KindNotFound {
				return nil, glerrors.New(glerrors.KindInvalid, "alumno no encontrado")
			}
			return nil, err
		}
		h, err := domain.NewScholarshipHolding(s.newID(), s.tenantID, id, student, today)
		if err != nil {
			return nil, err
		}
		wanted = append(wanted, h)
	}
	written, err := s.repo.Assign(ctx, wanted)
	if err != nil {
		return nil, err
	}
	return s.withOverlaps(ctx, sch, written)
}

// Unassign takes a scholarship away from up to 200 pupils. What it already
// settled for them is written down first; their pending charges go back to
// their price. A pupil who does not hold it is ignored.
func (s *ScholarshipService) Unassign(ctx context.Context, id string, studentIDs []string) (int, error) {
	if err := validBatch(studentIDs); err != nil {
		return 0, err
	}
	if _, err := s.repo.GetScholarship(ctx, id); err != nil {
		return 0, err
	}
	if err := s.ledger.FreezeSettledScholarships(ctx, studentIDs); err != nil {
		return 0, err
	}
	return s.repo.Unassign(ctx, id, studentIDs, s.clock.Today())
}

// withOverlaps pairs each holding with the pupil's other scholarships that
// share a target with sch.
func (s *ScholarshipService) withOverlaps(ctx context.Context, sch domain.Scholarship, holdings []domain.ScholarshipHolding) ([]ScholarshipHolder, error) {
	out := make([]ScholarshipHolder, 0, len(holdings))
	if len(holdings) == 0 {
		return out, nil
	}
	held, err := s.repo.HeldBy(ctx, studentsHolding(holdings))
	if err != nil {
		return nil, err
	}
	for _, h := range holdings {
		holder := ScholarshipHolder{StudentID: h.StudentID, StartsOn: h.StartsOn, Overlaps: []ScholarshipRef{}}
		for _, o := range held[h.StudentID] {
			other := o.Scholarship
			if other.ID() != sch.ID() && other.Active() && other.SharesTargetWith(sch) {
				holder.Overlaps = append(holder.Overlaps, ScholarshipRef{ID: other.ID(), Name: other.Name()})
			}
		}
		out = append(out, holder)
	}
	return out, nil
}

// validBatch checks the size of an assignment request.
func validBatch(studentIDs []string) error {
	if len(studentIDs) == 0 || len(studentIDs) > maxScholarshipBatch {
		return glerrors.New(glerrors.KindInvalid, "elige de 1 a 200 alumnos")
	}
	return nil
}

// studentsHolding is the pupils of a list of holdings.
func studentsHolding(hs []domain.ScholarshipHolding) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.StudentID)
	}
	return out
}

// sortTargets orders targets by group, then by name, then by key.
func sortTargets(ts []ScholarshipTarget, groupOrder map[string]int) {
	sort.Slice(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if groupOrder[a.Group] != groupOrder[b.Group] {
			return groupOrder[a.Group] < groupOrder[b.Group]
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.Key < b.Key
	})
}
