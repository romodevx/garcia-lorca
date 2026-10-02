package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the period port.
var _ ports.PeriodRepository = (*Repository)(nil)

// msgPeriodClosed is what a write into a closed period reads.
const msgPeriodClosed = "El periodo está cerrado"

// Period locks. A term's closer and its grade writers serialise on ONE
// transaction-scoped advisory lock keyed by tenant and term: the close takes it
// EXCLUSIVE, a write takes it SHARED and then reads the closure. So a write
// either commits before the close reads its snapshot, or waits for the close
// to commit and then sees the term closed — nothing slips in between. The lock
// is released with the transaction; no row has to exist for it to work (a term
// never closed has no period_closures row to lock).
const (
	lockExclusiveSQL = "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))"
	lockSharedSQL    = "SELECT pg_advisory_xact_lock_shared(hashtextextended(?, 0))"
)

func (r *Repository) periodLockKey(termID string) string {
	return "grades.period:" + r.tenantID + ":" + termID
}

// guardPeriod runs inside a grade write's transaction: with a term named, it
// takes the term's shared lock and refuses the write with code when the term
// is closed (PERIOD_LOCKED for a period grade, MONTH_LOCKED for the legacy
// per-term capture).
func (r *Repository) guardPeriod(tx *gorm.DB, lock ports.PeriodLock, code domain.RefusalCode) error {
	if lock.TermID == "" {
		return nil
	}
	if err := tx.Exec(lockSharedSQL, r.periodLockKey(lock.TermID)).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "locking the period", err)
	}
	var status string
	err := tx.Model(&periodClosureModel{}).Select("status").Where("term_id = ?", lock.TermID).Take(&status).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	case err != nil:
		return glerrors.Wrap(glerrors.KindInternal, "reading the period closure", err)
	case status == string(domain.PeriodClosed):
		return domain.Conflict(code, msgPeriodClosed)
	default:
		return nil
	}
}

// WithPeriodLock runs fn in ONE tenant-bound transaction holding the term's
// exclusive lock.
func (r *Repository) WithPeriodLock(ctx context.Context, termID string, fn func(ctx context.Context, tx ports.PeriodTx) error) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(lockExclusiveSQL, r.periodLockKey(termID)).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "locking the period", err)
		}
		return fn(ctx, &periodTx{tx: tx, tenantID: r.tenantID, termID: termID})
	})
}

// PeriodClosures reads the closures of termIDs in ONE query.
func (r *Repository) PeriodClosures(ctx context.Context, termIDs []string) (map[string]domain.PeriodClosure, error) {
	out := make(map[string]domain.PeriodClosure, len(termIDs))
	if len(termIDs) == 0 {
		return out, nil
	}
	var rows []periodClosureModel
	if err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("term_id IN ?", termIDs).Find(&rows).Error
	}); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading period closures", err)
	}
	for _, m := range rows {
		c, err := toDomainClosure(m)
		if err != nil {
			return nil, err
		}
		out[m.TermID] = c
	}
	return out, nil
}

// LatestReportCards lists the published report cards matching filter, without
// their lines, ordered by student name.
func (r *Repository) LatestReportCards(ctx context.Context, filter ports.ReportCardFilter) ([]domain.ReportCard, error) {
	var rows []reportCardModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Where("status = ?", string(domain.ReportCardPublished))
		if filter.TermID != "" {
			q = q.Where("term_id = ?", filter.TermID)
		}
		if filter.GroupID != "" {
			q = q.Where("group_id = ?", filter.GroupID)
		}
		if filter.StudentID != "" {
			q = q.Where("student_id = ?", filter.StudentID)
		}
		return q.Order("student_name, issued_at").Find(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing report cards", err)
	}
	out := make([]domain.ReportCard, 0, len(rows))
	for _, m := range rows {
		rc, err := toDomainReportCard(m, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

// ReportCard reads one report card and its lines (in print order).
func (r *Repository) ReportCard(ctx context.Context, id string) (domain.ReportCard, error) {
	var card reportCardModel
	var lines []reportCardLineModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&card).Error; err != nil {
			return translateFindErr(err)
		}
		if err := tx.Where("report_card_id = ?", id).Order("position").Find(&lines).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading report card lines", err)
		}
		return nil
	})
	if err != nil {
		return domain.ReportCard{}, err
	}
	return toDomainReportCard(card, lines)
}

