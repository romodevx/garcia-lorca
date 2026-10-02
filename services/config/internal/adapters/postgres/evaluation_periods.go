package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Compile-time proof the adapter satisfies the evaluation-period port.
var _ ports.EvaluationPeriodRepository = (*Repository)(nil)

// exclusionViolation is SQLSTATE 23P01: a row an EXCLUDE constraint refuses.
const exclusionViolation = "23P01"

// evaluationPeriodModel is the GORM row for `evaluation_periods` (0016).
// A rewrite deletes and re-inserts a grade's rows with their ids, carrying
// created_at over; updated_at is the rewrite's.
type evaluationPeriodModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	CycleID      string    `gorm:"column:cycle_id"`
	GradeLevelID string    `gorm:"column:grade_level_id"`
	Ordinal      int       `gorm:"column:ordinal"`
	Name         string    `gorm:"column:name"`
	StartsOn     time.Time `gorm:"column:starts_on;type:date"`
	EndsOn       time.Time `gorm:"column:ends_on;type:date"`
	TermID       *string   `gorm:"column:term_id"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (evaluationPeriodModel) TableName() string { return "evaluation_periods" }

func fromDomainEvaluationPeriod(p domain.EvaluationPeriod) evaluationPeriodModel {
	m := evaluationPeriodModel{
		ID: p.ID(), TenantID: p.TenantID(), CycleID: p.CycleID(), GradeLevelID: p.GradeLevelID(),
		Ordinal: p.Ordinal(), Name: p.Name(), StartsOn: p.StartsOn(), EndsOn: p.EndsOn(),
	}
	if termID := p.TermID(); termID != "" {
		m.TermID = &termID
	}
	return m
}

func toDomainEvaluationPeriod(m evaluationPeriodModel) (domain.EvaluationPeriod, error) {
	p, err := domain.NewEvaluationPeriod(m.ID, m.TenantID, m.CycleID, m.GradeLevelID, m.Name, m.Ordinal, m.StartsOn.UTC(), m.EndsOn.UTC())
	if err != nil {
		return domain.EvaluationPeriod{}, err
	}
	if m.TermID != nil {
		p = p.WithTermID(*m.TermID)
	}
	return p, nil
}

// ListEvaluationPeriods returns the periods of cycleID, narrowed to a level's
// grades and/or one grade when given, ordered by grade ordinal then period
// ordinal. Malformed ids match nothing (an empty list, never a uuid-cast
// error). Bounded like every list read.
func (r *Repository) ListEvaluationPeriods(ctx context.Context, cycleID, levelID, gradeLevelID string) ([]domain.EvaluationPeriod, error) {
	for _, id := range []string{levelID, gradeLevelID} {
		if id != "" && !isUUID(id) {
			return []domain.EvaluationPeriod{}, nil
		}
	}
	if !isUUID(cycleID) {
		return []domain.EvaluationPeriod{}, nil
	}
	var out []domain.EvaluationPeriod
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Table("evaluation_periods p").Select("p.*").
			Joins("JOIN grade_levels g ON g.id = p.grade_level_id").
			Where("p.cycle_id = ?", cycleID)
		if levelID != "" {
			q = q.Where("g.level_id = ?", levelID)
		}
		if gradeLevelID != "" {
			q = q.Where("p.grade_level_id = ?", gradeLevelID)
		}
		var rows []evaluationPeriodModel
		if err := q.Order("g.ordinal, p.grade_level_id, p.ordinal").Limit(maxListRows).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing evaluation periods", err)
		}
		out = make([]domain.EvaluationPeriod, 0, len(rows))
		for _, m := range rows {
			p, derr := toDomainEvaluationPeriod(m)
			if derr != nil {
				return derr
			}
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceEvaluationPeriods makes each grade's set in cycleID exactly the given
// one, in ONE transaction (see the port). Each grade's rows are deleted and
// its set inserted with the same ids: an in-place UPDATE would trip the
// EXCLUDE constraint mid-rewrite whenever neighbouring periods move together
// (and 0016 cannot defer it, see there). An overlap — also one a concurrent
// writer committed meanwhile — is domain.PeriodOverlap and nothing is written.
func (r *Repository) ReplaceEvaluationPeriods(ctx context.Context, cycleID string, sets []ports.GradePeriodSet) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, set := range sets {
			if err := writePeriodSet(tx, cycleID, set); err != nil {
				return err
			}
		}
		return nil
	})
}

// writePeriodSet replaces one grade's rows, keeping created_at of the periods
// that survive.
func writePeriodSet(tx *gorm.DB, cycleID string, set ports.GradePeriodSet) error {
	var removed []evaluationPeriodModel
	if err := tx.Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "created_at"}}}).
		Where("cycle_id = ? AND grade_level_id = ?", cycleID, set.GradeLevelID).
		Delete(&removed).Error; err != nil {
		return translatePeriodErr(err, "removing evaluation periods")
	}
	if len(set.Periods) == 0 {
		return nil
	}
	createdAt := make(map[string]time.Time, len(removed))
	for _, m := range removed {
		createdAt[m.ID] = m.CreatedAt
	}
	now := time.Now().UTC()
	rows := make([]evaluationPeriodModel, 0, len(set.Periods))
	for _, p := range set.Periods {
		m := fromDomainEvaluationPeriod(p)
		m.CreatedAt, m.UpdatedAt = now, now
		if at, kept := createdAt[m.ID]; kept {
			m.CreatedAt = at
		}
		rows = append(rows, m)
	}
	return translatePeriodErr(tx.Create(&rows).Error, "writing evaluation periods")
}

// translatePeriodErr maps the EXCLUDE refusal to the coded overlap; the rest
// goes through translateCreateErr (FK → KindInvalid, unique → KindConflict).
func translatePeriodErr(err error, internalMsg string) error {
	if err == nil {
		return nil
	}
	if pgCode(err) == exclusionViolation {
		return domain.PeriodOverlap(domain.EvaluationPeriod{}, domain.EvaluationPeriod{})
	}
	return translateCreateErr(err, internalMsg, "Ese número de periodo ya existe en el grado")
}
