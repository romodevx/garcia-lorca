package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Retry backoff of a refused publish: the first retry waits outboxBaseDelay,
// each later one twice the previous, never more than outboxMaxDelay. A broker
// that is down for an hour costs a few dozen attempts, not one per sweep.
const (
	outboxBaseDelay = 30 * time.Second
	outboxMaxDelay  = 30 * time.Minute
)

// retryDelay is how long to wait before the next attempt after the attempts-th
// failed one (attempts >= 1): base·2^(attempts-1), capped.
func retryDelay(attempts int) time.Duration {
	d := outboxBaseDelay
	for i := 1; i < attempts && d < outboxMaxDelay; i++ {
		d *= 2
	}
	if d > outboxMaxDelay {
		return outboxMaxDelay
	}
	return d
}

// QueueEvents writes the events in the locked term's transaction.
func (p *periodTx) QueueEvents(_ context.Context, events []ports.OutboxEvent) error {
	for _, e := range events {
		row := outboxModel{TenantID: p.tenantID, EventType: e.Type, Payload: e.Payload}
		if err := p.tx.Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "queueing event", err)
		}
	}
	return nil
}

// DrainOutbox publishes the due events, oldest first, one row per transaction.
//
// Each row is taken FOR UPDATE SKIP LOCKED, so two instances draining at once
// never publish the same event, and neither waits on the other. Publishing
// happens inside the row's transaction: a crash after the send but before the
// mark redelivers rather than loses — at least once; comms dedupes on the
// report card id + version. A refused publish counts the attempt and pushes
// next_attempt_at out by retryDelay; it is not a drain error.
func (r *Repository) DrainOutbox(ctx context.Context, limit int, publish func(ports.OutboxEvent) error) (int, error) {
	published := 0
	for taken := 0; taken < limit; taken++ {
		done := false
		err := r.withTenant(ctx, func(tx *gorm.DB) error {
			// Find, not Take: an empty queue is the ordinary answer, and Take
			// would log "record not found" on every sweep.
			var rows []outboxModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("published_at IS NULL AND next_attempt_at <= now()").
				Order("created_at, id").Limit(1).Find(&rows).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "reading the outbox", err)
			}
			if len(rows) == 0 {
				done = true
				return nil
			}
			row := rows[0]
			attempts := row.Attempts + 1
			if perr := publish(ports.OutboxEvent{Type: row.EventType, Payload: row.Payload}); perr != nil {
				return tx.Model(&outboxModel{}).Where("id = ?", row.ID).Updates(map[string]any{
					"attempts":        attempts,
					"next_attempt_at": gorm.Expr("now() + make_interval(secs => ?)", retryDelay(attempts).Seconds()),
				}).Error
			}
			published++
			return tx.Model(&outboxModel{}).Where("id = ?", row.ID).Updates(map[string]any{
				"published_at": gorm.Expr("now()"),
				"attempts":     attempts,
			}).Error
		})
		if err != nil {
			return published, glerrors.Wrap(glerrors.KindInternal, "draining the outbox", err)
		}
		if done {
			break
		}
	}
	return published, nil
}