// periodTx is the PeriodTx of one locked term.
type periodTx struct {
	tx       *gorm.DB
	tenantID string
	termID   string
}

func (p *periodTx) Closure(context.Context) (domain.PeriodClosure, bool, error) {
	var m periodClosureModel
	err := p.tx.Where("term_id = ?", p.termID).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.PeriodClosure{}, false, nil
	}
	if err != nil {
		return domain.PeriodClosure{}, false, glerrors.Wrap(glerrors.KindInternal, "reading the period closure", err)
	}
	c, err := toDomainClosure(m)
	return c, err == nil, err
}

func (p *periodTx) PeriodGrades(_ context.Context, enrollmentIDs, periodIDs []string) ([]domain.PeriodGrade, error) {
	return periodGradesIn(p.tx, enrollmentIDs, periodIDs)
}

// SaveClosure inserts the closure or, on (tenant, term), updates EVERY column
// a close or a reopen changes.
func (p *periodTx) SaveClosure(_ context.Context, c domain.PeriodClosure) error {
	m := fromDomainClosure(p.tenantID, c)
	err := p.tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "term_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"cycle_id":       m.CycleID,
			"level_id":       m.LevelID,
			"status":         m.Status,
			"version":        m.Version,
			"closed_at":      m.ClosedAt,
			"closed_by":      m.ClosedBy,
			"closed_by_name": m.ClosedByName,
			"reopened_at":    m.ReopenedAt,
			"months":         m.Months,
			"updated_at":     gorm.Expr("now()"),
		}),
	}).Create(&m).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "saving the period closure", err)
	}
	return nil
}

// PublishedReportCards reads the term's published cards and, in ONE more
// query, all their lines (no query per card), each card's in print order.
func (p *periodTx) PublishedReportCards(context.Context) ([]domain.ReportCard, error) {
	var cards []reportCardModel
	if err := p.tx.Where("term_id = ? AND status = ?", p.termID, string(domain.ReportCardPublished)).
		Order("student_name, id").Find(&cards).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the published report cards", err)
	}
	if len(cards) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	var lines []reportCardLineModel
	if err := p.tx.Where("report_card_id IN ?", ids).Order("report_card_id, position").Find(&lines).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the published report card lines", err)
	}
	byCard := make(map[string][]reportCardLineModel, len(cards))
	for _, l := range lines {
		byCard[l.ReportCardID] = append(byCard[l.ReportCardID], l)
	}
	out := make([]domain.ReportCard, 0, len(cards))
	for _, c := range cards {
		rc, err := toDomainReportCard(c, byCard[c.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

// IssueReportCards supersedes EVERY published card of the term (BEFORE
// inserting: at most one published card per enrollment and term), then
// inserts the cards and their lines. A re-close replaces the term's cards as a
// whole: a pupil withdrawn since the previous close gets no new card, and
// their old one must not stay published beside the new version (ADR 0020).
func (p *periodTx) IssueReportCards(_ context.Context, cards []domain.ReportCard) error {
	if err := p.tx.Model(&reportCardModel{}).
		Where("term_id = ? AND status = ?", p.termID, string(domain.ReportCardPublished)).
		Update("status", string(domain.ReportCardSuperseded)).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "superseding report cards", err)
	}
	if len(cards) == 0 {
		return nil
	}
	cardRows := make([]reportCardModel, 0, len(cards))
	var lineRows []reportCardLineModel
	for _, rc := range cards {
		card, lines, err := fromDomainReportCard(rc)
		if err != nil {
			return err
		}
		cardRows = append(cardRows, card)
		lineRows = append(lineRows, lines...)
	}
	if err := p.tx.CreateInBatches(cardRows, insertBatchSize).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "inserting report cards", err)
	}
	if len(lineRows) == 0 {
		return nil
	}
	if err := p.tx.CreateInBatches(lineRows, insertBatchSize).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "inserting report card lines", err)
	}
	return nil
}

// insertBatchSize bounds one INSERT's parameters (Postgres caps them at 65535;
// a report card row carries 24).
const insertBatchSize = 500

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
