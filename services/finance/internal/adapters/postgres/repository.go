package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.FinanceRepository = (*Repository)(nil)

const (
	uniqueViolation = "23505"
	// dateLayout renders a Go time as the DATE literal the `due_date` column is
	// compared against, so only the date part of the caller's clock matters.
	dateLayout = "2006-01-02"
	// maxListLimit caps every unpaginated ledger read, so no single request can
	// pull an unbounded number of rows into memory (defense in depth behind the
	// use case's scoping filters). It is a DOCUMENTED bound, not a hidden one:
	// each capped method says so. A cycle-wide read gets past it through the
	// keyset pages (ListChargesPage, ListOverduePage), whose size is
	// ports.MaxChargePageSize.
	maxListLimit = 500
)

// Repository is the GORM-backed finance ledger repository. Every operation runs
// in a transaction bound to a tenant so Row Level Security applies (mirrors
// iam/students/grades, ADR 0001). In this phase the tenant is fixed at
// construction (single-tenant).
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

// --- catalog ----------------------------------------------------------------

// CreateConcept stores a new catalog entry. A duplicate (tenant, name) is
// KindConflict.
func (r *Repository) CreateConcept(ctx context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error) {
	model := fromDomainConcept(c)
	var out domain.ChargeConcept
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			if isUniqueViolation(err) {
				return glerrors.Wrap(glerrors.KindConflict, "charge concept already exists", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "creating charge concept", err)
		}
		stored, err := toDomainConcept(model)
		if err != nil {
			return err
		}
		out = stored
		return nil
	})
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return out, nil
}

// GetConcept loads one catalog entry; a missing id yields glerrors.ErrNotFound.
func (r *Repository) GetConcept(ctx context.Context, id string) (domain.ChargeConcept, error) {
	var out domain.ChargeConcept
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m chargeConceptModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		stored, err := toDomainConcept(m)
		if err != nil {
			return err
		}
		out = stored
		return nil
	})
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return out, nil
}

// ListConcepts returns the catalog ordered by name. Deactivated entries are
// hidden unless includeInactive is set — they are never deleted, because issued
// charges keep pointing at them.
func (r *Repository) ListConcepts(ctx context.Context, includeInactive bool) ([]domain.ChargeConcept, error) {
	var out []domain.ChargeConcept
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeConceptModel{}).Order("name ASC")
		if !includeInactive {
			q = q.Where("active = ?", true)
		}
		var rows []chargeConceptModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing charge concepts", err)
		}
		for _, m := range rows {
			d, err := toDomainConcept(m)
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

// UpdateConcept persists a re-validated copy of an existing entry and returns
// the stored row. Renaming onto another concept's name is KindConflict; an
// unknown id is glerrors.ErrNotFound.
func (r *Repository) UpdateConcept(ctx context.Context, c domain.ChargeConcept) (domain.ChargeConcept, error) {
	var out domain.ChargeConcept
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&chargeConceptModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"name":                 c.Name(),
			"default_amount_cents": c.DefaultAmount().Int64(),
			"recurrence":           c.Recurrence(),
			"active":               c.Active(),
			"updated_at":           gorm.Expr("now()"),
		})
		if res.Error != nil {
			if isUniqueViolation(res.Error) {
				return glerrors.Wrap(glerrors.KindConflict, "charge concept already exists", res.Error)
			}
			return glerrors.Wrap(glerrors.KindInternal, "updating charge concept", res.Error)
		}

		var stored chargeConceptModel
		if err := tx.Where("id = ?", c.ID()).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		d, err := toDomainConcept(stored)
		if err != nil {
			return err
		}
		out = d
		return nil
	})
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return out, nil
}

// --- charges ----------------------------------------------------------------

// CreateCharge issues one charge. A second charge for the same
// (student, cycle, concept) is KindConflict — a student owes a given concept
// once per cycle.
func (r *Repository) CreateCharge(ctx context.Context, c domain.Charge) (domain.Charge, error) {
	model := fromDomainCharge(c)
	var out domain.Charge
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			if isUniqueViolation(err) {
				// Two partial indexes can raise this (migration 0003): the
				// keyless "one concept per cycle" rule, or a repeated external
				// ref — a retried creation for the same origin row. Both mean
				// the same thing to the caller: this charge already exists.
				return glerrors.Wrap(glerrors.KindConflict, "charge already exists", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "creating charge", err)
		}
		// A freshly issued charge has collected nothing yet.
		stored, err := toDomainCharge(model, 0)
		if err != nil {
			return err
		}
		out = stored
		return nil
	})
	if err != nil {
		return domain.Charge{}, err
	}
	return out, nil
}

// RescheduleUnpaid moves the window — opening day and deadline — of charges
// named by external ref.
//
// The run never re-issues a charge it has already written — that is what makes
// it idempotent — and that left a school unable to correct a deadline it had
// already billed: setting the inscription's date did nothing for the four
// hundred pupils already charged, so the setting looked broken.
//
// Guards, all in the WHERE and not in Go: a charge is moved only while it is
// STILL OWING — nothing paid, or part paid (abono, T5) — and only when a date
// actually differs. Still owing, because a settled charge is history and
// re-dating it would rewrite when a family was due to pay something they have
// already paid. Different, because the returned count is what the run reports
// to a person, and "moved 400" every month teaches them to stop reading it.
func (r *Repository) RescheduleUnpaid(ctx context.Context, items []ports.ChargeReschedule) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	type window struct{ starts, due time.Time }
	byWindow := make(map[window][]string, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if seen[item.ExternalRef] {
			continue
		}
		seen[item.ExternalRef] = true
		w := window{starts: item.StartsOn, due: item.DueOn}
		byWindow[w] = append(byWindow[w], item.ExternalRef)
	}

	var moved int
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// One statement per distinct window rather than one per charge: a school
		// prices a level at a time, so the four hundred pupils of a level share
		// the one window. Grouping keeps this a handful of round trips instead
		// of four hundred.
		for w, group := range byWindow {
			starts := optionalDate(w.starts)
			res := tx.Model(&chargeModel{}).
				Where("external_ref IN ?", group).
				Where("(due_date <> ? OR starts_on IS DISTINCT FROM ?::date)", w.due, starts).
				Where("status <> ?", domain.StatusCancelled).
				// Still owing. The subquery is the same tenant-bound view the
				// policy allows, so RLS still applies to it.
				Where("(SELECT COALESCE(SUM(p.amount_cents), 0) FROM payments p WHERE p.charge_id = charges.id) " +
					"< charges.amount_cents - charges.discount_cents - charges.scholarship_cents").
				Updates(map[string]any{"due_date": w.due, "starts_on": starts, "updated_at": gorm.Expr("now()")})
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInternal, "moving a deadline", res.Error)
			}
			moved += int(res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return moved, nil
}

// chargeInsertBatchSize bounds one INSERT of charges. Postgres caps a statement
// at 65535 parameters and a charge binds 18 columns, so a single INSERT broke
// near 3640 rows: the first full-school billing run answered 500 and billed
// nothing. 1000 rows binds 18000 parameters, and stays under the cap while the
// table has at most 65 columns.
const chargeInsertBatchSize = 1000

