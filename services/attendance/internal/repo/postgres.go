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

// subjectScope narrows a query to one list's subject: IS NULL for the daily list
// (a NULL never equals anything), equality for a per-class list. Kept as two
// plain predicates rather than IS NOT DISTINCT FROM so the planner can use the
// key's index.
func subjectScope(tx *gorm.DB, subjectID string) *gorm.DB {
	if subjectID == "" {
		return tx.Where("subject_id IS NULL")
	}
	return tx.Where("subject_id = ?", subjectID)
}

// MarksForList batch-loads the stored marks of many enrollments on one list in a
// single tenant-bound SELECT, collapsing what would otherwise be one query per
// roster student (N+1). The returned map is keyed by enrollment_id and holds a
// record only for enrollments that actually have an exception; present students —
// and any row hidden by RLS — are simply absent (no rows is not an error). An
// empty enrollmentIDs slice returns an empty map without querying.
func (r *Repo) MarksForList(ctx context.Context, enrollmentIDs []string, key service.ListKey) (map[string]service.AttendanceRecord, error) {
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		rows, err := r.loadList(tx, enrollmentIDs, key, false)
		if err != nil {
			return err
		}
		for id, rec := range rows {
			out[id] = rec
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// loadList reads one list's marks for enrollmentIDs, keyed by enrollment id.
// forUpdate row-locks them, so a reconcile plans against rows no concurrent save
// can change underneath it.
func (r *Repo) loadList(tx *gorm.DB, enrollmentIDs []string, key service.ListKey, forUpdate bool) (map[string]service.AttendanceRecord, error) {
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	q := subjectScope(tx.Where("enrollment_id IN ? AND date = ?::date", enrollmentIDs, key.Date.Format(dateLayout)), key.SubjectID)
	if forUpdate {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []attendanceModel
	if err := q.Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading attendance marks", err)
	}
	for _, m := range rows {
		out[m.EnrollmentID] = toRecord(m)
	}
	return out, nil
}

// SessionFor returns one list's session; found is false for a list nobody has
// saved (or one RLS hides).
func (r *Repo) SessionFor(ctx context.Context, key service.ListKey) (service.Session, bool, error) {
	var (
		out   service.Session
		found bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []sessionModel
		q := subjectScope(tx.Where("group_id = ? AND date = ?::date", key.GroupID, key.Date.Format(dateLayout)), key.SubjectID)
		if err := q.Limit(1).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "loading attendance session", err)
		}
		if len(rows) == 1 {
			out, found = toSession(rows[0]), true
		}
		return nil
	})
	if err != nil {
		return service.Session{}, false, err
	}
	return out, found, nil
}

// ReconcileList applies one save to a list inside ONE tenant-bound transaction:
//
//  1. load the list's current marks for enrollmentIDs, row-locked;
//  2. ask plan for the changes (the service's DiffList);
//  3. write inserts and in-place updates as ONE upsert, delete cleared rows by id;
//  4. upsert the list's session.
//
// The write is an UPSERT on the (tenant, enrollment, date, subject) key — NULLS
// NOT DISTINCT, so a daily list's NULL subjects collide as they should. An update
// carries the stored id and lands on the same row, so the id survives a
// correction; a concurrent save that inserted the same student·list first is
// absorbed as an update too (last write wins) instead of failing on the key.
// Every statement is bounded by RLS, so another tenant's rows are untouched.
func (r *Repo) ReconcileList(ctx context.Context, key service.ListKey, enrollmentIDs []string, taker service.Taker, plan service.PlanFunc) (service.ListChanges, error) {
	var applied service.ListChanges
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		current, err := r.loadList(tx, enrollmentIDs, key, true)
		if err != nil {
			return err
		}
		changes, err := plan(current)
		if err != nil {
			return err
		}
		if err := r.deleteMarks(tx, changes.Deletes); err != nil {
			return err
		}
		if err := r.upsertMarks(tx, changes); err != nil {
			return err
		}
		if err := r.upsertSession(tx, key, taker); err != nil {
			return err
		}
		applied = changes
		return nil
	})
	if err != nil {
		return service.ListChanges{}, err
	}
	return applied, nil
}

func (r *Repo) deleteMarks(tx *gorm.DB, cleared []service.AttendanceRecord) error {
	if len(cleared) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cleared))
	for _, rec := range cleared {
		ids = append(ids, rec.ID)
	}
	if err := tx.Where("id IN ?", ids).Delete(&attendanceModel{}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "clearing attendance marks", err)
	}
	return nil
}

func (r *Repo) upsertMarks(tx *gorm.DB, changes service.ListChanges) error {
	models := make([]attendanceModel, 0, len(changes.Inserts)+len(changes.Updates))
	for _, rec := range changes.Inserts {
		models = append(models, fromRecord(rec, r.tenantID))
	}
	for _, u := range changes.Updates {
		models = append(models, fromRecord(u.Record, r.tenantID))
	}
	if len(models) == 0 {
		return nil
	}
	err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "enrollment_id"}, {Name: "date"}, {Name: "subject_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"status":     gorm.Expr("excluded.status"),
			"note":       gorm.Expr("excluded.note"),
			"updated_at": gorm.Expr("now()"),
		}),
	}).Create(&models).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "storing attendance marks", err)
	}
	return nil
}

// upsertSession records that the list was taken. The first save's taker and
// taken_at are kept; a later save only bumps updated_at.
func (r *Repo) upsertSession(tx *gorm.DB, key service.ListKey, taker service.Taker) error {
	row := sessionModel{
		TenantID:    r.tenantID,
		GroupID:     key.GroupID,
		Date:        key.Date,
		SubjectID:   nullable(key.SubjectID),
		TakenBy:     taker.UserID,
		TakenByName: taker.Name,
	}
	err := tx.Omit("id", "taken_at", "updated_at").Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "group_id"}, {Name: "date"}, {Name: "subject_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{"updated_at": gorm.Expr("now()")}),
	}).Create(&row).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "recording attendance session", err)
	}
	return nil
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
