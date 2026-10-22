package postgres

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Linking payment vouchers to money (ADR 0040 §4). Everything here runs inside
// a deposit transaction that already holds the pupil's charges FOR UPDATE, and
// locks voucher rows only after them: charges → vouchers, the same order in
// RegisterDeposit, VoidDeposit and CorrectDeposit, so they cannot deadlock.

// errVoucherChanged: the voucher was released (its deposit voided) between the
// screen reading it and this payment; its idempotency key moved on.
var errVoucherChanged = glerrors.New(glerrors.KindConflict, "el comprobante cambió mientras lo revisabas; ábrelo de nuevo")

// voucherClaim is the voucher a new deposit is about to pay, and how it was
// chosen (applied by the office, or auto_linked as the pupil's only pending
// one). The zero claim is "no voucher".
type voucherClaim struct {
	voucher domain.PaymentVoucher
	kind    string
}

// claimVoucher decides which voucher, if any, the deposit d pays, locking it.
//
// A payment by voucher (case A) names it: it must be the pupil's, pending, and
// carry the key derived from its current release count. A payment by hand
// (case B) takes the pupil's pending vouchers the office did not decline: none
// is nothing, one links itself, more than one is ErrVoucherChoiceRequired and
// nothing is taken. Never another pupil's; never matched by amount or name.
func claimVoucher(tx *gorm.DB, d domain.Deposit) (voucherClaim, error) {
	choice := d.VoucherChoice()
	if choice.VoucherID != "" {
		return claimChosenVoucher(tx, d, choice.VoucherID)
	}
	q := voucherQuery(tx).
		Where("v.student_id = ? AND v.status = ?", d.StudentID(), domain.VoucherPending).
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "v"}}).
		Order("v.created_at, v.id")
	if declined := wellFormedIDs(choice.Declined); len(declined) > 0 {
		q = q.Where("v.id NOT IN ?", declined)
	}
	var rows []paymentVoucherModel
	if err := q.Find(&rows).Error; err != nil {
		return voucherClaim{}, glerrors.Wrap(glerrors.KindInternal, "reading the pupil's pending vouchers", err)
	}
	switch len(rows) {
	case 0:
		return voucherClaim{}, nil
	case 1:
		return voucherClaim{voucher: toDomainVoucher(rows[0]), kind: domain.VoucherEventAutoLinked}, nil
	default:
		return voucherClaim{}, domain.ErrVoucherChoiceRequired
	}
}

func claimChosenVoucher(tx *gorm.DB, d domain.Deposit, id string) (voucherClaim, error) {
	if !uuidShaped.MatchString(id) {
		return voucherClaim{}, domain.ErrVoucherNotOwned
	}
	v, err := readVoucher(tx, id, true)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return voucherClaim{}, domain.ErrVoucherNotOwned
	}
	if err != nil {
		return voucherClaim{}, err
	}
	if v.StudentID() != d.StudentID() {
		return voucherClaim{}, domain.ErrVoucherNotOwned
	}
	switch v.Status() {
	case domain.VoucherApplied:
		return voucherClaim{}, domain.VoucherAlreadyApplied(v.DepositFolio())
	case domain.VoucherRejected:
		return voucherClaim{}, domain.ErrVoucherRejected
	}
	if v.IdempotencyKey() != d.IdempotencyKey() {
		return voucherClaim{}, errVoucherChanged
	}
	return voucherClaim{voucher: v, kind: domain.VoucherEventApplied}, nil
}

// linkVoucher applies the claimed voucher to the stored deposit and returns
// it as it now reads (with the deposit's folio).
func (r *Repository) linkVoucher(tx *gorm.DB, claim voucherClaim, deposit domain.Deposit) (*domain.PaymentVoucher, error) {
	if claim.kind == "" {
		return nil, nil
	}
	at := deposit.CapturedAt()
	applied, err := claim.voucher.Apply(deposit.ID(), deposit.ReceivedBy(), at)
	if err != nil {
		return nil, err
	}
	if err := r.saveVoucher(tx, applied, domain.VoucherEvent{
		VoucherID: applied.ID(), Kind: claim.kind, DepositID: deposit.ID(), By: deposit.ReceivedBy(), At: at,
	}); err != nil {
		return nil, err
	}
	return linkedVoucherOf(tx, deposit.ID())
}

// replayedVoucher completes a replayed deposit with the voucher it paid. A
// payment by voucher whose replay is VOIDED money is refused: the voucher was
// handed back by that void and its next application has a key of its own.
func replayedVoucher(tx *gorm.DB, d domain.Deposit, record *ports.DepositRecord) error {
	if d.VoucherChoice().VoucherID != "" && record.Deposit.Voided() {
		return errVoucherChanged
	}
	linked, err := linkedVoucherOf(tx, record.Deposit.ID())
	if err != nil {
		return err
	}
	record.LinkedVoucher = linked
	return nil
}

// linkedVoucherOf is the voucher a deposit paid, or nil.
func linkedVoucherOf(tx *gorm.DB, depositID string) (*domain.PaymentVoucher, error) {
	v, found, err := voucherOfDeposit(tx, depositID, false)
	if err != nil || !found {
		return nil, err
	}
	return &v, nil
}

func voucherOfDeposit(tx *gorm.DB, depositID string, forUpdate bool) (domain.PaymentVoucher, bool, error) {
	q := voucherQuery(tx).Where("v.deposit_id = ?", depositID)
	if forUpdate {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "v"}})
	}
	var m paymentVoucherModel
	err := q.Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.PaymentVoucher{}, false, nil
	}
	if err != nil {
		return domain.PaymentVoucher{}, false, glerrors.Wrap(glerrors.KindInternal, "reading a deposit's voucher", err)
	}
	return toDomainVoucher(m), true, nil
}

// releaseVoucherOf hands a voided deposit's voucher back to the queue.
func (r *Repository) releaseVoucherOf(tx *gorm.DB, voided domain.Deposit) error {
	v, found, err := voucherOfDeposit(tx, voided.ID(), true)
	if err != nil || !found {
		return err
	}
	at := voided.VoidedAt()
	released, err := v.Release(at)
	if err != nil {
		return err
	}
	return r.saveVoucher(tx, released, domain.VoucherEvent{
		VoucherID: v.ID(), Kind: domain.VoucherEventReleasedByVoid, DepositID: voided.ID(),
		Reason: voided.VoidReason(), At: at,
	})
}

// moveVoucher re-points the corrected deposit's voucher at its correction and
// returns it as it now reads, or nil when the original had none.
func (r *Repository) moveVoucher(tx *gorm.DB, originalID string, corrected domain.Deposit, by domain.Receiver, at time.Time) (*domain.PaymentVoucher, error) {
	v, found, err := voucherOfDeposit(tx, originalID, true)
	if err != nil || !found {
		return nil, err
	}
	moved, err := v.MoveTo(corrected.ID(), at)
	if err != nil {
		return nil, err
	}
	if err := r.saveVoucher(tx, moved, domain.VoucherEvent{
		VoucherID: v.ID(), Kind: domain.VoucherEventMovedByCorrection, DepositID: corrected.ID(), By: by, At: at,
	}); err != nil {
		return nil, err
	}
	return linkedVoucherOf(tx, corrected.ID())
}

// wellFormedIDs keeps the uuid-shaped ids: anything else names no row.
func wellFormedIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if uuidShaped.MatchString(id) {
			out = append(out, id)
		}
	}
	return out
}