// CreateChargesBulk issues many charges in one transaction, skipping the rows
// whose (student, cycle, concept) key already exists instead of failing the
// batch. Postgres reports only the rows actually inserted, so re-running a
// generation is idempotent and the caller can infer the skipped count from
// len(cs) - created.
func (r *Repository) CreateChargesBulk(ctx context.Context, cs []domain.Charge) (int, error) {
	if len(cs) == 0 {
		return 0, nil
	}

	// The batch is split by whether the charge carries an external ref, because
	// the two kinds are deduplicated by DIFFERENT partial indexes (migration
	// 0003) and ON CONFLICT takes exactly one arbiter.
	//
	// A keyless charge is one per (student, cycle, concept): a student owes a
	// given concept once a cycle. A referenced charge is one per ref, which is
	// what lets a school year hold ten tuition charges for the same concept.
	//
	// Naming only the keyless arbiter — which this did until M15 — meant a
	// referenced charge collided against the index that was NOT the arbiter, so
	// Postgres raised a duplicate-key error instead of skipping. Nothing was
	// wrong until the billing run started using refs, and then the second run of
	// every month failed outright.
	var keyless, referenced []chargeModel
	for _, c := range cs {
		model := fromDomainCharge(c)
		if c.ExternalRef() == "" {
			keyless = append(keyless, model)
			continue
		}
		referenced = append(referenced, model)
	}

	// NOTE, for whoever changes the two TargetWhere clauses below: the
	// testcontainers harness does NOT reproduce the failure they guard against.
	// The integration tests here pass with a parameterised predicate and fail only
	// against the compose database, on the same Postgres 16 and the same GORM
	// config — the difference was never found. So the literals are load-bearing
	// and untested: verify a change to them against a running stack, not just
	// against `go test -tags=integration`.
	var created int
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// The target is spelled out on purpose in both cases. A bare
		// OnConflict{DoNothing} infers its arbiter from whatever unique index
		// happens to match, so a change to the table's keys could stop this
		// deduplicating with no error and re-bill the whole school.
		if len(keyless) > 0 {
			res := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "student_id"}, {Name: "cycle_id"}, {Name: "concept_id"}},
				// TargetWhere, not Where: this is the predicate of the PARTIAL
				// index being used as the arbiter, not a condition on DO UPDATE.
				//
				// A raw expression, not clause.Eq, and that is not a style choice:
				// clause.Eq binds the '' as a PARAMETER, and Postgres cannot prove
				// a parameterised predicate implies a partial index's, so it
				// answers "there is no unique or exclusion constraint matching the
				// ON CONFLICT specification". The literal is what makes the
				// arbiter resolvable.
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "external_ref = ''"},
				}},
				DoNothing: true,
			}).CreateInBatches(&keyless, chargeInsertBatchSize)
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInternal, "bulk creating charges", res.Error)
			}
			created += int(res.RowsAffected)
		}
		if len(referenced) > 0 {
			res := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "external_ref"}},
				// Literal, for the same reason as above: a bound parameter makes
				// the partial index unresolvable as an arbiter.
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "external_ref <> ''"},
				}},
				DoNothing: true,
			}).CreateInBatches(&referenced, chargeInsertBatchSize)
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInternal, "bulk creating charges", res.Error)
			}
			created += int(res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

// CancelChargeIfUnpaid voids a charge only when nothing has been collected
// against it, deciding under the charge's row lock so the answer cannot go stale
// between the read and the write.
//
// It exists for automated callers, and the uniforms expiry sweep is the first:
// it voids the charge behind a hold nobody came to pay for, and it runs while a
// cashier may be collecting that exact payment. CancelCharge would void the
// charge anyway (the school does that deliberately — see domain.Charge.Cancel),
// which here would mean money collected against a cancelled charge and released
// goods. Taking the same FOR UPDATE lock RegisterPayment takes makes the two
// serialize: whichever runs second sees the other's work and the loser gets a
// conflict instead of a silent inconsistency.
func (r *Repository) CancelChargeIfUnpaid(ctx context.Context, id string) (ports.ChargeWithPaid, error) {
	return r.CancelChargeIfUnpaidWithReason(ctx, id, "")
}

// CancelChargeIfUnpaidWithReason applies the guarded cancellation and its audit
// reason under the same row lock as payment posting.
func (r *Repository) CancelChargeIfUnpaidWithReason(ctx context.Context, id, reason string) (ports.ChargeWithPaid, error) {
	var out ports.ChargeWithPaid
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var row chargeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&row).Error; err != nil {
			return translateFindErr(err)
		}
		paid, err := paidFor(tx, id)
		if err != nil {
			return err
		}
		charge, err := toDomainCharge(row, paid)
		if err != nil {
			return err
		}
		cancelled, err := charge.CancelIfUnpaidWithReason(paid, reason)
		if err != nil {
			return err
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", id).Updates(map[string]any{
			"status":              cancelled.Status(),
			"cancellation_reason": cancelled.CancellationReason(),
			"updated_at":          gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "cancelling charge", err)
		}
		out = ports.ChargeWithPaid{Charge: cancelled, PaidCents: paid}
		return nil
	})
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return out, nil
}

// RepriceUnpaid sets the amount of charges named by external ref, for the ones
// nobody has paid anything against.
//
// Grouped by amount for the same reason RescheduleUnpaid groups by date: a
// school prices a LEVEL, so the four hundred pupils of that level share the one
// new price, and grouping keeps this a handful of round trips instead of four
// hundred.
//
// The NOT EXISTS on payments is the enforcement point of ADR 0011's freeze rule,
// and it lives here rather than in the use case because it has to be part of the
// same statement that writes: a check that read first would leave a window where
// a cashier takes money between the read and the update, and the family's
// receipt would stop matching their statement.
func (r *Repository) RepriceUnpaid(ctx context.Context, items []ports.ChargeReprice) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	amountByRef := make(map[string]int64, len(items))
	for _, item := range items {
		if _, seen := amountByRef[item.ExternalRef]; seen {
			continue
		}
		amountByRef[item.ExternalRef] = item.AmountCents
	}

	var repriced int
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		byAmount := make(map[int64][]string, len(amountByRef))
		for ref, amount := range amountByRef {
			byAmount[amount] = append(byAmount[amount], ref)
		}
		for amount, group := range byAmount {
			res := tx.Model(&chargeModel{}).
				Where("external_ref IN ?", group).
				// Only upward. A school that edits its table cheaper must not
				// walk four hundred already-quoted inscriptions down with it: a
				// charge that has been issued is a number a family has been told,
				// and lowering it is a decision somebody makes by hand — which is
				// what a discount is for.
				Where("amount_cents < ?", amount).
				Where("status <> ?", domain.StatusCancelled).
				// Nothing negotiated. A discount is a price a PERSON decided for
				// one family, and a price list must not overwrite it. The full
				// scholarship proves the rule: it never receives a peso, so the
				// payment guard below never protects it, and raising the amount
				// while the discount stands turns a gift into a debt.
				//
				// It is also what keeps one row from killing the batch. This
				// statement covers a whole level, and a downward re-price under a
				// discount would violate CHECK (discount_cents <= amount_cents),
				// rolling back every other pupil's price and failing the nightly
				// run wholesale over one scholarship.
				Where("discount_cents = 0").
				// Nor a scholarship's part written when it was settled (T18).
				Where("scholarship_cents = 0").
				// Nothing collected. The subquery is the same tenant-bound view
				// the policy allows, so RLS still applies to it.
				Where("NOT EXISTS (SELECT 1 FROM payments p WHERE p.charge_id = charges.id)").
				Updates(map[string]any{"amount_cents": amount, "updated_at": gorm.Expr("now()")})
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInternal, "re-pricing an inscription", res.Error)
			}
			repriced += int(res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return repriced, nil
}

// GetCharge loads one charge together with its collected total; a missing id
// yields glerrors.ErrNotFound.
func (r *Repository) GetCharge(ctx context.Context, id string) (ports.ChargeWithPaid, error) {
	var out ports.ChargeWithPaid
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m chargeModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		paid, err := paidFor(tx, id)
		if err != nil {
			return err
		}
		charge, err := toDomainCharge(m, paid)
		if err != nil {
			return err
		}
		out = ports.ChargeWithPaid{Charge: charge, PaidCents: paid}
		return nil
	})
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	return out, nil
}

// ListCharges returns charges with their collected totals. An empty studentID or
// cycleID skips that filter. The payment totals for the whole page are resolved
// in ONE grouped query rather than a lookup per charge (no N+1).
//
// The read returns AT MOST maxListLimit rows, the earliest due first, ordered by
// (due_date, id) so the cut is deterministic rather than whatever the planner
// happened to emit. It serves the per-pupil reads; a cycle-wide read walks
// ListChargesPage, because a cycle holds thousands of charges.
func (r *Repository) ListCharges(ctx context.Context, studentID, cycleID string) ([]ports.ChargeWithPaid, error) {
	var out []ports.ChargeWithPaid
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeModel{}).Order("due_date ASC, id ASC").Limit(maxListLimit)
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		var rows []chargeModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing charges", err)
		}
		assembled, err := withPaidTotals(tx, rows)
		if err != nil {
			return err
		}
		out = assembled
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListIssuedLines returns the generated inscription and fee lines of a cycle,
// optionally for one student. See ports.FinanceRepository.
//
// Served by idx_charges_cycle (tenant_id, cycle_id); the prefix filter runs on
// the cycle's rows. Uncapped on purpose: the run reads the whole cycle, and a
// capped read would re-issue every line past the cap.
func (r *Repository) ListIssuedLines(ctx context.Context, cycleID, studentID string) ([]ports.IssuedLine, error) {
	var rows []chargeModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeModel{}).
			Select("student_id", "external_ref", "description").
			Where("cycle_id = ?", cycleID).
			Where("(external_ref LIKE ? OR external_ref LIKE ?)",
				domain.EnrollmentRefPrefix+"%", domain.FeeRefPrefix+"%").
			Where("status <> ?", domain.StatusCancelled).
			Order("student_id ASC, external_ref ASC")
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing issued lines", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.IssuedLine, 0, len(rows))
	for _, m := range rows {
		out = append(out, ports.IssuedLine{StudentID: m.StudentID, ExternalRef: m.ExternalRef, Description: m.Description})
	}
	return out, nil
}

// SaveCharge persists the mutable part of an existing charge (its discount and
// derived status), as left by ApplyDiscount or CancelCharge, and returns the
// stored row. An unknown id is glerrors.ErrNotFound.
func (r *Repository) SaveCharge(ctx context.Context, c domain.Charge) (domain.Charge, error) {
	var out domain.Charge
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// Deliberately NOT amount_cents. Both callers (ApplyDiscount and
		// CancelCharge) read the charge, change it and write it back OUTSIDE a
		// transaction; writing the price here would put the amount they read
		// minutes ago back into the row, undoing a re-price that landed in
		// between — the ladder walking backwards in silence, which is exactly
		// what the one-way ratchet exists to prevent.
		//
		// Re-pricing has its own two paths, and both decide under a lock or in
		// one statement: RepriceUnpaid for the run, DepositWrite.Repriced for the
		// counter. A price that moves needs one of those, never this.
		res := tx.Model(&chargeModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"discount_cents": c.Discount().Int64(),
			"status":         c.Status(),
			"updated_at":     gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving charge", res.Error)
		}

		var stored chargeModel
		if err := tx.Where("id = ?", c.ID()).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		// Re-derive from the row's own collected total so the returned entity is
		// the canonical stored state, not the caller's copy.
		paid, err := paidFor(tx, c.ID())
		if err != nil {
			return err
		}
		charge, err := toDomainCharge(stored, paid)
		if err != nil {
			return err
		}
		out = charge
		return nil
	})
	if err != nil {
		return domain.Charge{}, err
	}
	return out, nil
}

