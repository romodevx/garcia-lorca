package repo

import (
	"context"
	"fmt"
	"strings"
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
	// listLockNamespace prefixes a list's advisory-lock key so it cannot hash
	// alongside another feature's locks by construction of the text.
	listLockNamespace = "attendance-list"
	// upsertMarkColumns is how many bound values one upsertMarkRow carries.
	upsertMarkColumns = 7
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
//  1. take the list's transaction-scoped advisory lock, so two saves of the
//     same list run one after the other;
//  2. load the list's current marks for enrollmentIDs, row-locked;
//  3. ask plan for the changes (the service's DiffList);
//  4. write inserts and in-place updates as ONE upsert, delete cleared rows by id;
//  5. upsert the list's session.
//
// The advisory lock is what makes the plan honest. Row locks only cover rows
// that exist, so without it two concurrent saves of a student nobody had marked
// would both plan an INSERT; the key would absorb the loser as an update, yet it
// would still be reported — and published — as an insert with an id that was
// never stored. Serialized, the second save plans against the first one's row.
//
// The upsert is still keyed on (tenant, enrollment, date, subject) — NULLS NOT
// DISTINCT, so a daily list's NULL subjects collide — and its RETURNING clause
// decides what is reported: the STORED id of every row, and whether Postgres
// inserted or updated it. A planned insert that landed on an existing row (a
// writer outside this method) is reported as a status-changing update of the
// stored row, never as a second insert. Every statement is bounded by RLS, so
// another tenant's rows are untouched.
func (r *Repo) ReconcileList(ctx context.Context, key service.ListKey, enrollmentIDs []string, taker service.Taker, plan service.PlanFunc) (service.ListChanges, error) {
	var applied service.ListChanges
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := r.lockList(tx, key); err != nil {
			return err
		}
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
		stored, err := r.upsertMarks(tx, changes)
		if err != nil {
			return err
		}
		if err := r.upsertSession(tx, key, taker); err != nil {
			return err
		}
		applied = asStored(changes, stored)
		return nil
	})
	if err != nil {
		return service.ListChanges{}, err
	}
	return applied, nil
}

// lockList takes the transaction-scoped advisory lock of one list (tenant,
// group, date, subject). It is released with the transaction, so it can never
// leak; saves of different lists never wait on each other (barring a hash
// collision, which only costs a brief wait).
func (r *Repo) lockList(tx *gorm.DB, key service.ListKey) error {
	lockKey := strings.Join([]string{listLockNamespace, r.tenantID, key.GroupID, key.Date.Format(dateLayout), key.SubjectID}, "|")
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "locking attendance list", err)
	}
	return nil
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

// storedMark is what the upsert's RETURNING clause says about one row: its
// stored id, its stored revision, and whether this statement inserted it
// (xmax = 0) or updated it.
type storedMark struct {
	ID           string
	EnrollmentID string
	Revision     int64
	Inserted     bool
}

// upsertMarkSQL writes a batch of marks as ONE statement. The VALUES rows are
// appended by upsertMarks.
const upsertMarkSQL = `INSERT INTO attendance (id, tenant_id, enrollment_id, date, subject_id, status, note, created_at, updated_at) VALUES %s
ON CONFLICT (tenant_id, enrollment_id, date, subject_id)
DO UPDATE SET status = excluded.status, note = excluded.note, revision = attendance.revision + 1, updated_at = now()
RETURNING id, enrollment_id, revision, (xmax = 0) AS inserted`

// upsertMarkRow is one VALUES row of upsertMarkSQL.
const upsertMarkRow = "(?, ?, ?, ?::date, ?, ?, ?, now(), now())"

// upsertMarks stores inserts and in-place updates as one upsert and returns
// what Postgres actually did to each row, keyed by enrollment id.
func (r *Repo) upsertMarks(tx *gorm.DB, changes service.ListChanges) (map[string]storedMark, error) {
	records := make([]service.AttendanceRecord, 0, len(changes.Inserts)+len(changes.Updates))
	records = append(records, changes.Inserts...)
	for _, u := range changes.Updates {
		records = append(records, u.Record)
	}
	out := make(map[string]storedMark, len(records))
	if len(records) == 0 {
		return out, nil
	}
	rows := make([]string, 0, len(records))
	args := make([]any, 0, len(records)*upsertMarkColumns)
	for _, rec := range records {
		rows = append(rows, upsertMarkRow)
		args = append(args, rec.ID, r.tenantID, rec.EnrollmentID, rec.Date.Format(dateLayout), nullable(rec.SubjectID), rec.Status, rec.Note)
	}
	var stored []storedMark
	if err := tx.Raw(fmt.Sprintf(upsertMarkSQL, strings.Join(rows, ", ")), args...).Scan(&stored).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "storing attendance marks", err)
	}
	for _, m := range stored {
		out[m.EnrollmentID] = m
	}
	return out, nil
}

// asStored rewrites a plan into what was stored: every insert and update
// carries the row's STORED id and revision, and a planned insert that Postgres
// turned into an update (it met an existing row) is reported as a
// status-changing update.
func asStored(planned service.ListChanges, stored map[string]storedMark) service.ListChanges {
	out := service.ListChanges{Deletes: planned.Deletes}
	for _, rec := range planned.Inserts {
		m, ok := stored[rec.EnrollmentID]
		if ok {
			rec.ID, rec.Revision = m.ID, m.Revision
		}
		if !ok || m.Inserted {
			out.Inserts = append(out.Inserts, rec)
			continue
		}
		out.Updates = append(out.Updates, service.RecordUpdate{Record: rec, StatusChanged: true})
	}
	for _, u := range planned.Updates {
		if m, ok := stored[u.Record.EnrollmentID]; ok {
			u.Record.ID, u.Record.Revision = m.ID, m.Revision
		}
		out.Updates = append(out.Updates, u)
	}
	return out
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
