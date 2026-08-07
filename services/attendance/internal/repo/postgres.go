package repo

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the adapter satisfies the seam the service declares.
var _ service.Repo = (*Repo)(nil)

const (
	// dateLayout renders a Go time as the DATE literal the `date` column is
	// compared against, so only the date part of a caller's time matters and no
	// time zone can shift a school day.
	dateLayout = "2006-01-02"
	// maxListLimit caps the unpaginated history read, so an open-ended date range
	// cannot pull an unbounded number of rows into memory. It is a DOCUMENTED
	// bound, not a hidden one: the capped method says so, as does the proto
	// contract. Explicit pagination is the follow-up that removes the need for it.
	maxListLimit = 500
)

// Repo is the GORM-backed attendance repository. Every operation runs in a
// transaction bound to a tenant so Row Level Security applies (mirrors
// iam/students/grades, ADR 0001). In this phase the tenant is fixed at
// construction (single-tenant).
type Repo struct {
	db       *gorm.DB
	tenantID string
}

// New builds the repository scoped to tenantID.
func New(db *gorm.DB, tenantID string) *Repo {
	return &Repo{db: db, tenantID: tenantID}
}

// withTenant runs fn inside a transaction that has bound app.current_tenant,
// delegating to the shared pkg/authctx RLS primitive so tenant binding lives in
// exactly one place across every service.
func (r *Repo) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	})
}

// MarksForDate batch-loads the stored marks for many enrollments on one date in a
// single tenant-bound SELECT, collapsing what would otherwise be one query per
// roster student (N+1). The returned map is keyed by enrollment_id and holds a
// record only for enrollments that actually have an exception; present students —
// and any row hidden by RLS — are simply absent (no rows is not an error). An
// empty enrollmentIDs slice returns an empty map without querying.
func (r *Repo) MarksForDate(ctx context.Context, enrollmentIDs []string, date time.Time) (map[string]service.AttendanceRecord, error) {
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []attendanceModel
		if err := tx.Where(
			"enrollment_id IN ? AND date = ?::date",
			enrollmentIDs, date.Format(dateLayout),
		).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "batch loading attendance marks", err)
		}
		for _, m := range rows {
			out[m.EnrollmentID] = toRecord(m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReconcileDay makes the stored state of one group·day match records inside ONE
// tenant-bound transaction: the day's marks for every enrollment in scope are
// deleted and records written in their place. The delete is what turns a dropped
// exception back into "present", and doing both in one transaction is what keeps
// a concurrent roster read from ever seeing a half-saved day. Both statements are
// bounded by RLS, so another tenant's marks for the same enrollment·day are
// untouched.
//
// The write is an UPSERT on the (tenant, enrollment, date) key, not a plain
// insert: two teachers saving the same group·day concurrently each delete rows
// the other has not committed yet, so a plain insert would lose the race to the
// UNIQUE constraint and fail the save. On conflict the row's status, note and
// updated_at are overwritten — last write wins — while the pre-existing row keeps
// its id (the day still holds exactly one mark per enrollment either way).
func (r *Repo) ReconcileDay(ctx context.Context, enrollmentIDs []string, date time.Time, records []service.AttendanceRecord) error {
	if len(enrollmentIDs) == 0 && len(records) == 0 {
		return nil
	}
	day := date.Format(dateLayout)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if len(enrollmentIDs) > 0 {
			if err := tx.Where(
				"enrollment_id IN ? AND date = ?::date",
				enrollmentIDs, day,
			).Delete(&attendanceModel{}).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "clearing the day's attendance marks", err)
			}
		}
		if len(records) == 0 {
			return nil
		}
		models := make([]attendanceModel, 0, len(records))
		for _, rec := range records {
			models = append(models, fromRecord(rec, r.tenantID))
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "enrollment_id"}, {Name: "date"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"status":     gorm.Expr("excluded.status"),
				"note":       gorm.Expr("excluded.note"),
				"updated_at": gorm.Expr("now()"),
			}),
		}).Create(&models).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing attendance marks", err)
		}
		return nil
	})
}

// ListByEnrollment returns one enrollment's stored marks within an inclusive date
// range; a zero from or to skips that bound. Only exceptions are stored, so
// present days never appear.
//
// The read returns AT MOST maxListLimit rows, the MOST RECENT first, ordered by
// (date, id) so the cut is deterministic rather than whatever the planner
// happened to emit. Newest-first is what makes the cap safe: an open-ended range
// over years of history drops the oldest days, never the ones being looked at.
// Explicit pagination is a follow-up.
func (r *Repo) ListByEnrollment(ctx context.Context, enrollmentID string, from, to time.Time) ([]service.AttendanceRecord, error) {
	var out []service.AttendanceRecord
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&attendanceModel{}).
			Where("enrollment_id = ?", enrollmentID).
			Order("date DESC, id ASC").
			Limit(maxListLimit)
		if !from.IsZero() {
			q = q.Where("date >= ?::date", from.Format(dateLayout))
		}
		if !to.IsZero() {
			q = q.Where("date <= ?::date", to.Format(dateLayout))
		}
		var rows []attendanceModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing attendance marks", err)
		}
		for _, m := range rows {
			out = append(out, toRecord(m))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
