package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// SetTermMonths replaces the calendar months a term covers ("YYYY-MM" each).
// A trimester is a set of months configured per cycle and level, not a date
// rule. Every month must touch the term's cycle (MonthOutsideCycle) and none
// may belong to a sibling term of the same cycle and level (MonthInOtherTerm,
// naming the month). An empty set clears the term.
func (s *CatalogService) SetTermMonths(ctx context.Context, termID string, rawMonths []string) (domain.Term, error) {
	ms, err := parseMonths(rawMonths)
	if err != nil {
		return domain.Term{}, err
	}
	term, err := s.repo.GetTerm(ctx, termID)
	if err != nil {
		return domain.Term{}, err
	}
	cycle, err := s.repo.GetCycle(ctx, term.CycleID())
	if err != nil {
		return domain.Term{}, err
	}
	for _, m := range ms {
		if !cycle.ContainsMonth(m) {
			return domain.Term{}, domain.MonthOutsideCycle(m)
		}
	}
	updated, err := term.WithMonths(ms)
	if err != nil {
		return domain.Term{}, err
	}
	siblings, err := s.repo.ListTerms(ctx, term.CycleID(), term.LevelID())
	if err != nil {
		return domain.Term{}, err
	}
	if m, clash := domain.FirstMonthInOtherTerm(updated, siblings); clash {
		return domain.Term{}, domain.MonthInOtherTerm(m)
	}
	if err := s.repo.ApplyTermMonths(ctx, nil, []domain.Term{updated}); err != nil {
		return domain.Term{}, err
	}
	return updated, nil
}

// CopyTermMonths makes each target level's trimesters mirror the source
// level's in one cycle, in ONE transaction. Terms are matched by ordinal: a
// matched target term keeps its own name and dates and takes the source's
// months; a missing one is created from the source (name, ordinal, dates,
// months); a target term the source has no ordinal for is kept (it may carry
// grades) but emptied, so the copy never leaves a month in two terms. Returns
// the target levels' terms, level by level in request order, then by ordinal.
func (s *CatalogService) CopyTermMonths(ctx context.Context, cycleID, fromLevelID string, toLevelIDs []string) ([]domain.Term, error) {
	targets, err := s.copyTargets(ctx, cycleID, fromLevelID, toLevelIDs)
	if err != nil {
		return nil, err
	}
	all, err := s.repo.ListTerms(ctx, cycleID, "")
	if err != nil {
		return nil, err
	}
	byLevel := groupByLevel(all)
	source := byLevel[fromLevelID]
	if len(source) == 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "El nivel de origen no tiene periodos en el ciclo")
	}
	var creates, updates, out []domain.Term
	for _, levelID := range targets {
		created, updated, err := s.mirrorTerms(source, byLevel[levelID], levelID)
		if err != nil {
			return nil, err
		}
		creates = append(creates, created...)
		updates = append(updates, updated...)
		out = append(out, sortedByOrdinal(append(append([]domain.Term(nil), created...), updated...))...)
	}
	if err := s.repo.ApplyTermMonths(ctx, creates, updates); err != nil {
		return nil, err
	}
	return out, nil
}

// copyTargets validates a copy's scope: the cycle and both sides exist, there
// is at least one target, and no target is the source. Targets are
// de-duplicated, request order kept.
func (s *CatalogService) copyTargets(ctx context.Context, cycleID, fromLevelID string, toLevelIDs []string) ([]string, error) {
	if _, err := s.repo.GetCycle(ctx, cycleID); err != nil {
		return nil, requireParent(err, "cycle not found")
	}
	if _, err := s.repo.GetLevel(ctx, fromLevelID); err != nil {
		return nil, requireParent(err, "source level not found")
	}
	seen := make(map[string]struct{}, len(toLevelIDs))
	targets := make([]string, 0, len(toLevelIDs))
	for _, id := range toLevelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, glerrors.New(glerrors.KindInvalid, "target level id is required")
		}
		if id == fromLevelID {
			return nil, glerrors.New(glerrors.KindInvalid, "a level cannot be copied onto itself")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		if _, err := s.repo.GetLevel(ctx, id); err != nil {
			return nil, requireParent(err, "target level not found")
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return nil, glerrors.New(glerrors.KindInvalid, "at least one target level is required")
	}
	return targets, nil
}

// mirrorTerms plans one target level: the terms to create and the existing
// ones whose months change.
func (s *CatalogService) mirrorTerms(source, existing []domain.Term, levelID string) (creates, updates []domain.Term, err error) {
	byOrdinal := make(map[int]domain.Term, len(existing))
	for _, t := range existing {
		byOrdinal[t.Ordinal()] = t
	}
	for _, src := range source {
		target, found := byOrdinal[src.Ordinal()]
		if !found {
			target, err = domain.NewTerm(s.newID(), s.tenant, src.CycleID(), levelID, src.Name(), src.Ordinal(), src.StartsOn(), src.EndsOn())
			if err != nil {
				return nil, nil, err
			}
		}
		delete(byOrdinal, src.Ordinal())
		mirrored, err := target.WithMonths(src.Months())
		if err != nil {
			return nil, nil, err
		}
		if found {
			updates = append(updates, mirrored)
		} else {
			creates = append(creates, mirrored)
		}
	}
	for _, orphan := range byOrdinal {
		cleared, err := orphan.WithMonths(nil)
		if err != nil {
			return nil, nil, err
		}
		updates = append(updates, cleared)
	}
	return creates, updates, nil
}

func parseMonths(raw []string) ([]time.Time, error) {
	out := make([]time.Time, 0, len(raw))
	for _, r := range raw {
		m, err := domain.ParseMonth(r)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func groupByLevel(terms []domain.Term) map[string][]domain.Term {
	out := make(map[string][]domain.Term)
	for _, t := range terms {
		out[t.LevelID()] = append(out[t.LevelID()], t)
	}
	return out
}

func sortedByOrdinal(terms []domain.Term) []domain.Term {
	sort.Slice(terms, func(i, j int) bool { return terms[i].Ordinal() < terms[j].Ordinal() })
	return terms
}