// --- payments ---------------------------------------------------------------

// RegisterPayment posts a payment against a charge atomically. It locks the
// charge row FOR UPDATE, sums what has already been collected, and hands the
// charge plus the prospective new total to derive, which owns the decision: an
// error from derive aborts the transaction, so a rejected payment (cancelled
// charge, overpay) writes NOTHING. The row lock is what makes those rules
// race-free — two concurrent payments serialize on it instead of both reading a
// stale total. On success the payment row and the charge's new status are
// written in the same transaction. A missing charge yields glerrors.ErrNotFound.
//
// Deduplication happens under that same lock: a payment carrying an idempotency
// key the tenant has already used returns the ORIGINAL payment with
// replayed=true and writes nothing, so a retried request cannot collect twice.
// Looking it up after acquiring the lock is what makes the check race-free for
// the common case — a concurrent retry of the same charge waits, then sees the
// committed row. Two retries against DIFFERENT charges share no lock, and there
// the partial unique index (migration 0002) is the backstop: the loser's insert
// fails rather than double-charging.
func (r *Repository) RegisterPayment(
	ctx context.Context,
	ps []domain.Payment,
	derive func(charge domain.Charge, newPaidCents int64) (string, error),
	outbox func(charge ports.ChargeWithPaid) []ports.OutboxEvent,
) ([]domain.Payment, ports.ChargeWithPaid, bool, error) {
	if len(ps) == 0 {
		return nil, ports.ChargeWithPaid{}, false, glerrors.New(glerrors.KindInvalid, "a payment needs at least one row")
	}
	first := ps[0]
	var (
		storedPayments []domain.Payment
		result         ports.ChargeWithPaid
		replayed       bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var row chargeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", first.ChargeID()).First(&row).Error; err != nil {
			return translateFindErr(err)
		}

		currentPaid, err := paidFor(tx, first.ChargeID())
		if err != nil {
			return err
		}
		charge, err := toDomainCharge(row, currentPaid)
		if err != nil {
			return err
		}

		original, found, err := paymentsByIdempotencyKey(tx, first.IdempotencyKey())
		if err != nil {
			return err
		}
		if found {
			storedPayments = original
			result = ports.ChargeWithPaid{Charge: charge, PaidCents: currentPaid}
			replayed = true
			return nil
		}

		newPaid := currentPaid
		for _, p := range ps {
			if p.ChargeID() != first.ChargeID() {
				return glerrors.New(glerrors.KindInvalid, "a combined payment pays one charge")
			}
			newPaid += p.Amount().Int64()
		}
		status, err := derive(charge, newPaid)
		if err != nil {
			return err
		}

		storedPayments = make([]domain.Payment, 0, len(ps))
		for _, p := range ps {
			payment, err := fromDomainPayment(p)
			if err != nil {
				return err
			}
			if err := tx.Create(&payment).Error; err != nil {
				// The partial unique index caught a same-key attempt that did not
				// serialize on this charge's lock. It is a retry, not a new payment,
				// so the caller sees a conflict rather than a duplicated collection.
				if isUniqueViolation(err) {
					return glerrors.Wrap(glerrors.KindConflict, "payment already registered under this idempotency key", err)
				}
				return glerrors.Wrap(glerrors.KindInternal, "storing payment", err)
			}
			d, err := toDomainPayment(payment)
			if err != nil {
				return err
			}
			storedPayments = append(storedPayments, d)
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", first.ChargeID()).Updates(map[string]any{
			"status":     status,
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating charge status", err)
		}

		// derive has already vetoed cancelled and overpaying charges, so
		// re-deriving from the new total reproduces exactly the status written.
		result = ports.ChargeWithPaid{Charge: charge.WithStatus(newPaid), PaidCents: newPaid}
		if outbox != nil {
			// In the SAME transaction as the money: either both land or neither
			// does, which is the whole point.
			if err := writeOutbox(tx, r.tenantID, outbox(result)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, ports.ChargeWithPaid{}, false, err
	}
	return storedPayments, result, replayed, nil
}

// paymentsByIdempotencyKey looks up the payment set already posted under key,
// if any: the row carrying the key itself and, for a combined payment (T8),
// the rows carrying "key#2", "key#3"… in that order. An empty key never
// matches — it means the caller opted out of deduplication, and the partial
// unique index excludes blank keys for the same reason. The query is
// tenant-bound by RLS, so a key is only ever a duplicate within its own school.
//
// The set's keys are named exactly (IN, never a prefix match): the lookup stays
// on the partial unique index while the charge is locked, and a caller's key
// that merely starts with another's "key#" is never mistaken for its row.
func paymentsByIdempotencyKey(tx *gorm.DB, key string) ([]domain.Payment, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	keys := make([]string, 0, domain.MaxTenders)
	for i := range domain.MaxTenders {
		keys = append(keys, domain.TenderKey(key, i))
	}
	var rows []paymentModel
	if err := tx.Where("idempotency_key IN ?", keys).
		Order("length(idempotency_key), idempotency_key").Find(&rows).Error; err != nil {
		return nil, false, glerrors.Wrap(glerrors.KindInternal, "looking up payment by idempotency key", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	out := make([]domain.Payment, 0, len(rows))
	for _, m := range rows {
		stored, err := toDomainPayment(m)
		if err != nil {
			return nil, false, err
		}
		out = append(out, stored)
	}
	return out, true, nil
}

// ListPayments returns every payment posted against the student's charges, for
// the account statement. The join is tenant-bound on both sides (RLS applies to
// charges and payments alike).
func (r *Repository) ListPayments(ctx context.Context, studentID string) ([]domain.Payment, error) {
	var out []domain.Payment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []paymentModel
		if err := tx.Model(&paymentModel{}).
			Select("payments.*").
			Joins("JOIN charges ON charges.id = payments.charge_id").
			Where("charges.student_id = ?", studentID).
			Order("payments.paid_on ASC, payments.paid_at ASC").
			Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing payments", err)
		}
		for _, m := range rows {
			d, err := toDomainPayment(m)
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

// ListChargesPage returns one keyset page of charges with their collected
// totals. See ports.FinanceRepository.
//
// One row past `limit` is read so the last page is known without sending the
// caller back for an empty one. Keyset (due_date, id) rather than OFFSET: a
// cycle walk runs while the counter writes payments and the billing run writes
// surcharges, and an OFFSET walk would skip or repeat rows as the set shifts.
func (r *Repository) ListChargesPage(ctx context.Context, studentID, cycleID string, after ports.ChargeCursor, limit int) (ports.ChargePage, error) {
	limit = clampPageLimit(limit)
	if err := validateCursor(after); err != nil {
		return ports.ChargePage{}, err
	}
	var page ports.ChargePage
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeModel{})
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		rows, next, err := keysetPage(q, after, limit)
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing charges page", err)
		}
		charges, err := withPaidTotals(tx, rows)
		if err != nil {
			return err
		}
		page = ports.ChargePage{Charges: charges, Next: next}
		return nil
	})
	if err != nil {
		return ports.ChargePage{}, err
	}
	return page, nil
}

// ListOverduePage walks the overdue set in keyset pages. See
// ports.FinanceRepository.
//
// It was written for the billing run, which must see every overdue charge: a
// school with 400 pupils and eleven monthly charges holds ~4,400 charges in a
// cycle, and the capped read it replaced stopped accruing surcharges on the
// NEWEST debt past 500 and said nothing.
//
// The still-owing filter runs after the page's candidates are fetched, so Next
// is the last candidate EXAMINED. Taking it from the last row returned instead
// would stall on a page of settled charges and hide every debt after it.
func (r *Repository) ListOverduePage(ctx context.Context, cycleID string, today time.Time, after ports.ChargeCursor, limit int) (ports.ChargePage, error) {
	limit = clampPageLimit(limit)
	if err := validateCursor(after); err != nil {
		return ports.ChargePage{}, err
	}
	var page ports.ChargePage
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeModel{}).
			Where("due_date < ?::date AND status <> ?", today.Format(dateLayout), domain.StatusCancelled)
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		rows, next, err := keysetPage(q, after, limit)
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing overdue charges page", err)
		}
		candidates, err := withPaidTotals(tx, rows)
		if err != nil {
			return err
		}
		owed := candidates[:0:0]
		for _, c := range candidates {
			if c.Charge.Effective().Int64()-c.PaidCents > 0 {
				owed = append(owed, c)
			}
		}
		page = ports.ChargePage{Charges: owed, Next: next}
		return nil
	})
	if err != nil {
		return ports.ChargePage{}, err
	}
	return page, nil
}

// validateCursor refuses a cursor whose id cannot be a charge id. The token it
// came from decoded, so it is a forgery or a corruption: the caller's mistake,
// which must not reach the uuid cast and come back as a 500.
func validateCursor(after ports.ChargeCursor) error {
	if after.IsZero() {
		return nil
	}
	if _, err := uuid.Parse(after.ID); err != nil {
		return glerrors.New(glerrors.KindInvalid, "invalid page token")
	}
	return nil
}

// clampPageLimit keeps a page inside (0, ports.MaxChargePageSize].
func clampPageLimit(limit int) int {
	if limit <= 0 || limit > ports.MaxChargePageSize {
		return ports.MaxChargePageSize
	}
	return limit
}

