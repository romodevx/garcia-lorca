package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// PaymentVoucherService is what families send from the app as proof of a
// transfer, and what the office does with it short of taking the money (ADR
// 0040): the upload, the queue, the file and the refusal. Applying a voucher is
// a deposit — FinanceService.RegisterDeposit with its VoucherID — never here.
type PaymentVoucherService struct {
	repo     ports.PaymentVoucherRepository
	students ports.StudentReader
	receipts ports.ReceiptStore
	newID    func() string
	clock    SchoolClock
}

// NewPaymentVoucherService wires the use case. receipts may be nil: a
// deployment with no bucket takes no vouchers, and says so (NotFound).
func NewPaymentVoucherService(repo ports.PaymentVoucherRepository, students ports.StudentReader,
	receipts ports.ReceiptStore, newID func() string, clock SchoolClock,
) *PaymentVoucherService {
	return &PaymentVoucherService{repo: repo, students: students, receipts: receipts, newID: newID, clock: clock}
}

// VoucherUpload is a voucher as the family sends it.
type VoucherUpload struct {
	StudentID string
	Data      []byte
	// ClaimedAmountCents is what the family says it paid; 0 is "did not say".
	ClaimedAmountCents int64
	// ClaimedPaidOn is the day the family says it paid; zero is "did not say".
	ClaimedPaidOn time.Time
	// By is the guardian, set by the gateway from the session — never a client.
	By domain.Receiver
}

// VoucherScope is whose vouchers a caller may read (ADR 0010 condition 2,
// enforced here and not only at the edge). A family names its children; only
// the school's queue reads everybody's, and it has to say so: an empty list of
// children is refused, never read as "all" — a guardian with no linked pupil
// would otherwise see the whole school's transfers.
type VoucherScope struct {
	StudentIDs []string
	// School is the office's queue: every pupil, StudentIDs then only filter.
	School bool
}

var errVoucherScopeRequired = glerrors.New(glerrors.KindInvalid, "a voucher read must name its pupils or be the school's queue")

func (s VoucherScope) validate() error {
	if !s.School && len(s.StudentIDs) == 0 {
		return errVoucherScopeRequired
	}
	return nil
}

func (s VoucherScope) allows(studentID string) bool {
	if s.School && len(s.StudentIDs) == 0 {
		return true
	}
	for _, id := range s.StudentIDs {
		if id == studentID {
			return true
		}
	}
	return false
}

// voucherStatuses are the statuses a read may filter on ("" is any).
var voucherStatuses = map[string]bool{
	"": true, domain.VoucherPending: true, domain.VoucherApplied: true, domain.VoucherRejected: true,
}

// Upload stores a family's voucher: a JPEG, PNG or PDF judged by its bytes, at
// most 5 MB, for one pupil. The same file again for the same pupil returns the
// voucher already sent (replayed) without uploading it twice; a pupil with
// MaxPendingVouchersPerStudent waiting is ErrVoucherLimit.
//
// Everything that can refuse it is checked before the upload. The file then
// goes up first and the row second; if the row cannot be written, the upload
// is deleted, so no object is left that nothing references.
func (s *PaymentVoucherService) Upload(ctx context.Context, in VoucherUpload) (domain.PaymentVoucher, bool, error) {
	if s.receipts == nil {
		return domain.PaymentVoucher{}, false, errReceiptsUnavailable
	}
	file, err := domain.NewReceiptFile(in.Data)
	if err != nil {
		return domain.PaymentVoucher{}, false, err
	}
	if _, err := s.students.ByID(ctx, in.StudentID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.PaymentVoucher{}, false, glerrors.New(glerrors.KindNotFound, "student not found")
		}
		return domain.PaymentVoucher{}, false, err
	}
	if live, found, err := s.repo.LivePaymentVoucherByFile(ctx, in.StudentID, file.SHA256()); err != nil || found {
		return live, found, err
	}

	id := s.newID()
	voucher, err := domain.NewPaymentVoucher(id, domain.VoucherDraft{
		StudentID: in.StudentID, ClaimedAmountCents: in.ClaimedAmountCents, ClaimedPaidOn: in.ClaimedPaidOn,
	}, domain.VoucherFile{
		Key: "vouchers/" + id + "/" + s.newID(), ContentType: file.ContentType(),
		SizeBytes: len(file.Data()), SHA256: file.SHA256(),
	}, in.By, s.clock.Today())
	if err != nil {
		return domain.PaymentVoucher{}, false, err
	}

	key := voucher.File().Key
	if err := s.receipts.Put(ctx, key, file.ContentType(), file.Data()); err != nil {
		return domain.PaymentVoucher{}, false, err
	}
	stored, replayed, err := s.repo.CreatePaymentVoucher(ctx, voucher, domain.VoucherEvent{
		ID: s.newID(), VoucherID: id, Kind: domain.VoucherEventUploaded, By: in.By, At: s.clock.Now(),
	})
	if err != nil {
		return domain.PaymentVoucher{}, false, errors.Join(err, s.receipts.Delete(ctx, key))
	}
	if replayed {
		// Another upload of the same file won the race: its object is the
		// one referenced. Best-effort, as AttachReceipt's: a failure leaves an
		// unreferenced object, which the store logs with its key.
		_ = s.receipts.Delete(ctx, key)
	}
	return stored, replayed, nil
}

