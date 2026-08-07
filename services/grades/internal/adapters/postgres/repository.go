package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.GradeEntryRepository = (*Repository)(nil)

// Repository is the GORM-backed grade-entries repository. Every operation runs
// in a transaction bound to a tenant so Row Level Security applies (mirrors
// iam/students, ADR 0001). In this phase the tenant is fixed at construction
// (single-tenant).
type Repository struct {
	db       *gorm.DB
	tenantID string
}

// NewRepository builds the repository scoped to tenantID.
func NewRepository(db *gorm.DB, tenantID string) *Repository {
	return &Repository{db: db, tenantID: tenantID}
}

// withTenant runs fn inside a transaction that has bound app.current_tenant,
// delegating to the shared pkg/authctx RLS primitive so tenant binding lives in
// exactly one place across every service.
func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	})
}

// UpsertGradeEntry inserts a grade entry, or updates the value of the existing
// row on the (tenant, enrollment, subject, term) key — one grade per
// student·subject·term ("capturar/editar"). It bumps updated_at on the update
// path and leaves the reserved published_at column untouched (NULL). The stored
// row is re-read inside the same tenant-bound transaction and returned, so the
// caller sees the canonical id/value even when the upsert updated a pre-existing
// row.
func (r *Repository) UpsertGradeEntry(ctx context.Context, e domain.GradeEntry) (domain.GradeEntry, error) {
	model := fromDomain(e)
	var out domain.GradeEntry
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "enrollment_id"}, {Name: "subject_id"}, {Name: "term_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"value":      model.Value,
				"updated_at": gorm.Expr("now()"),
			}),
		}).Create(&model)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "upserting grade entry", res.Error)
		}

		var stored gradeEntryModel
		if err := tx.Where(
			"enrollment_id = ? AND subject_id = ? AND term_id = ?",
			model.EnrollmentID, model.SubjectID, model.TermID,
		).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		d, err := toDomain(stored)
		if err != nil {
			return err
		}
		out = d
		return nil
	})
	if err != nil {
		return domain.GradeEntry{}, err
	}
	return out, nil
}

// GetForKeys batch-loads the grade entries for many enrollments sharing the
// same (subject, term) in ONE tenant-bound transaction (a single SELECT with an
// IN filter), collapsing what would otherwise be one query per roster student
// (N+1). The returned map is keyed by enrollment_id and contains an entry only
// for enrollments that actually have a grade; enrollments with no grade — and
// any hidden by RLS — are simply absent (no rows is not an error). An empty
// enrollmentIDs slice returns an empty map without querying.
func (r *Repository) GetForKeys(ctx context.Context, enrollmentIDs []string, subjectID, termID string) (map[string]domain.GradeEntry, error) {
	out := make(map[string]domain.GradeEntry, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []gradeEntryModel
		if err := tx.Where(
			"subject_id = ? AND term_id = ? AND enrollment_id IN ?",
			subjectID, termID, enrollmentIDs,
		).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "batch loading grade entries", err)
		}
		for _, m := range rows {
			d, err := toDomain(m)
			if err != nil {
				return err
			}
			out[m.EnrollmentID] = d
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListGradeEntries returns the entries for an enrollment in a term; an empty
// string skips that filter. Results are ordered by id for a stable read.
func (r *Repository) ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]domain.GradeEntry, error) {
	var out []domain.GradeEntry
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&gradeEntryModel{}).Order("id ASC")
		if enrollmentID != "" {
			q = q.Where("enrollment_id = ?", enrollmentID)
		}
		if termID != "" {
			q = q.Where("term_id = ?", termID)
		}
		var rows []gradeEntryModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing grade entries", err)
		}
		for _, m := range rows {
			d, err := toDomain(m)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// translateFindErr maps a GORM not-found to the shared sentinel; anything else
// is an internal error.
func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}