// keysetPage reads up to limit rows of q strictly after `after` in (due_date,
// id) order, plus one look-ahead row that only says whether another page
// exists. next is the last row returned when it does, zero when it does not.
func keysetPage(q *gorm.DB, after ports.ChargeCursor, limit int) ([]chargeModel, ports.ChargeCursor, error) {
	if !after.IsZero() {
		// Row-wise comparison: strictly after (due_date, id) in the same total
		// order the ORDER BY imposes.
		q = q.Where("(due_date, id) > (?::date, ?)", after.DueDate.Format(dateLayout), after.ID)
	}
	var rows []chargeModel
	if err := q.Order("due_date ASC, id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, ports.ChargeCursor{}, err
	}
	if len(rows) <= limit {
		return rows, ports.ChargeCursor{}, nil
	}
	rows = rows[:limit]
	last := rows[limit-1]
	return rows, ports.ChargeCursor{DueDate: last.DueDate, ID: last.ID}, nil
}

// --- helpers ----------------------------------------------------------------

// paidFor sums the payments collected against one charge. COALESCE keeps "no
// payments yet" a zero rather than a NULL.
func paidFor(tx *gorm.DB, chargeID string) (int64, error) {
	var paid int64
	if err := tx.Model(&paymentModel{}).
		Where("charge_id = ?", chargeID).
		Select("COALESCE(SUM(amount_cents), 0)").
		Scan(&paid).Error; err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "summing payments", err)
	}
	return paid, nil
}

// withPaidTotals pairs each charge row with its collected total, resolving every
// total in ONE grouped query instead of a lookup per charge. Charges with no
// payments are simply absent from the sum result and default to zero.
func withPaidTotals(tx *gorm.DB, rows []chargeModel) ([]ports.ChargeWithPaid, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(rows))
	for _, m := range rows {
		ids = append(ids, m.ID)
	}

	type chargePaid struct {
		ChargeID  string `gorm:"column:charge_id"`
		Paid      int64  `gorm:"column:paid"`
		PaidByDue int64  `gorm:"column:paid_by_due"`
	}
	// paid_by_due is what was paid on or before the due date, by the payment's
	// school-local day (paid_on, ADR 0035) — never paid_at, whose UTC date is
	// the next day for a live payment after 18:00 in Morelia: what decides
	// whether a scholarship was kept (T18).
	var sums []chargePaid
	if err := tx.Raw(`SELECT p.charge_id,
	                         COALESCE(SUM(p.amount_cents), 0) AS paid,
	                         COALESCE(SUM(p.amount_cents) FILTER (
	                             WHERE p.paid_on <= c.due_date), 0) AS paid_by_due
	                    FROM payments p
	                    JOIN charges c ON c.id = p.charge_id
	                   WHERE p.charge_id IN ?
	                   GROUP BY p.charge_id`, ids).
		Scan(&sums).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "summing payments", err)
	}
	paidByCharge := make(map[string]chargePaid, len(sums))
	for _, s := range sums {
		paidByCharge[s.ChargeID] = s
	}

	out := make([]ports.ChargeWithPaid, 0, len(rows))
	for _, m := range rows {
		sum := paidByCharge[m.ID]
		charge, err := toDomainCharge(m, sum.Paid)
		if err != nil {
			return nil, err
		}
		out = append(out, ports.ChargeWithPaid{Charge: charge, PaidCents: sum.Paid, PaidByDueCents: sum.PaidByDue})
	}
	return out, nil
}

// folioUniqueConstraint is deposits' UNIQUE (tenant_id, folio) (migration 0018).
const folioUniqueConstraint = "deposits_folio_unique"

// violates reports whether err is a unique violation of the named constraint.
func violates(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation, which the ledger surfaces as KindConflict.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// translateFindErr maps a GORM not-found to the shared sentinel; anything else
// is an internal error.
func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}

// --- tuition (M15) ----------------------------------------------------------

// EnsureConceptOfKind returns the tenant's concept for a system kind, creating it
// with the given name if the school has none.
//
// The lookup is by KIND, never by name. Names are the school's to change, and a
// monthly billing run that found its concept by name would stop finding it the
// day somebody renamed "Colegiatura" to "Mensualidad" — silently, and only
// noticeably a month later.
func (r *Repository) EnsureConceptOfKind(ctx context.Context, id, kind, name string) (domain.ChargeConcept, error) {
	var out domain.ChargeConcept
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var existing chargeConceptModel
		err := tx.Where("kind = ?", kind).Take(&existing).Error
		if err == nil {
			stored, convErr := toDomainConcept(existing)
			if convErr != nil {
				return convErr
			}
			out = stored
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return glerrors.Wrap(glerrors.KindInternal, "reading the charge concept", err)
		}

		concept, err := domain.NewChargeConcept(id, r.tenantID, name, 0, recurrenceFor(kind), true)
		if err != nil {
			return err
		}
		concept, err = concept.WithKind(kind)
		if err != nil {
			return err
		}
		model := fromDomainConcept(concept)
		if err := tx.Create(&model).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "creating the charge concept", err)
		}
		out = concept
		return nil
	})
	if err != nil {
		return domain.ChargeConcept{}, err
	}
	return out, nil
}

// recurrenceFor is the recurrence a system concept is born with. Only tuition
// repeats; an inscription happens once a year and a surcharge is a one-off event
// that merely happens to be able to recur.
func recurrenceFor(kind string) string {
	if kind == domain.KindTuition {
		return "monthly"
	}
	return "once"
}

// UpsertTuitionPlan stores the tuition table for one (cycle, level), replacing
// whatever was there.
//
// Replacing rather than versioning: the table states the CURRENT price, and every
// charge already issued carries its own amount, so correcting a typo cannot
// disturb what a family already owes.
func (r *Repository) UpsertTuitionPlan(ctx context.Context, p domain.TuitionPlan) (domain.TuitionPlan, error) {
	model, err := fromDomainPlan(p)
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	var out domain.TuitionPlan
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "cycle_id"}, {Name: "level_id"}},
			// EVERY mutable column. A list that misses one silently keeps the
			// old value on an update while an insert stores the new one, so the
			// same request does two different things depending on whether the
			// level had been priced before — which is exactly how the daily
			// surcharge and the split schedule looked saved and were not.
			DoUpdates: clause.AssignmentColumns([]string{
				"enrollment_cents", "monthly_cents", "months",
				"first_billed_month", "due_day", "late_fee_cents", "active", "updated_at",
				"late_fee_daily_cents", "late_fee_mode", "surcharge_counts_all_days",
				"early_payment_cents", "early_payment_days", "schedule",
				"monthly_discount_cents", "fees", "enrollment_due_on",
				"enrollment_starts_on", "enrollment_concepts",
			}),
		}).Create(&model).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing the tuition table", err)
		}
		// Read back: on a conflict the stored id is the ORIGINAL row's, not the
		// one just minted, and returning the minted id would hand the caller an
		// identifier that matches nothing.
		var stored tuitionPlanModel
		if err := tx.Where("cycle_id = ? AND level_id = ?", p.CycleID(), p.LevelID()).
			Take(&stored).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading back the tuition table", err)
		}
		plan, err := toDomainPlan(stored)
		if err != nil {
			return err
		}
		out = plan
		return nil
	})
	if err != nil {
		return domain.TuitionPlan{}, err
	}
	return out, nil
}

