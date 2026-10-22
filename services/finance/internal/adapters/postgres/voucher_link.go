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
// carry the key derived from its current release count. A payment by hand at a
// voucher-aware counter (case B) takes the pupil's pending vouchers the office
// did not decline (any other payment by hand links nothing): none
// is nothing, one links itself, more than one is ErrVoucherChoiceRequired and
// nothing is taken. Never another pupil's; never matched by amount or name.
func claimVoucher(tx *gorm.DB, d domain.Deposit) (voucherClaim, error) {
	choice := d.VoucherChoice()
	if choice.VoucherID != "" {
		return claimChosenVoucher(tx, d, choice.VoucherID)
	}
	if !choice.Aware {
		// A counter that does not show vouchers cannot decline them: linking
		// (or refusing) on its behalf would block payments it cannot unblock.
		return voucherClaim{}, nil
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

// replayedVoucher completes a replayed deposit with the voucher it paid.
//
// A payment BY voucher is answered as a replay only when the stored deposit is
// that voucher's application: the same pupil, and the voucher still linked to
// it. Anything else under its key is not its replay — a payment by hand that
// took the key, the voided money a release handed back (its next application
// has a key of its own), or the superseded original of a correction. Then the
// voucher's CURRENT state answers: applied to another deposit is
// VOUCHER_ALREADY_APPLIED naming that deposit's folio; otherwise the screen
// read a voucher that has since changed.
func replayedVoucher(tx *gorm.DB, d domain.Deposit, record *ports.DepositRecord) error {
	linked, err := linkedVoucherOf(tx, record.Deposit.ID())
	if err != nil {
		return err
	}
	if chosen := d.VoucherChoice().VoucherID; chosen != "" {
		ownReplay := record.Deposit.StudentID() == d.StudentID() && !record.Deposit.Voided() &&
			linked != nil && linked.ID() == chosen
		if !ownReplay {
			return notTheVouchersReplay(tx, chosen)
		}
	}
	record.LinkedVoucher = linked
	return nil
}

// notTheVouchersReplay is the refusal of a voucher application whose key
// found some other deposit (see replayedVoucher).
func notTheVouchersReplay(tx *gorm.DB, voucherID string) error {
	if !uuidShaped.MatchString(voucherID) {
		return domain.ErrVoucherNotOwned
	}
	v, err := readVoucher(tx, voucherID, false)
	if err != nil {
		return errVoucherChanged
	}
	if v.Status() == domain.VoucherApplied {
		return domain.VoucherAlreadyApplied(v.DepositFolio())
	}
	return errVoucherChanged
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
