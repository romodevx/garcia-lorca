package postgres

import (
	"context"
	"errors"
	"time"

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
	// each capped method says so, as does the proto contract. Explicit pagination
	// is the follow-up that removes the need for it.
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

// CreateChargesBulk issues many charges in ONE statement, skipping the rows
// whose (student, cycle, concept) key already exists instead of failing the
// batch. Postgres reports only the rows actually inserted, so re-running a
// generation is idempotent and the caller can infer the skipped count from
// len(cs) - created.
func (r *Repository) CreateChargesBulk(ctx context.Context, cs []domain.Charge) (int, error) {
	if len(cs) == 0 {
		return 0, nil
	}
	models := make([]chargeModel, 0, len(cs))
	for _, c := range cs {
		models = append(models, fromDomainCharge(c))
	}

	var created int
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// The target is spelled out on purpose. A bare OnConflict{DoNothing}
		// infers its arbiter from whatever unique index happens to match, so a
		// change to the table's keys could stop this deduplicating with no
		// error and re-bill the whole school on the next generation run.
		res := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "student_id"}, {Name: "cycle_id"}, {Name: "concept_id"}},
			// TargetWhere, not Where: this is the predicate of the PARTIAL index
			// being used as the arbiter, not a condition on a DO UPDATE.
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Eq{Column: clause.Column{Name: "external_ref"}, Value: ""}}},
			DoNothing:   true,
		}).Create(&models)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "bulk creating charges", res.Error)
		}
		created = int(res.RowsAffected)
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
		cancelled, err := charge.CancelIfUnpaid(paid)
		if err != nil {
			return err
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", id).Updates(map[string]any{
			"status":     cancelled.Status(),
			"updated_at": gorm.Expr("now()"),
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
// happened to emit. Explicit pagination is a follow-up.
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

// SaveCharge persists the mutable part of an existing charge (its discount and
// derived status), as left by ApplyDiscount or CancelCharge, and returns the
// stored row. An unknown id is glerrors.ErrNotFound.
func (r *Repository) SaveCharge(ctx context.Context, c domain.Charge) (domain.Charge, error) {
	var out domain.Charge
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
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
	p domain.Payment,
	derive func(charge domain.Charge, newPaidCents int64) (string, error),
) (domain.Payment, ports.ChargeWithPaid, bool, error) {
	var (
		storedPayment domain.Payment
		result        ports.ChargeWithPaid
		replayed      bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var row chargeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", p.ChargeID()).First(&row).Error; err != nil {
			return translateFindErr(err)
		}

		currentPaid, err := paidFor(tx, p.ChargeID())
		if err != nil {
			return err
		}
		charge, err := toDomainCharge(row, currentPaid)
		if err != nil {
			return err
		}

		original, found, err := paymentByIdempotencyKey(tx, p.IdempotencyKey())
		if err != nil {
			return err
		}
		if found {
			storedPayment = original
			result = ports.ChargeWithPaid{Charge: charge, PaidCents: currentPaid}
			replayed = true
			return nil
		}

		newPaid := currentPaid + p.Amount().Int64()
		status, err := derive(charge, newPaid)
		if err != nil {
			return err
		}

		payment := fromDomainPayment(p)
		if err := tx.Create(&payment).Error; err != nil {
			// The partial unique index caught a same-key attempt that did not
			// serialize on this charge's lock. It is a retry, not a new payment,
			// so the caller sees a conflict rather than a duplicated collection.
			if isUniqueViolation(err) {
				return glerrors.Wrap(glerrors.KindConflict, "payment already registered under this idempotency key", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "storing payment", err)
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", p.ChargeID()).Updates(map[string]any{
			"status":     status,
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating charge status", err)
		}

		d, err := toDomainPayment(payment)
		if err != nil {
			return err
		}
		storedPayment = d
		// derive has already vetoed cancelled and overpaying charges, so
		// re-deriving from the new total reproduces exactly the status written.
		result = ports.ChargeWithPaid{Charge: charge.WithStatus(newPaid), PaidCents: newPaid}
		return nil
	})
	if err != nil {
		return domain.Payment{}, ports.ChargeWithPaid{}, false, err
	}
	return storedPayment, result, replayed, nil
}

// paymentByIdempotencyKey looks up the payment already posted under key, if any.
// An empty key never matches — it means the caller opted out of deduplication,
// and the partial unique index excludes blank keys for the same reason. The
// query is tenant-bound by RLS, so a key is only ever a duplicate within its own
// school.
func paymentByIdempotencyKey(tx *gorm.DB, key string) (domain.Payment, bool, error) {
	if key == "" {
		return domain.Payment{}, false, nil
	}
	var m paymentModel
	if err := tx.Where("idempotency_key = ?", key).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Payment{}, false, nil
		}
		return domain.Payment{}, false, glerrors.Wrap(glerrors.KindInternal, "looking up payment by idempotency key", err)
	}
	stored, err := toDomainPayment(m)
	if err != nil {
		return domain.Payment{}, false, err
	}
	return stored, true, nil
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
			Order("payments.paid_at ASC").
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

// ListOverdue returns the charges past their due date that still owe money
// (due_date < today, effective - paid > 0, not cancelled). An empty cycleID
// spans every cycle. today is supplied by the caller — never read from a clock
// here — so the query stays deterministic. The remaining balance is computed
// from the same single grouped payment query the list reads (no N+1).
//
// The read examines AT MOST maxListLimit candidates, the earliest due first,
// ordered by (due_date, id) — so the collection queue is bounded and its cut is
// deterministic. Since the still-owing filter is applied afterwards, the result
// holds no more than that many rows and never fewer of the oldest debts.
// Explicit pagination is a follow-up.
func (r *Repository) ListOverdue(ctx context.Context, cycleID string, today time.Time) ([]ports.ChargeWithPaid, error) {
	var out []ports.ChargeWithPaid
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&chargeModel{}).
			Where("due_date < ?::date AND status <> ?", today.Format(dateLayout), domain.StatusCancelled).
			Order("due_date ASC, id ASC").
			Limit(maxListLimit)
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		var rows []chargeModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing overdue charges", err)
		}
		candidates, err := withPaidTotals(tx, rows)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			if c.Charge.Effective().Int64()-c.PaidCents > 0 {
				out = append(out, c)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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
		ChargeID string `gorm:"column:charge_id"`
		Paid     int64  `gorm:"column:paid"`
	}
	var sums []chargePaid
	if err := tx.Model(&paymentModel{}).
		Select("charge_id, COALESCE(SUM(amount_cents), 0) AS paid").
		Where("charge_id IN ?", ids).
		Group("charge_id").
		Scan(&sums).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "summing payments", err)
	}
	paidByCharge := make(map[string]int64, len(sums))
	for _, s := range sums {
		paidByCharge[s.ChargeID] = s.Paid
	}

	out := make([]ports.ChargeWithPaid, 0, len(rows))
	for _, m := range rows {
		paid := paidByCharge[m.ID]
		charge, err := toDomainCharge(m, paid)
		if err != nil {
			return nil, err
		}
		out = append(out, ports.ChargeWithPaid{Charge: charge, PaidCents: paid})
	}
	return out, nil
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