// ListTuitionPlans returns the whole tuition table of a cycle.
func (r *Repository) ListTuitionPlans(ctx context.Context, cycleID string) ([]domain.TuitionPlan, error) {
	var rows []tuitionPlanModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("cycle_id = ?", cycleID).Order("created_at").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the tuition tables", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.TuitionPlan, 0, len(rows))
	for _, row := range rows {
		plan, err := toDomainPlan(row)
		if err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	return out, nil
}

// --- deposits (M16) --------------------------------------------------------

// RegisterDeposit stores money as it arrived and the payments it splits into, in
// ONE transaction.
//
// The order of operations is the whole design. The student's charges are locked
// FOR UPDATE first, ordered by due date so two concurrent deposits for the same
// family take the rows in the same order and cannot deadlock against each other.
// Their collected totals are summed under that lock, and only then is the
// caller's plan run: the split is decided against a balance that cannot move
// under it. Planning outside and writing inside would reopen exactly the race
// the lock exists to close.
//
// A repeat under the same idempotency key returns what was stored the first time
// and collects nothing.
func (r *Repository) RegisterDeposit(
	ctx context.Context,
	d domain.Deposit,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
	outbox func(record ports.DepositRecord) []ports.OutboxEvent,
) (ports.DepositRecord, bool, error) {
	var (
		record   ports.DepositRecord
		replayed bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		original, found, err := depositByIdempotencyKey(tx, d.IdempotencyKey())
		if err != nil {
			return err
		}
		if found {
			stored, rerr := rebuildDepositRecord(tx, original)
			if rerr != nil {
				return rerr
			}
			record = stored
			replayed = true
			return nil
		}

		stored, err := r.storeDeposit(tx, d, plan)
		if err != nil {
			return err
		}
		record = stored
		if outbox != nil {
			if err := writeOutbox(tx, r.tenantID, outbox(record)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ports.DepositRecord{}, false, err
	}
	return record, replayed, nil
}

// storeDeposit writes a new deposit and what it splits into, inside the
// caller's transaction: the student's charges are locked FOR UPDATE first (in
// due-date order, so concurrent deposits cannot deadlock), the plan runs
// against that locked balance, the folio is allocated, and the re-prices,
// discounts, surcharges and allocations are written in that order. Shared by
// RegisterDeposit and CorrectDeposit (T9), so a correction is split by
// exactly the path a deposit is.
func (r *Repository) storeDeposit(
	tx *gorm.DB,
	d domain.Deposit,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
) (ports.DepositRecord, error) {
	open, err := lockedChargesOf(tx, d.StudentID())
	if err != nil {
		return ports.DepositRecord{}, err
	}
	write, err := plan(open)
	if err != nil {
		return ports.DepositRecord{}, err
	}

	// Refused before a folio is spent on it: the folio's year is its day's.
	if d.PaidOn().IsZero() {
		return ports.DepositRecord{}, errUndated("deposit", d.ID())
	}
	folio, capturedAt, err := r.nextDepositFolio(tx, d.PaidOn().Year())
	if err != nil {
		return ports.DepositRecord{}, err
	}
	d = d.WithFolio(folio)
	row, err := fromDomainDeposit(d)
	if err != nil {
		return ports.DepositRecord{}, err
	}
	// One clock for the folio's year and the capture it names.
	row.CreatedAt = capturedAt
	if err := tx.Create(&row).Error; err != nil {
		if violates(err, folioUniqueConstraint) {
			// The counter is behind the stored folios (a restore, a hand
			// edit): a fault to fix, never "this payment already exists".
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "the deposit folio counter is behind its folios", err)
		}
		if isUniqueViolation(err) {
			// A concurrent request carrying the same key got there first.
			// It is a retry, not a second collection.
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindConflict,
				"deposit already registered under this idempotency key", err)
		}
		return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "storing deposit", err)
	}
	tenders := fromDomainTenders(d)
	if err := tx.Create(&tenders).Error; err != nil {
		return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "storing the deposit's tenders", err)
	}

	// The discounts come first: the allocations that follow assume the new,
	// lower amount, and a charge saved after them would be a row whose
	// payments were computed against a price it no longer has.
	// The re-prices come first of all: the discounts and the allocations that
	// follow are both computed against the new amount, and a price written
	// after them would be a row whose payments answer to a number it no
	// longer has. This write is also what FREEZES the ladder — the money
	// landing below is the first peso (ADR 0011).
	for _, c := range write.Repriced {
		if err := tx.Model(&chargeModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"amount_cents": c.Amount().Int64(),
			"updated_at":   gorm.Expr("now()"),
		}).Error; err != nil {
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "re-pricing an inscription", err)
		}
	}

	for _, c := range write.Discounted {
		if err := tx.Model(&chargeModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"discount_cents":  c.Discount().Int64(),
			"discount_reason": c.DiscountReason(),
			"updated_at":      gorm.Expr("now()"),
		}).Error; err != nil {
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "applying discount", err)
		}
	}

	// The scholarship's part of each charge this money settles under one
	// (T18): written now, so a later change to the scholarship never rewrites a
	// paid charge. After the discounts, before the allocations it explains.
	for _, c := range write.Scholarships {
		if err := tx.Model(&chargeModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"scholarship_cents": c.Scholarship().Int64(),
			"scholarship_id":    optionalString(c.ScholarshipID()),
			"updated_at":        gorm.Expr("now()"),
		}).Error; err != nil {
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "writing a scholarship", err)
		}
	}

	for _, fee := range write.Surcharges {
		model := fromDomainCharge(fee)
		if err := tx.Create(&model).Error; err != nil {
			return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "storing surcharge", err)
		}
	}

	settled, err := applyAllocations(tx, d, write.Allocations,
		replaceCharges(replaceCharges(replaceCharges(open, write.Repriced), write.Discounted), write.Scholarships), write.Surcharges)
	if err != nil {
		return ports.DepositRecord{}, err
	}

	return ports.DepositRecord{
		Deposit:          d,
		Surcharges:       write.Surcharges,
		Allocations:      write.Allocations,
		UnallocatedCents: write.UnallocatedCents,
		Settled:          settled,
	}, nil
}

// applyAllocations writes the payments a deposit split into and re-derives the
// status of every charge they touched. It returns the charges this deposit
// CLOSED, and only those: a consumer that releases goods on ChargeSettled must
// never act on a partial payment.
func applyAllocations(
	tx *gorm.DB,
	d domain.Deposit,
	allocations []domain.Payment,
	open []ports.ChargeWithPaid,
	fresh []domain.Charge,
) ([]ports.ChargeWithPaid, error) {
	known := make(map[string]domain.Charge, len(open)+len(fresh))
	paid := make(map[string]int64, len(open))
	for _, c := range open {
		known[c.Charge.ID()] = c.Charge
		paid[c.Charge.ID()] = c.PaidCents
	}
	for _, c := range fresh {
		known[c.ID()] = c
	}

	var settled []ports.ChargeWithPaid
	for _, p := range allocations {
		charge, ok := known[p.ChargeID()]
		if !ok {
			// The planner may only allocate to charges it was handed or created.
			return nil, glerrors.New(glerrors.KindInternal,
				"deposit allocated to a charge that is not in the transaction")
		}
		model, err := fromDomainPayment(p)
		if err != nil {
			return nil, err
		}
		depositID := d.ID()
		model.DepositID = &depositID
		if err := tx.Create(&model).Error; err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "storing allocation", err)
		}

		newPaid := paid[p.ChargeID()] + p.Amount().Int64()
		paid[p.ChargeID()] = newPaid
		if newPaid > charge.Effective().Int64() {
			// The plan ran under the lock, so this can only mean the planner
			// itself overspent. Abort rather than store money against a charge
			// that did not owe it.
			return nil, glerrors.New(glerrors.KindInternal,
				"deposit allocated more than a charge owed")
		}
		status := charge.DeriveStatus(newPaid)
		if err := tx.Model(&chargeModel{}).Where("id = ?", p.ChargeID()).Updates(map[string]any{
			"status":     status,
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "updating charge status", err)
		}
		if status == domain.StatusPaid {
			settled = append(settled, ports.ChargeWithPaid{
				Charge: charge.WithStatus(newPaid), PaidCents: newPaid,
			})
		}
	}
	return settled, nil
}

// replaceCharges swaps in the discounted versions, so the allocation veto
// measures each payment against the amount the family was actually charged
// rather than the one read a moment before the discount was granted.
func replaceCharges(open []ports.ChargeWithPaid, discounted []domain.Charge) []ports.ChargeWithPaid {
	if len(discounted) == 0 {
		return open
	}
	byID := make(map[string]domain.Charge, len(discounted))
	for _, c := range discounted {
		byID[c.ID()] = c
	}
	out := make([]ports.ChargeWithPaid, 0, len(open))
	for _, c := range open {
		if replacement, ok := byID[c.Charge.ID()]; ok {
			c.Charge = replacement
		}
		out = append(out, c)
	}
	return out
}

// lockedChargesOf takes every live charge of a student FOR UPDATE and pairs each
// with what has been collected against it. Ordered by (due_date, id) so
// concurrent deposits for the same family lock the rows in the same order and
// cannot deadlock against one another.
func lockedChargesOf(tx *gorm.DB, studentID string) ([]ports.ChargeWithPaid, error) {
	var rows []chargeModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("student_id = ? AND status <> ?", studentID, domain.StatusCancelled).
		Order("due_date ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "locking student charges", err)
	}
	return withPaidTotals(tx, rows)
}

// cancelUnpaidSurchargesOf cancels the daily surcharges a voided deposit wrote
// ("dailyfee:<month>:<deposit>") that nothing else has paid. They exist only
// because the deposit settled their month (task 9); left standing, a void and a
// re-key dated earlier would keep the days of the wrong date, since a row
// already written is subtracted from what the month earns, never shrunk.
func cancelUnpaidSurchargesOf(tx *gorm.DB, studentID, depositID string) error {
	var rows []chargeModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		// The student narrows it to an index: the ref's leading wildcard cannot.
		Where("student_id = ? AND external_ref LIKE ? AND status <> ?", studentID, domain.DailyFeeRefPrefix+"%:"+depositID, domain.StatusCancelled).
		Find(&rows).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "listing the deposit's surcharges", err)
	}
	for _, row := range rows {
		paid, err := paidFor(tx, row.ID)
		if err != nil {
			return err
		}
		if paid > 0 {
			// Another deposit paid it: it is that deposit's debt now.
			continue
		}
		charge, err := toDomainCharge(row, paid)
		if err != nil {
			return err
		}
		cancelled, err := charge.CancelIfUnpaid(paid)
		if err != nil {
			return err
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", row.ID).Updates(map[string]any{
			"status":     cancelled.Status(),
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "cancelling the deposit's surcharge", err)
		}
	}
	return nil
}

// nextDepositFolio allocates the school's next receipt number for year, under
// the counter row's own lock (migration 0018), and returns this transaction's
// instant for created_at. The year is the school's year of the deposit's day
// (paid_on, D8), the one the school clock dated it with: the database's now()
// in UTC would number a payment taken at 19:00 in Morelia on 31 December in
// the next year's run. Per school: a SEQUENCE would share one run.
func (r *Repository) nextDepositFolio(tx *gorm.DB, year int) (string, time.Time, error) {
	var got struct {
		Year int
		Seq  int
		At   time.Time
	}
	err := tx.Raw(`INSERT INTO deposit_folio_counters (tenant_id, year, next_seq)
	               VALUES (?, ?, 2)
	               ON CONFLICT (tenant_id, year)
	               DO UPDATE SET next_seq = deposit_folio_counters.next_seq + 1
	               RETURNING year, next_seq - 1 AS seq, now() AS at`, r.tenantID, year).Scan(&got).Error
	if err != nil {
		return "", time.Time{}, glerrors.Wrap(glerrors.KindInternal, "allocating a deposit folio", err)
	}
	return domain.FormatDepositFolio(got.Year, got.Seq), got.At, nil
}