// List reads vouchers within scope, newest first, optionally by status.
func (s *PaymentVoucherService) List(ctx context.Context, scope VoucherScope, status string, limit int) ([]domain.PaymentVoucher, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if !voucherStatuses[status] {
		return nil, glerrors.New(glerrors.KindInvalid, "status must be pending, applied or rejected")
	}
	if limit < 0 || limit > ports.MaxVoucherListLimit {
		return nil, glerrors.New(glerrors.KindInvalid, "limit must be between 0 and 500")
	}
	return s.repo.ListPaymentVouchers(ctx, ports.VoucherFilter{Status: status, StudentIDs: scope.StudentIDs, Limit: limit})
}

// File returns a voucher's file, within scope. Outside it, or missing, is
// ErrVoucherNotOwned: an id is never confirmed to exist.
func (s *PaymentVoucherService) File(ctx context.Context, id string, scope VoucherScope) (domain.ReceiptFile, error) {
	if err := scope.validate(); err != nil {
		return domain.ReceiptFile{}, err
	}
	if s.receipts == nil {
		return domain.ReceiptFile{}, errReceiptsUnavailable
	}
	v, err := s.voucher(ctx, id)
	if err != nil {
		return domain.ReceiptFile{}, err
	}
	if !scope.allows(v.StudentID()) {
		return domain.ReceiptFile{}, domain.ErrVoucherNotOwned
	}
	data, err := s.receipts.Get(ctx, v.File().Key)
	if err != nil {
		return domain.ReceiptFile{}, err
	}
	return domain.NewReceiptFile(data)
}

// Reject refuses a pending voucher with the reason the family will read.
// Rejecting a rejected voucher again answers it as it stands (replayed): the
// first reason stays and no second event is written. An applied voucher is
// ErrVoucherAlreadyApplied — its money is undone by voiding the deposit.
func (s *PaymentVoucherService) Reject(ctx context.Context, id, reason string, by domain.Receiver) (domain.PaymentVoucher, bool, error) {
	canonical, err := uuid.Parse(id)
	if err != nil {
		return domain.PaymentVoucher{}, false, domain.ErrVoucherNotOwned
	}
	replayed := false
	at := s.clock.Now()
	v, err := s.repo.UpdatePaymentVoucher(ctx, canonical.String(),
		func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error) {
			if cur.Status() == domain.VoucherRejected {
				replayed = true
				return cur, domain.VoucherEvent{}, nil
			}
			next, err := cur.Reject(reason, by, at)
			if err != nil {
				return domain.PaymentVoucher{}, domain.VoucherEvent{}, err
			}
			return next, domain.VoucherEvent{
				ID: s.newID(), VoucherID: cur.ID(), Kind: domain.VoucherEventRejected,
				By: by, Reason: next.RejectionReason(), At: at,
			}, nil
		})
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return domain.PaymentVoucher{}, false, domain.ErrVoucherNotOwned
	}
	if err != nil {
		return domain.PaymentVoucher{}, false, err
	}
	return v, replayed, nil
}

// voucher reads one voucher by id; a malformed or missing id is
// ErrVoucherNotOwned.
func (s *PaymentVoucherService) voucher(ctx context.Context, id string) (domain.PaymentVoucher, error) {
	canonical, err := uuid.Parse(id)
	if err != nil {
		return domain.PaymentVoucher{}, domain.ErrVoucherNotOwned
	}
	v, err := s.repo.GetPaymentVoucher(ctx, canonical.String())
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return domain.PaymentVoucher{}, domain.ErrVoucherNotOwned
	}
	return v, err
}
