package postgres

import (
	"context"
	"database/sql/driver"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Payment vouchers (migration 0032, ADR 0040): what families send from the
// app. Same Repository, same tenant binding, its own port. Linking a voucher
// to money lives with the deposit transactions (voucher_link.go).

var _ ports.PaymentVoucherRepository = (*Repository)(nil)

const (
	// voucherColumns reads a voucher with the folio of the deposit it paid.
	voucherColumns = "v.*, d.folio AS deposit_folio"
	// voucherDepositIndex is the one-voucher-per-deposit invariant.
	voucherDepositIndex = "uq_payment_vouchers_deposit"
	// voucherLockNamespace keys the per-pupil advisory lock of an upload.
	voucherLockNamespace = "finance.payment_vouchers:"
)

// paymentVoucherModel is the GORM row for `payment_vouchers`.
type paymentVoucherModel struct {
	ID                 string     `gorm:"column:id;primaryKey"`
	TenantID           string     `gorm:"column:tenant_id"`
	StudentID          string     `gorm:"column:student_id"`
	UploadedBy         string     `gorm:"column:uploaded_by"`
	UploadedByName     string     `gorm:"column:uploaded_by_name"`
	ClaimedAmountCents *int64     `gorm:"column:claimed_amount_cents"`
	ClaimedPaidOn      *time.Time `gorm:"column:claimed_paid_on"`
	FileKey            string     `gorm:"column:file_key"`
	FileContentType    string     `gorm:"column:file_content_type"`
	FileSizeBytes      int        `gorm:"column:file_size_bytes"`
	FileSHA256         []byte     `gorm:"column:file_sha256"`
	Status             string     `gorm:"column:status"`
	DepositID          *string    `gorm:"column:deposit_id"`
	DecidedBy          *string    `gorm:"column:decided_by"`
	DecidedByName      *string    `gorm:"column:decided_by_name"`
	DecidedAt          *time.Time `gorm:"column:decided_at"`
	RejectionReason    *string    `gorm:"column:rejection_reason"`
	ReleaseCount       int        `gorm:"column:release_count"`
	CreatedAt          time.Time  `gorm:"column:created_at;default:now()"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;default:now()"`
	// DepositFolio is read with the voucher (a join), never written.
	DepositFolio *string `gorm:"column:deposit_folio;->"`
}

func (paymentVoucherModel) TableName() string { return "payment_vouchers" }

// paymentVoucherEventModel is the GORM row for `payment_voucher_events`. The
// id and the instant default in the database when the caller leaves them.
type paymentVoucherEventModel struct {
	ID        string    `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	TenantID  string    `gorm:"column:tenant_id"`
	VoucherID string    `gorm:"column:voucher_id"`
	Kind      string    `gorm:"column:kind"`
	DepositID *string   `gorm:"column:deposit_id"`
	ActorID   *string   `gorm:"column:actor_id"`
	ActorName *string   `gorm:"column:actor_name"`
	Reason    *string   `gorm:"column:reason"`
	At        time.Time `gorm:"column:at;default:clock_timestamp()"`
}

func (paymentVoucherEventModel) TableName() string { return "payment_voucher_events" }

// bytea passes a []byte as ONE parameter: GORM expands a bare slice in a
// condition into one placeholder per byte.
type bytea []byte

// Value implements driver.Valuer.
func (b bytea) Value() (driver.Value, error) { return []byte(b), nil }

// --- mapping ----------------------------------------------------------------

func (r *Repository) fromDomainVoucher(v domain.PaymentVoucher) paymentVoucherModel {
	m := paymentVoucherModel{
		ID: v.ID(), TenantID: r.tenantID, StudentID: v.StudentID(),
		UploadedBy: v.UploadedBy().UserID, UploadedByName: v.UploadedBy().Name,
		FileKey: v.File().Key, FileContentType: v.File().ContentType, FileSizeBytes: v.File().SizeBytes,
		FileSHA256: v.File().SHA256, Status: v.Status(), ReleaseCount: v.Releases(),
	}
	if amount, ok := v.ClaimedAmount(); ok {
		cents := amount.Int64()
		m.ClaimedAmountCents = &cents
	}
	if day, ok := v.ClaimedPaidOn(); ok {
		m.ClaimedPaidOn = &day
	}
	m.DepositID, m.DecidedBy, m.DecidedByName, m.DecidedAt, m.RejectionReason = decisionColumns(v)
	return m
}

// decisionColumns are the voucher's mutable decision columns, NULL where the
// status has none: the shape CHECK of migration 0032 reads them together.
func decisionColumns(v domain.PaymentVoucher) (deposit, by, byName *string, at *time.Time, reason *string) {
	deposit = nullableString(v.DepositID())
	if !v.DecidedAt().IsZero() {
		decidedAt := v.DecidedAt()
		at = &decidedAt
		by = nullableString(v.DecidedBy().UserID)
		name := v.DecidedBy().Name
		byName = &name
	}
	reason = nullableString(v.RejectionReason())
	return deposit, by, byName, at, reason
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toDomainVoucher(m paymentVoucherModel) domain.PaymentVoucher {
	draft := domain.VoucherDraft{StudentID: m.StudentID}
	if m.ClaimedAmountCents != nil {
		draft.ClaimedAmountCents = *m.ClaimedAmountCents
	}
	if m.ClaimedPaidOn != nil {
		draft.ClaimedPaidOn = *m.ClaimedPaidOn
	}
	state := domain.VoucherState{
		Status: m.Status, DepositID: derefString(m.DepositID),
		DecidedBy:       domain.Receiver{UserID: derefString(m.DecidedBy), Name: derefString(m.DecidedByName)},
		RejectionReason: derefString(m.RejectionReason), Releases: m.ReleaseCount,
		DepositFolio: derefString(m.DepositFolio), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.DecidedAt != nil {
		state.DecidedAt = *m.DecidedAt
	}
	file := domain.VoucherFile{Key: m.FileKey, ContentType: m.FileContentType, SizeBytes: m.FileSizeBytes, SHA256: m.FileSHA256}
	return domain.RehydratePaymentVoucher(m.ID, draft, file,
		domain.Receiver{UserID: m.UploadedBy, Name: m.UploadedByName}, state)
}

// --- writes -----------------------------------------------------------------

// CreatePaymentVoucher stores a pending voucher and its uploaded event.
//
// Serialised per pupil by an advisory transaction lock, so the dedupe and the
// pending ceiling are decided against counts that cannot move under them: two
// uploads at once would otherwise both see two pending and both make it four.
func (r *Repository) CreatePaymentVoucher(ctx context.Context, v domain.PaymentVoucher, uploaded domain.VoucherEvent) (domain.PaymentVoucher, bool, error) {
	var (
		out      domain.PaymentVoucher
		replayed bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			voucherLockNamespace+r.tenantID+":"+v.StudentID()).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "locking the pupil's vouchers", err)
		}
		live, found, err := liveVoucherByFile(tx, v.StudentID(), v.File().SHA256)
		if err != nil || found {
			out, replayed = live, found
			return err
		}
		var pending int64
		if err := tx.Model(&paymentVoucherModel{}).
			Where("student_id = ? AND status = ?", v.StudentID(), domain.VoucherPending).
			Count(&pending).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting the pupil's pending vouchers", err)
		}
		if pending >= domain.MaxPendingVouchersPerStudent {
			return domain.ErrVoucherLimit
		}
		m := r.fromDomainVoucher(v)
		if err := tx.Omit("deposit_folio").Create(&m).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing a payment voucher", err)
		}
		if err := r.insertVoucherEvent(tx, uploaded); err != nil {
			return err
		}
		out, err = readVoucher(tx, v.ID(), false)
		return err
	})
	if err != nil {
		return domain.PaymentVoucher{}, false, err
	}
	return out, replayed, nil
}

// UpdatePaymentVoucher runs change on the voucher under its row lock and
// stores the result with its event.
func (r *Repository) UpdatePaymentVoucher(ctx context.Context, id string,
	change func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error),
) (domain.PaymentVoucher, error) {
	if !uuidShaped.MatchString(id) {
		return domain.PaymentVoucher{}, glerrors.ErrNotFound
	}
	var out domain.PaymentVoucher
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		cur, err := readVoucher(tx, id, true)
		if err != nil {
			return err
		}
		next, event, err := change(cur)
		if err != nil {
			return err
		}
		if event.Kind == "" {
			out = cur
			return nil
		}
		if err := r.saveVoucher(tx, next, event); err != nil {
			return err
		}
		out, err = readVoucher(tx, id, false)
		return err
	})
	if err != nil {
		return domain.PaymentVoucher{}, err
	}
	return out, nil
}

// saveVoucher writes EVERY mutable column of a voucher and its event, inside
// the caller's transaction. The one write path of a decision: the update
// endpoint and the deposit transactions (apply, auto-link, release, move) all
// go through it, so no path can forget a column.
func (r *Repository) saveVoucher(tx *gorm.DB, v domain.PaymentVoucher, event domain.VoucherEvent) error {
	deposit, by, byName, at, reason := decisionColumns(v)
	res := tx.Model(&paymentVoucherModel{}).Where("id = ?", v.ID()).Updates(map[string]any{
		"status":           v.Status(),
		"deposit_id":       deposit,
		"decided_by":       by,
		"decided_by_name":  byName,
		"decided_at":       at,
		"rejection_reason": reason,
		"release_count":    v.Releases(),
		"updated_at":       gorm.Expr("now()"),
	})
	if res.Error != nil {
		if violates(res.Error, voucherDepositIndex) {
			return glerrors.Wrap(glerrors.KindConflict, "ese pago ya tiene un comprobante", domain.ErrVoucherAlreadyApplied)
		}
		return glerrors.Wrap(glerrors.KindInternal, "storing a payment voucher's decision", res.Error)
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return r.insertVoucherEvent(tx, event)
}

func (r *Repository) insertVoucherEvent(tx *gorm.DB, e domain.VoucherEvent) error {
	m := paymentVoucherEventModel{
		ID: e.ID, TenantID: r.tenantID, VoucherID: e.VoucherID, Kind: e.Kind,
		DepositID: nullableString(e.DepositID), ActorID: nullableString(e.By.UserID),
		ActorName: nullableString(e.By.Name), Reason: nullableString(e.Reason), At: e.At,
	}
	q := tx
	if m.ID == "" {
		q = q.Omit("id")
	}
	if m.At.IsZero() {
		q = q.Omit("at")
	}
	if err := q.Create(&m).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "recording a payment voucher event", err)
	}
	return nil
}

// --- reads ------------------------------------------------------------------

// GetPaymentVoucher reads one voucher, or KindNotFound.
func (r *Repository) GetPaymentVoucher(ctx context.Context, id string) (domain.PaymentVoucher, error) {
	if !uuidShaped.MatchString(id) {
		return domain.PaymentVoucher{}, glerrors.ErrNotFound
	}
	var out domain.PaymentVoucher
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = readVoucher(tx, id, false)
		return err
	})
	return out, err
}

// LivePaymentVoucherByFile is the pupil's pending or applied voucher with that
// file digest, if any.
func (r *Repository) LivePaymentVoucherByFile(ctx context.Context, studentID string, sha256 []byte) (domain.PaymentVoucher, bool, error) {
	if !uuidShaped.MatchString(studentID) {
		return domain.PaymentVoucher{}, false, nil
	}
	var (
		out   domain.PaymentVoucher
		found bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, found, err = liveVoucherByFile(tx, studentID, sha256)
		return err
	})
	return out, found, err
}

// ListPaymentVouchers reads vouchers newest first, capped: the cap cuts the
// oldest, never today's.
func (r *Repository) ListPaymentVouchers(ctx context.Context, f ports.VoucherFilter) ([]domain.PaymentVoucher, error) {
	students := make([]string, 0, len(f.StudentIDs))
	for _, id := range f.StudentIDs {
		if uuidShaped.MatchString(id) {
			students = append(students, id)
		}
	}
	if len(f.StudentIDs) > 0 && len(students) == 0 {
		// Asked for pupils, none of them well-formed: nothing matches. Never
		// fall through to the unfiltered read.
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 || limit > ports.MaxVoucherListLimit {
		limit = ports.MaxVoucherListLimit
	}
	var rows []paymentVoucherModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := voucherQuery(tx)
		if len(students) > 0 {
			q = q.Where("v.student_id IN ?", students)
		}
		if f.Status != "" {
			q = q.Where("v.status = ?", f.Status)
		}
		if err := q.Order("v.created_at DESC, v.id DESC").Limit(limit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing payment vouchers", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return toDomainVouchers(rows), nil
}

// ListPaymentVoucherEvents reads a voucher's history, oldest first.
func (r *Repository) ListPaymentVoucherEvents(ctx context.Context, id string) ([]domain.VoucherEvent, error) {
	if !uuidShaped.MatchString(id) {
		return nil, nil
	}
	var rows []paymentVoucherEventModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("voucher_id = ?", id).Order("at, id").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing a payment voucher's history", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.VoucherEvent, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.VoucherEvent{
			ID: m.ID, VoucherID: m.VoucherID, Kind: m.Kind, DepositID: derefString(m.DepositID),
			By:     domain.Receiver{UserID: derefString(m.ActorID), Name: derefString(m.ActorName)},
			Reason: derefString(m.Reason), At: m.At,
		})
	}
	return out, nil
}

// voucherQuery selects vouchers with their deposit's folio.
func voucherQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table("payment_vouchers v").Select(voucherColumns).
		Joins("LEFT JOIN deposits d ON d.id = v.deposit_id")
}

// readVoucher reads one voucher inside the caller's transaction; forUpdate
// locks its row (only the voucher's: the joined deposit is not locked).
func readVoucher(tx *gorm.DB, id string, forUpdate bool) (domain.PaymentVoucher, error) {
	q := voucherQuery(tx).Where("v.id = ?", id)
	if forUpdate {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "v"}})
	}
	var m paymentVoucherModel
	if err := q.Take(&m).Error; err != nil {
		return domain.PaymentVoucher{}, translateFindErr(err)
	}
	return toDomainVoucher(m), nil
}

func liveVoucherByFile(tx *gorm.DB, studentID string, sha256 []byte) (domain.PaymentVoucher, bool, error) {
	var rows []paymentVoucherModel
	if err := voucherQuery(tx).
		Where("v.student_id = ? AND v.file_sha256 = ? AND v.status <> ?", studentID, bytea(sha256), domain.VoucherRejected).
		Limit(1).Find(&rows).Error; err != nil {
		return domain.PaymentVoucher{}, false, glerrors.Wrap(glerrors.KindInternal, "looking up a voucher by its file", err)
	}
	if len(rows) == 0 {
		return domain.PaymentVoucher{}, false, nil
	}
	return toDomainVoucher(rows[0]), true, nil
}

func toDomainVouchers(rows []paymentVoucherModel) []domain.PaymentVoucher {
	out := make([]domain.PaymentVoucher, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainVoucher(m))
	}
	return out
}