// depositByIdempotencyKey finds a deposit already stored under key. An empty key
// never matches: it means the caller opted out of deduplication.
func depositByIdempotencyKey(tx *gorm.DB, key string) (domain.Deposit, bool, error) {
	if key == "" {
		return domain.Deposit{}, false, nil
	}
	var row depositModel
	err := tx.Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Deposit{}, false, nil
	}
	if err != nil {
		return domain.Deposit{}, false, glerrors.Wrap(glerrors.KindInternal, "looking up deposit", err)
	}
	ds, derr := toDomainDeposits(tx, []depositModel{row})
	if derr != nil {
		return domain.Deposit{}, false, derr
	}
	return ds[0], true, nil
}

// toDomainDeposits rebuilds stored deposits with their tenders, read in ONE
// query for the whole page (no read per deposit).
func toDomainDeposits(tx *gorm.DB, rows []depositModel) ([]domain.Deposit, error) {
	if len(rows) == 0 {
		return []domain.Deposit{}, nil
	}
	ids := make([]string, 0, len(rows))
	for _, m := range rows {
		ids = append(ids, m.ID)
	}
	var tenderRows []depositTenderModel
	if err := tx.Where("deposit_id IN ?", ids).Order("deposit_id, position").Find(&tenderRows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the deposits' tenders", err)
	}
	byDeposit := make(map[string][]depositTenderModel, len(rows))
	for _, t := range tenderRows {
		byDeposit[t.DepositID] = append(byDeposit[t.DepositID], t)
	}
	folios, err := linkedFolios(tx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Deposit, 0, len(rows))
	for _, m := range rows {
		d, err := toDomainDeposit(m, byDeposit[m.ID], folios)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// linkedFolios reads, in ONE query, the folios of the deposits a page of
// deposits is linked to by a correction (T9): the one each corrects and the
// one that replaced it. Most pages link to nothing and make no query.
func linkedFolios(tx *gorm.DB, rows []depositModel) (map[string]string, error) {
	var ids []string
	for _, m := range rows {
		if m.CorrectsDepositID != nil {
			ids = append(ids, *m.CorrectsDepositID)
		}
		if m.SupersededBy != nil {
			ids = append(ids, *m.SupersededBy)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var linked []struct {
		ID    string
		Folio string
	}
	if err := tx.Model(&depositModel{}).Select("id, folio").Where("id IN ?", ids).Scan(&linked).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the corrected deposits' folios", err)
	}
	out := make(map[string]string, len(linked))
	for _, l := range linked {
		out[l.ID] = l.Folio
	}
	return out, nil
}

// rebuildDepositRecord reconstructs what a stored deposit did, so a replay
// returns the same answer as the call that collected the money.
func rebuildDepositRecord(tx *gorm.DB, d domain.Deposit) (ports.DepositRecord, error) {
	var rows []paymentModel
	if err := tx.Where("deposit_id = ?", d.ID()).Order("id ASC").Find(&rows).Error; err != nil {
		return ports.DepositRecord{}, glerrors.Wrap(glerrors.KindInternal, "listing allocations", err)
	}
	allocations := make([]domain.Payment, 0, len(rows))
	var allocated int64
	for _, m := range rows {
		p, err := toDomainPayment(m)
		if err != nil {
			return ports.DepositRecord{}, err
		}
		allocations = append(allocations, p)
		allocated += p.Amount().Int64()
	}
	return ports.DepositRecord{
		Deposit:          d,
		Allocations:      allocations,
		UnallocatedCents: d.Amount().Int64() - allocated,
	}, nil
}

// DepositReceipt reads a deposit and what it paid, concept by concept (task
// 12). One join: each payment of the deposit with its charge and the charge's
// concept, whose kind says a hand-typed late fee; the ref says the daily and
// monthly ones. A voided deposit has no payments left, so no lines.
func (r *Repository) DepositReceipt(ctx context.Context, id string) (ports.DepositReceipt, error) {
	var out ports.DepositReceipt
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m depositModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		ds, err := toDomainDeposits(tx, []depositModel{m})
		if err != nil {
			return err
		}
		d := ds[0]
		var rows []struct {
			ChargeID      string
			Description   string
			AmountCents   int64
			ExternalRef   string
			Kind          string
			DiscountCents int64
			// The scholarship's part (T18), on the same settling payment.
			ScholarshipCents int64
			ScholarshipName  string
		}
		// A charge's discount is realized by the payment that settles it: only
		// the last payment captured on a paid charge shows it, so a charge paid
		// in two deposits does not read its discount twice.
		if err := tx.Raw(`SELECT p.charge_id, c.description, p.amount_cents, c.external_ref, cc.kind,
		                         CASE WHEN settles THEN c.discount_cents ELSE 0 END AS discount_cents,
		                         CASE WHEN settles THEN c.scholarship_cents ELSE 0 END AS scholarship_cents,
		                         CASE WHEN settles THEN COALESCE(s.name, '') ELSE '' END AS scholarship_name
		                    FROM payments p
		                    JOIN charges c ON c.id = p.charge_id
		                    JOIN charge_concepts cc ON cc.id = c.concept_id
		                    LEFT JOIN scholarships s ON s.id = c.scholarship_id
		                    CROSS JOIN LATERAL (SELECT c.status = ? AND p.id = (
		                                  SELECT p2.id FROM payments p2
		                                   WHERE p2.charge_id = c.id
		                                   ORDER BY p2.created_at DESC, p2.id DESC LIMIT 1) AS settles) last
		                   WHERE p.deposit_id = ?
		                   ORDER BY c.due_date, p.id`, domain.StatusPaid, id).Scan(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the deposit's receipt", err)
		}
		out = ports.DepositReceipt{Deposit: d, Lines: make([]ports.ReceiptLine, 0, len(rows))}
		var allocated int64
		// A charge paid by two tenders (T8) is two payment rows and ONE concept
		// on the receipt: the rows of a charge are adjacent (ordered by due
		// date), so they fold into the line before. The discount rides on one
		// of them at most.
		at := map[string]int{}
		for _, row := range rows {
			allocated += row.AmountCents
			if i, seen := at[row.ChargeID]; seen {
				out.Lines[i].AmountCents += row.AmountCents
				out.Lines[i].DiscountCents = max(out.Lines[i].DiscountCents, row.DiscountCents)
				if row.ScholarshipCents > out.Lines[i].ScholarshipCents {
					out.Lines[i].ScholarshipCents, out.Lines[i].ScholarshipName = row.ScholarshipCents, row.ScholarshipName
				}
				continue
			}
			at[row.ChargeID] = len(out.Lines)
			out.Lines = append(out.Lines, ports.ReceiptLine{
				ChargeID: row.ChargeID, Description: row.Description, AmountCents: row.AmountCents,
				Surcharge:        row.Kind == domain.KindLateFee || domain.IsFeeRef(row.ExternalRef),
				DiscountCents:    row.DiscountCents,
				ScholarshipCents: row.ScholarshipCents,
				ScholarshipName:  row.ScholarshipName,
			})
		}
		// A voided or superseded deposit's money is gone or moved: no credit.
		if d.Counts() {
			out.UnallocatedCents = d.Amount().Int64() - allocated
		}
		return nil
	})
	if err != nil {
		return ports.DepositReceipt{}, err
	}
	return out, nil
}

// UnallocatedCents is what a family has paid that no charge has claimed.
//
// Two sums rather than a join per deposit: the deposits they still have
// standing, and what those deposits actually landed on. The difference is the
// credit. A cancelled or superseded deposit (T9) is excluded from both sides —
// releasing it already deleted its allocations, so counting its amount would
// invent money.
func (r *Repository) UnallocatedCents(ctx context.Context, studentID string) (int64, error) {
	var credit int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var deposited int64
		if err := tx.Model(&depositModel{}).
			Where("student_id = ? AND voided_at IS NULL AND superseded_at IS NULL", studentID).
			Select("COALESCE(SUM(amount_cents), 0)").Scan(&deposited).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "summing deposits", err)
		}
		var allocated int64
		if err := tx.Model(&paymentModel{}).
			Where("deposit_id IN (?)", tx.Model(&depositModel{}).
				Select("id").Where("student_id = ? AND voided_at IS NULL AND superseded_at IS NULL", studentID)).
			Select("COALESCE(SUM(amount_cents), 0)").Scan(&allocated).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "summing allocations", err)
		}
		credit = deposited - allocated
		return nil
	})
	if err != nil {
		return 0, err
	}
	return credit, nil
}

