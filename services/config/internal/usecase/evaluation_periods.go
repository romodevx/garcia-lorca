package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// EvaluationPeriodService configures the evaluation periods of each grade in a
// cycle (T14, ADR 0029): list them, replace one grade's set, and copy one
// grade's set onto others. A grade's periods never overlap and lie inside the
// cycle (domain.CheckPeriodSet); the database refuses overlaps as well, so two
// concurrent editors cannot get past it either.
type EvaluationPeriodService struct {
	repo   ports.EvaluationPeriodRepository
	tenant string
	newID  func() string
}

// NewEvaluationPeriodService wires the use case (tenant and id mint as in
// NewCatalogService).
func NewEvaluationPeriodService(repo ports.EvaluationPeriodRepository, tenant string, newID func() string) *EvaluationPeriodService {
	return &EvaluationPeriodService{repo: repo, tenant: tenant, newID: newID}
}

// PeriodInput is one period as the school types it. ID names the grade's
// existing period the row edits ("" for a new one).
type PeriodInput struct {
	ID       string
	Ordinal  int
	Name     string
	StartsOn time.Time
	EndsOn   time.Time
}

// ListEvaluationPeriods returns the periods of cycleID, narrowed to a level's
// grades and/or one grade when given, by grade then ordinal.
func (s *EvaluationPeriodService) ListEvaluationPeriods(ctx context.Context, cycleID, levelID, gradeLevelID string) ([]domain.EvaluationPeriod, error) {
	if strings.TrimSpace(cycleID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	return s.repo.ListEvaluationPeriods(ctx, cycleID, strings.TrimSpace(levelID), strings.TrimSpace(gradeLevelID))
}

// UpsertEvaluationPeriods makes inputs the complete period set of one grade in
// a cycle. A row carrying the id of one of the grade's periods keeps that id
// and its term link — the id follows the row, not its position, so removing
// the 1st period removes it even though the rest are renumbered. Any other id
// (blank, stale, another grade's) is a new period. A period no row names is
// removed; an empty set clears the grade. Returns the set by ordinal.
func (s *EvaluationPeriodService) UpsertEvaluationPeriods(ctx context.Context, cycleID, gradeLevelID string, inputs []PeriodInput) ([]domain.EvaluationPeriod, error) {
	cycle, err := s.repo.GetCycle(ctx, cycleID)
	if err != nil {
		return nil, requireParent(err, "cycle not found")
	}
	if _, err := s.repo.GetGradeLevel(ctx, gradeLevelID); err != nil {
		return nil, requireParent(err, "grade level not found")
	}
	existing, err := s.repo.ListEvaluationPeriods(ctx, cycleID, "", gradeLevelID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.EvaluationPeriod, len(existing))
	for _, p := range existing {
		byID[p.ID()] = p
	}
	set, err := s.buildSet(cycle, gradeLevelID, inputs, func(in PeriodInput) (domain.EvaluationPeriod, bool) {
		p, ok := byID[in.ID]
		return p, ok
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceEvaluationPeriods(ctx, cycleID, []ports.GradePeriodSet{{GradeLevelID: gradeLevelID, Periods: set}}); err != nil {
		return nil, err
	}
	return set, nil
}

// CopyEvaluationPeriods gives every target grade the source grade's periods
// (names and dates) in ONE transaction, matched by ordinal like an upsert: a
// target's existing ordinal keeps its id, a missing one is created, an extra
// one is removed. "Apply to the whole level" is a copy onto the level's other
// grades. Returns the targets' periods, grade by grade in request order.
func (s *EvaluationPeriodService) CopyEvaluationPeriods(ctx context.Context, cycleID, fromGradeLevelID string, toGradeLevelIDs []string) ([]domain.EvaluationPeriod, error) {
	cycle, err := s.repo.GetCycle(ctx, cycleID)
	if err != nil {
		return nil, requireParent(err, "cycle not found")
	}
	targets, err := s.copyTargets(ctx, fromGradeLevelID, toGradeLevelIDs)
	if err != nil {
		return nil, err
	}
	// One read of the cycle's periods serves the source and every target.
	all, err := s.repo.ListEvaluationPeriods(ctx, cycleID, "", "")
	if err != nil {
		return nil, err
	}
	byGrade := make(map[string][]domain.EvaluationPeriod)
	for _, p := range all {
		byGrade[p.GradeLevelID()] = append(byGrade[p.GradeLevelID()], p)
	}
	source := byGrade[fromGradeLevelID]
	if len(source) == 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "El grado de origen no tiene periodos en el ciclo")
	}
	inputs := make([]PeriodInput, 0, len(source))
	for _, p := range source {
		inputs = append(inputs, PeriodInput{Ordinal: p.Ordinal(), Name: p.Name(), StartsOn: p.StartsOn(), EndsOn: p.EndsOn()})
	}
	sets := make([]ports.GradePeriodSet, 0, len(targets))
	var out []domain.EvaluationPeriod
	for _, gradeID := range targets {
		// A copy has no rows of its own: a target's period with the same
		// ordinal is the one that takes the source's dates.
		target := byOrdinal(byGrade[gradeID])
		set, err := s.buildSet(cycle, gradeID, inputs, func(in PeriodInput) (domain.EvaluationPeriod, bool) {
			p, ok := target[in.Ordinal]
			return p, ok
		})
		if err != nil {
			return nil, err
		}
		sets = append(sets, ports.GradePeriodSet{GradeLevelID: gradeID, Periods: set})
		out = append(out, set...)
	}
	if err := s.repo.ReplaceEvaluationPeriods(ctx, cycleID, sets); err != nil {
		return nil, err
	}
	return out, nil
}

// EvaluationPeriodOn is the period of a grade that contains the calendar date
// of on (pass the school's local date), or false when the date falls in no
// period. T15 files a grade captured on a date under this period.
func (s *EvaluationPeriodService) EvaluationPeriodOn(ctx context.Context, cycleID, gradeLevelID string, on time.Time) (domain.EvaluationPeriod, bool, error) {
	periods, err := s.ListEvaluationPeriods(ctx, cycleID, "", gradeLevelID)
	if err != nil {
		return domain.EvaluationPeriod{}, false, err
	}
	p, ok := domain.PeriodOn(periods, gradeLevelID, on)
	return p, ok, nil
}

// buildSet turns inputs into one grade's validated period set; matchOf names
// the existing period a row keeps (its id and term link). An existing period
// is kept by one row at most. Sorted by ordinal.
func (s *EvaluationPeriodService) buildSet(cycle domain.Cycle, gradeLevelID string, inputs []PeriodInput, matchOf func(PeriodInput) (domain.EvaluationPeriod, bool)) ([]domain.EvaluationPeriod, error) {
	set := make([]domain.EvaluationPeriod, 0, len(inputs))
	claimed := make(map[string]struct{}, len(inputs))
	for _, in := range inputs {
		id, termID := s.newID(), ""
		if prev, ok := matchOf(in); ok {
			if _, taken := claimed[prev.ID()]; !taken {
				id, termID = prev.ID(), prev.TermID()
				claimed[id] = struct{}{}
			}
		}
		p, err := domain.NewEvaluationPeriod(id, s.tenant, cycle.ID(), gradeLevelID, in.Name, in.Ordinal, in.StartsOn, in.EndsOn)
		if err != nil {
			return nil, err
		}
		set = append(set, p.WithTermID(termID))
	}
	if err := domain.CheckPeriodSet(cycle, set); err != nil {
		return nil, err
	}
	sort.Slice(set, func(i, j int) bool { return set[i].Ordinal() < set[j].Ordinal() })
	return set, nil
}

// copyTargets validates a copy's targets: at least one, none the source, each
// an existing grade. De-duplicated, request order kept.
func (s *EvaluationPeriodService) copyTargets(ctx context.Context, fromGradeLevelID string, toGradeLevelIDs []string) ([]string, error) {
	if _, err := s.repo.GetGradeLevel(ctx, fromGradeLevelID); err != nil {
		return nil, requireParent(err, "source grade level not found")
	}
	seen := make(map[string]struct{}, len(toGradeLevelIDs))
	targets := make([]string, 0, len(toGradeLevelIDs))
	for _, id := range toGradeLevelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, glerrors.New(glerrors.KindInvalid, "target grade level id is required")
		}
		if id == fromGradeLevelID {
			return nil, glerrors.New(glerrors.KindInvalid, "a grade cannot be copied onto itself")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		if _, err := s.repo.GetGradeLevel(ctx, id); err != nil {
			return nil, requireParent(err, "target grade level not found")
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "at least one target grade level is required")
	}
	return targets, nil
}

func byOrdinal(periods []domain.EvaluationPeriod) map[int]domain.EvaluationPeriod {
	out := make(map[int]domain.EvaluationPeriod, len(periods))
	for _, p := range periods {
		out[p.Ordinal()] = p
	}
	return out
}
