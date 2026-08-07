package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the adapter satisfies the seam the service declares.
var _ service.Repo = (*Repo)(nil)

// maxListLimit caps the unpaginated reads, so neither a long-lived notification
// feed nor a tenant-wide push fanout can pull an unbounded number of rows into
// memory. It is a DOCUMENTED bound, not a hidden one: the capped methods say so,
// as does the proto contract. Explicit pagination is the follow-up that removes
// the need for it.
const maxListLimit = 500

// Repo is the GORM-backed comms repository. Every operation runs in a transaction
// bound to a tenant so Row Level Security applies (mirrors
// iam/students/grades/attendance, ADR 0001). In this phase the tenant is fixed at
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

// RecordEvent claims eventKey in the consumed-event ledger and materialises n in
// ONE tenant-bound transaction. isNew reports whether the claim was actually
// made: a redelivered event conflicts on the ledger's (tenant_id, event_key)
// primary key, is absorbed by ON CONFLICT DO NOTHING, and returns isNew=false
// with nothing written.
//
// Both writes live in the same transaction on purpose. Were the claim committed
// on its own, a failure while storing the notification would leave the event
// marked handled with nothing to show for it — and because the ledger now skips
// the redelivery, that notification would be lost rather than retried. Rolling
// the claim back with the notification is what makes redelivery the recovery.
//
// The ledger row is scoped by RLS, so one tenant handling an event never
// suppresses another tenant's identical event.
func (r *Repo) RecordEvent(ctx context.Context, eventKey string, n service.Notification) (service.Notification, bool, error) {
	var (
		out   service.Notification
		isNew bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		claim := processedEventModel{TenantID: r.tenantID, EventKey: eventKey}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "claiming consumed event", res.Error)
		}
		if res.RowsAffected == 0 {
			return nil // already handled: leave isNew false and write nothing
		}

		m := fromNotification(n, r.tenantID)
		if err := tx.Create(&m).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing notification", err)
		}
		isNew = true
		out = toNotification(m)
		return nil
	})
	if err != nil {
		return service.Notification{}, false, err
	}
	return out, isNew, nil
}

// ListNotifications returns one student's notifications, and with unreadOnly only
// those whose read_at is unset (the badge count).
//
// The read returns AT MOST maxListLimit rows, the MOST RECENT first, ordered by
// (created_at, id) so the cut is deterministic rather than whatever the planner
// happened to emit. Newest-first is what makes the cap safe: a student with years
// of history loses the oldest entries, never the ones on screen. Explicit
// pagination is a follow-up.
func (r *Repo) ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]service.Notification, error) {
	var out []service.Notification
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&notificationModel{}).
			Where("student_id = ?", studentID).
			Order("created_at DESC, id ASC").
			Limit(maxListLimit)
		if unreadOnly {
			q = q.Where("read_at IS NULL")
		}
		var rows []notificationModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing notifications", err)
		}
		for _, m := range rows {
			out = append(out, toNotification(m))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkRead stamps a notification as read and returns the stored row. The update
// is conditional on read_at IS NULL, which is what makes re-marking idempotent:
// an already-read notification keeps its ORIGINAL instant instead of silently
// moving to now. An id no row matches — including a row hidden by RLS — is
// glerrors.ErrNotFound.
func (r *Repo) MarkRead(ctx context.Context, id string) (service.Notification, error) {
	var out service.Notification
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&notificationModel{}).
			Where("id = ? AND read_at IS NULL", id).
			Update("read_at", gorm.Expr("now()"))
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "marking notification read", res.Error)
		}

		// Re-read in the same transaction: it both returns the stamped row and is
		// what distinguishes "already read" (found, untouched) from "no such
		// notification" (the conditional update matches nothing either way).
		var stored notificationModel
		if err := tx.Where("id = ?", id).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		out = toNotification(stored)
		return nil
	})
	if err != nil {
		return service.Notification{}, err
	}
	return out, nil
}

// UpsertDevice registers d, keyed on (tenant_id, fcm_token) rather than on the
// id: a client re-registering the same token updates the stored row's owner and
// platform and KEEPS its id, so a phone that calls RegisterDevice on every launch
// is never fanned out twice. The returned device is the stored row, whose id may
// therefore differ from the submitted one.
func (r *Repo) UpsertDevice(ctx context.Context, d service.Device) (service.Device, error) {
	var out service.Device
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		m := fromDevice(d, r.tenantID)
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "fcm_token"}},
			DoUpdates: clause.Assignments(map[string]any{
				"user_id":  gorm.Expr("excluded.user_id"),
				"platform": gorm.Expr("excluded.platform"),
			}),
		}).Create(&m).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "registering device", err)
		}

		var stored deviceModel
		if err := tx.Where("fcm_token = ?", d.FCMToken).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		out = toDevice(stored)
		return nil
	})
	if err != nil {
		return service.Device{}, err
	}
	return out, nil
}

// DevicesForTenant returns the tenant's push targets, AT MOST maxListLimit of
// them, ordered by (created_at, id) so the set a push fans out to is
// deterministic.
//
// Targeting is deliberately coarse in this phase: the guardian↔student link that
// would say WHICH devices belong to a notification's student does not exist yet,
// so the push goes tenant-wide while the per-student feed — the durable part of
// the product — is already exact. Narrowing this to a student's guardians is the
// carry-forward, and it is why the push is a courtesy rather than the delivery
// guarantee.
func (r *Repo) DevicesForTenant(ctx context.Context) ([]service.Device, error) {
	var out []service.Device
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []deviceModel
		if err := tx.Model(&deviceModel{}).
			Order("created_at ASC, id ASC").
			Limit(maxListLimit).
			Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing push devices", err)
		}
		for _, m := range rows {
			out = append(out, toDevice(m))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// translateFindErr maps GORM's "no row" into the shared NotFound sentinel and
// wraps anything else as internal. A row hidden by RLS is indistinguishable from
// a missing one here, which is exactly the behaviour tenant isolation wants.
func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}