// IncomeLines sums collected money per concept over a window, in ONE grouped
// query rather than a read per concept.
//
// The window is on payments.paid_on — the school-local day the money changed
// hands (ADR 0035), not the UTC date of paid_at. Grouping
// on the charge's due date instead would credit October with September's
// arrears, which is precisely the month a school most wants to read honestly.
//
// Payments against a cancelled charge are excluded. That money is a correction
// to make, not income to report.
//
// Rows paid by a credit note come back as their own lines, CreditNote set.
func (r *Repository) IncomeLines(ctx context.Context, from, to time.Time) ([]ports.IncomeLine, error) {
	start := domain.CalendarDay(from).Format(dateLayout)
	end := domain.CalendarDay(to).AddDate(0, 0, 1).Format(dateLayout)

	var rows []ports.IncomeLine
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Table("payments p").
			Select("c.concept_id AS concept_id, k.name AS concept, COALESCE(SUM(p.amount_cents), 0) AS amount_cents, "+
				"p.method = ? AS credit_note", domain.MethodCreditNote).
			Joins("JOIN charges c ON c.id = p.charge_id").
			Joins("JOIN charge_concepts k ON k.id = c.concept_id").
			Where("p.paid_on >= ? AND p.paid_on < ? AND c.status <> ?", start, end, domain.StatusCancelled).
			// What a credit note settled is grouped apart (T8): it is not money,
			// and the use case reports it on its own line.
			Group("c.concept_id, k.name, credit_note").
			Scan(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "summing income by concept", err)
	}
	return rows, nil
}

// GetDeposit returns one deposit by id (KindNotFound if absent or hidden by RLS).
func (r *Repository) GetDeposit(ctx context.Context, id string) (domain.Deposit, error) {
	var out domain.Deposit
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m depositModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		ds, derr := toDomainDeposits(tx, []depositModel{m})
		if derr != nil {
			return derr
		}
		out = ds[0]
		return nil
	})
	if err != nil {
		return domain.Deposit{}, err
	}
	return out, nil
}

// ListDeposits returns a student's deposits, newest first. An empty studentID
// returns the tenant's deposits, which is what the daily cash-up reads.
func (r *Repository) ListDeposits(ctx context.Context, studentID string) ([]domain.Deposit, error) {
	var out []domain.Deposit
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// By day first: a live payment's instant can sort after a later
		// counter-picked day's midnight UTC.
		q := tx.Model(&depositModel{}).Order("paid_on DESC, paid_at DESC, id DESC").Limit(maxListLimit)
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		var rows []depositModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing deposits", err)
		}
		var derr error
		out, derr = toDomainDeposits(tx, rows)
		return derr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListDepositsOn returns every deposit taken on one calendar day, cancelled
// ones included. The day is paid_on — the school-local day the money changed
// hands (ADR 0035), not the day it was keyed nor the UTC date of paid_at.
func (r *Repository) ListDepositsOn(ctx context.Context, day time.Time) ([]domain.Deposit, error) {
	on := domain.CalendarDay(day).Format(dateLayout)
	var out []domain.Deposit
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []depositModel
		if err := tx.Where("paid_on = ?", on).
			Order("paid_at ASC, id ASC").Limit(maxListLimit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the day's deposits", err)
		}
		var derr error
		out, derr = toDomainDeposits(tx, rows)
		return derr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListLoosePaymentsOn returns the payments taken on one day outside any
// deposit, with the description of the charge each one paid. Same day as
// ListDepositsOn: paid_on.
func (r *Repository) ListLoosePaymentsOn(ctx context.Context, day time.Time) ([]ports.LoosePayment, error) {
	on := domain.CalendarDay(day).Format(dateLayout)
	var out []ports.LoosePayment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []paymentModel
		if err := tx.Where("deposit_id IS NULL AND paid_on = ?", on).
			Order("paid_at ASC, id ASC").Limit(maxListLimit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the day's loose payments", err)
		}
		if len(rows) == 0 {
			return nil
		}
		chargeIDs := make([]string, 0, len(rows))
		for _, m := range rows {
			chargeIDs = append(chargeIDs, m.ChargeID)
		}
		var described []struct {
			ID          string
			Description string
		}
		if err := tx.Model(&chargeModel{}).Select("id, description").Where("id IN ?", chargeIDs).
			Scan(&described).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading what the loose payments paid", err)
		}
		descriptionOf := make(map[string]string, len(described))
		for _, d := range described {
			descriptionOf[d.ID] = d.Description
		}
		out = make([]ports.LoosePayment, 0, len(rows))
		for _, m := range rows {
			p, perr := toDomainPayment(m)
			if perr != nil {
				return perr
			}
			out = append(out, ports.LoosePayment{Payment: p, Description: descriptionOf[m.ChargeID]})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// VoidDeposit cancels a deposit and releases what it settled, in one
// transaction: the allocations are removed and every charge they touched goes
// back to the status its remaining payments imply.
//
// The deposit row survives with its reason. A school that deletes money rows
// cannot explain its own ledger, and the day's takings still have to add up when
// a cashier keyed $67,000 instead of $6,700.
func (r *Repository) VoidDeposit(ctx context.Context, d domain.Deposit) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		// The deposit row first, as CorrectDeposit takes it: a void and a
		// correction of the same receipt queue on it instead of deadlocking
		// over its payments and charges.
		if _, err := lockedDeposit(tx, d.ID()); err != nil {
			return err
		}
		if _, err := releaseDeposit(tx, d, false); err != nil {
			return err
		}
		res := tx.Model(&depositModel{}).
			Where("id = ? AND voided_at IS NULL AND superseded_at IS NULL", d.ID()).
			Updates(map[string]any{
				"voided_at":   d.VoidedAt(),
				"void_reason": d.VoidReason(),
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "cancelling deposit", res.Error)
		}
		if res.RowsAffected == 0 {
			// Gone, cancelled by another cashier first, or corrected (T9):
			// each is a conflict, not a silent success.
			return glerrors.New(glerrors.KindConflict, "this deposit is already cancelled or corrected")
		}
		return nil
	})
}

// releaseDeposit removes a deposit's allocations and puts every charge they
// touched back to the status its remaining payments imply — what a void does,
// and what a correction does to the deposit it supersedes (T9). It returns the
// charges that were PAID before the release, so a correction announces as
// settled only what it newly closed.
//
// A price-at-payment-date discount always goes with the deposit (it exists
// only because this deposit settled the line). resetEarned also takes back the
// early-payment reward the RULE gave this deposit for its date — a correction
// re-dates the money and the rule decides again; a reward the school granted
// by hand (ReasonEarlyPaymentGranted) is a person's decision and stays. The
// reward is this deposit's only when it was the last payment on the charge:
// a later deposit that settled the month earned it, not this one.
func releaseDeposit(tx *gorm.DB, d domain.Deposit, resetEarned bool) (map[string]bool, error) {
	var allocations []paymentModel
	if err := tx.Where("deposit_id = ?", d.ID()).Find(&allocations).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing allocations", err)
	}
	touched := make(map[string]bool, len(allocations))
	for _, a := range allocations {
		touched[a.ChargeID] = true
	}
	if err := tx.Where("deposit_id = ?", d.ID()).Delete(&paymentModel{}).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "releasing allocations", err)
	}

	wasPaid := make(map[string]bool, len(touched))
	for chargeID := range touched {
		var row chargeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", chargeID).First(&row).Error; err != nil {
			return nil, translateFindErr(err)
		}
		wasPaid[chargeID] = row.Status == domain.StatusPaid
		remaining, err := paidFor(tx, chargeID)
		if err != nil {
			return nil, err
		}
		updates := map[string]any{"updated_at": gorm.Expr("now()")}
		takeBack, err := discountGoesWith(tx, row, d, resetEarned)
		if err != nil {
			return nil, err
		}
		if takeBack {
			row.DiscountCents, row.DiscountReason = 0, ""
			updates["discount_cents"], updates["discount_reason"] = int64(0), ""
		}
		// A scholarship's part stays only while what is left still settles the
		// charge (T18): released, the rule decides again — on a correction, on
		// the corrected date.
		if row.ScholarshipCents > 0 && remaining < row.AmountCents-row.DiscountCents-row.ScholarshipCents {
			row.ScholarshipCents, row.ScholarshipID = 0, nil
			updates["scholarship_cents"], updates["scholarship_id"] = int64(0), nil
		}
		charge, err := toDomainCharge(row, remaining)
		if err != nil {
			return nil, err
		}
		if charge.Status() == domain.StatusCancelled {
			continue
		}
		updates["status"] = charge.DeriveStatus(remaining)
		if err := tx.Model(&chargeModel{}).Where("id = ?", chargeID).Updates(updates).Error; err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "restoring charge status", err)
		}
	}

	if err := cancelUnpaidSurchargesOf(tx, d.StudentID(), d.ID()); err != nil {
		return nil, err
	}
	return wasPaid, nil
}

// discountGoesWith reports whether a charge's discount was this deposit's to
// take back on release (see releaseDeposit).
func discountGoesWith(tx *gorm.DB, row chargeModel, d domain.Deposit, resetEarned bool) (bool, error) {
	switch {
	case row.DiscountReason == domain.ReasonPriceAtPaymentDate:
		// A price-at-payment-date discount exists only because this deposit
		// settled the line (task 7): it goes with the deposit, or a released
		// deposit would leave the inscription permanently cheaper.
		return true, nil
	case !resetEarned || row.DiscountReason != domain.ReasonEarlyPayment:
		return false, nil
	}
	var later int64
	if err := tx.Model(&paymentModel{}).
		Where("charge_id = ? AND created_at > ?", row.ID, d.CapturedAt()).
		Count(&later).Error; err != nil {
		return false, glerrors.Wrap(glerrors.KindInternal, "reading who settled the charge", err)
	}
	return later == 0, nil
}

// --- corrections (T9) ------------------------------------------------------

// CorrectDeposit replaces a deposit with its correction in ONE transaction.
//
// Order matters as much as in RegisterDeposit. The original is locked first
// (two people correcting the same receipt are serialised; the second finds it
// superseded), then the family's charges in the order every deposit locks
// them, then the original is released, and only then is the corrected deposit
// planned and stored — through storeDeposit, the same path as any deposit — so
// the split, the surcharges and the early-payment reward are worked out on the
// corrected date by the same arithmetic. The original is then marked
// superseded (its row, folio and tenders kept) and the history row written.
func (r *Repository) CorrectDeposit(
	ctx context.Context,
	w ports.DepositCorrectionWrite,
	plan func(open []ports.ChargeWithPaid) (ports.DepositWrite, error),
	outbox func(record ports.CorrectionRecord) []ports.OutboxEvent,
) (ports.CorrectionRecord, bool, error) {
	var (
		record   ports.CorrectionRecord
		replayed bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		prior, found, err := depositByIdempotencyKey(tx, w.Corrected.IdempotencyKey())
		if err != nil {
			return err
		}
		if found {
			if prior.Corrects() != w.OriginalID {
				return glerrors.New(glerrors.KindConflict, "this idempotency key belongs to another payment")
			}
			record, err = storedCorrection(tx, prior)
			replayed = err == nil
			return err
		}

		original, err := lockedDeposit(tx, w.OriginalID)
		if err != nil {
			return err
		}
		if original.StudentID() != w.Corrected.StudentID() {
			return glerrors.New(glerrors.KindInvalid, "a correction pays the same pupil's account")
		}
		superseded, err := original.Supersede(w.Corrected.ID(), w.At)
		if err != nil {
			return err
		}
		if _, err := lockedChargesOf(tx, original.StudentID()); err != nil {
			return err
		}
		before, err := allocationsOf(tx, original.ID())
		if err != nil {
			return err
		}
		wasPaid, err := releaseDeposit(tx, original, true)
		if err != nil {
			return err
		}

		stored, err := r.storeDeposit(tx, w.Corrected.WithCorrects(original.ID(), original.Folio()), plan)
		if err != nil {
			return err
		}
		if err := tx.Model(&depositModel{}).Where("id = ?", original.ID()).Updates(map[string]any{
			"superseded_at": w.At,
			"superseded_by": stored.Deposit.ID(),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "marking the corrected deposit superseded", err)
		}

		correction, err := domain.NewDepositCorrection(w.CorrectionID, r.tenantID, original.StudentID(),
			original.ID(), stored.Deposit.ID(), w.Reason,
			domain.SnapshotOf(original, before), domain.SnapshotOf(stored.Deposit, stored.Allocations), w.By, w.At)
		if err != nil {
			return err
		}
		model, err := fromDomainCorrection(correction)
		if err != nil {
			return err
		}
		if err := tx.Create(&model).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing the correction", err)
		}

		stored.Settled = newlySettled(stored.Settled, wasPaid)
		record = ports.CorrectionRecord{
			Original:   superseded.RehydrateSupersession(w.At, stored.Deposit.ID(), stored.Deposit.Folio()),
			Corrected:  stored,
			Correction: correction,
		}
		if outbox != nil {
			return writeOutbox(tx, r.tenantID, outbox(record))
		}
		return nil
	})
	if err != nil {
		return ports.CorrectionRecord{}, false, err
	}
	return record, replayed, nil
}

// lockedDeposit reads one deposit FOR UPDATE.
func lockedDeposit(tx *gorm.DB, id string) (domain.Deposit, error) {
	var row depositModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&row).Error; err != nil {
		return domain.Deposit{}, translateFindErr(err)
	}
	ds, err := toDomainDeposits(tx, []depositModel{row})
	if err != nil {
		return domain.Deposit{}, err
	}
	return ds[0], nil
}

// allocationsOf reads the payments a deposit split into, oldest debt first.
func allocationsOf(tx *gorm.DB, depositID string) ([]domain.Payment, error) {
	var rows []paymentModel
	if err := tx.Table("payments p").Select("p.*").
		Joins("JOIN charges c ON c.id = p.charge_id").
		Where("p.deposit_id = ?", depositID).
		Order("c.due_date, p.id").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the deposit's allocations", err)
	}
	out := make([]domain.Payment, 0, len(rows))
	for _, m := range rows {
		p, err := toDomainPayment(m)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// newlySettled keeps the charges a correction closed that were not already
// paid before it: a month the original had settled and the correction settles
// again was never open, and announcing it again would read as a second
// settlement.
func newlySettled(settled []ports.ChargeWithPaid, wasPaid map[string]bool) []ports.ChargeWithPaid {
	out := make([]ports.ChargeWithPaid, 0, len(settled))
	for _, c := range settled {
		if !wasPaid[c.Charge.ID()] {
			out = append(out, c)
		}
	}
	return out
}

// storedCorrection rebuilds a correction already stored, for a replay.
func storedCorrection(tx *gorm.DB, corrected domain.Deposit) (ports.CorrectionRecord, error) {
	var row depositCorrectionModel
	if err := tx.Where("corrected_deposit_id = ?", corrected.ID()).First(&row).Error; err != nil {
		return ports.CorrectionRecord{}, translateFindErr(err)
	}
	correction, err := toDomainCorrection(row)
	if err != nil {
		return ports.CorrectionRecord{}, err
	}
	var originalRow depositModel
	if err := tx.Where("id = ?", correction.OriginalID()).First(&originalRow).Error; err != nil {
		return ports.CorrectionRecord{}, translateFindErr(err)
	}
	originals, err := toDomainDeposits(tx, []depositModel{originalRow})
	if err != nil {
		return ports.CorrectionRecord{}, err
	}
	stored, err := rebuildDepositRecord(tx, corrected)
	if err != nil {
		return ports.CorrectionRecord{}, err
	}
	return ports.CorrectionRecord{Original: originals[0], Corrected: stored, Correction: correction}, nil
}

// ListDepositCorrections returns a student's corrections, oldest first
// (served by idx_deposit_corrections_student). Capped at maxListLimit, the
// NEWEST kept: past the cap it is the oldest history that drops, never the
// corrections that stand now.
func (r *Repository) ListDepositCorrections(ctx context.Context, studentID string) ([]domain.DepositCorrection, error) {
	var out []domain.DepositCorrection
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []depositCorrectionModel
		if err := tx.Where("student_id = ?", studentID).Order("corrected_at DESC, id DESC").
			Limit(maxListLimit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing corrections", err)
		}
		out = make([]domain.DepositCorrection, 0, len(rows))
		for i := len(rows) - 1; i >= 0; i-- {
			c, err := toDomainCorrection(rows[i])
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- outbox (M16) ----------------------------------------------------------

// writeOutbox queues events inside the caller's transaction. It is unexported
// and takes a tx on purpose: an outbox row written in its own transaction would
// be exactly the thing this exists to prevent.
func writeOutbox(tx *gorm.DB, tenantID string, events []ports.OutboxEvent) error {
	for _, e := range events {
		row := outboxModel{TenantID: tenantID, EventType: e.Type, Payload: e.Payload}
		if err := tx.Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "queueing event", err)
		}
	}
	return nil
}

// DrainOutbox publishes what is waiting, oldest first.
//
// Each event is taken under SKIP LOCKED, so two instances draining at the same
// moment never publish the same event twice — and neither waits on the other.
// An event the publisher refuses is left unpublished with its attempt count
// raised: the next drain tries it again, and a count that keeps climbing is the
// signal that the broker is wrong, not the payment.
//
// Publishing happens INSIDE the row's transaction. That is deliberate: it means
// a crash after the send but before the mark redelivers the event rather than
// losing it, and every consumer here is idempotent — comms dedupes on its event
// ledger, students' "mark inscribed" is a no-op when already inscribed. At least
// once is the contract; at most once would be the wrong trade for money.
func (r *Repository) DrainOutbox(ctx context.Context, limit int, publish func(ports.OutboxEvent) error) (int, error) {
	published := 0
	for taken := 0; taken < limit; taken++ {
		var done bool
		err := r.withTenant(ctx, func(tx *gorm.DB) error {
			// Find and not Take: an empty queue is the ORDINARY answer here —
			// the sweep runs every thirty seconds and the payment path drains
			// its own events — and Take reports "record not found" as an error
			// GORM logs at warn level. A line every half minute saying nothing
			// happened is a log nobody reads, which buries the ones that matter.
			var rows []outboxModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("published_at IS NULL").
				Order("created_at, id").
				Limit(1).
				Find(&rows).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "reading the outbox", err)
			}
			if len(rows) == 0 {
				done = true
				return nil
			}
			row := rows[0]

			if perr := publish(ports.OutboxEvent{Type: row.EventType, Payload: row.Payload}); perr != nil {
				// Left unpublished on purpose. Counting the attempt is the only
				// write, so the event survives to be tried again.
				return tx.Model(&outboxModel{}).Where("id = ?", row.ID).
					Update("attempts", row.Attempts+1).Error
			}
			published++
			return tx.Model(&outboxModel{}).Where("id = ?", row.ID).Updates(map[string]any{
				"published_at": gorm.Expr("now()"),
				"attempts":     row.Attempts + 1,
			}).Error
		})
		if err != nil {
			return published, err
		}
		if done {
			break
		}
	}
	return published, nil
}
