package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Payment voucher statuses (ADR 0040). A voucher is never deleted: rejected
// and applied ones are the record of what a family sent and what the office
// did with it.
const (
	VoucherPending  = "pending"
	VoucherApplied  = "applied"
	VoucherRejected = "rejected"
)

const (
	// MaxPendingVouchersPerStudent is how many vouchers a pupil may have
	// waiting for the office at once: enough for a month and its arrears,
	// not a way to flood the queue.
	MaxPendingVouchersPerStudent = 3
	// MinVoucherRejectionReason and MaxVoucherRejectionReason bound, in
	// characters, what the office tells a family when it refuses a voucher.
	MinVoucherRejectionReason = 3
	MaxVoucherRejectionReason = 300
	// voucherKeyPrefix starts every idempotency key derived from a voucher.
	voucherKeyPrefix = "voucher:"
	// sha256Bytes is the length of a file digest.
	sha256Bytes = 32
)

// Machine codes of the voucher refusals, sent as X-Error-Code so a screen can
// react to them without parsing Spanish.
const (
	CodeVoucherAlreadyApplied = "VOUCHER_ALREADY_APPLIED"
	CodeVoucherRejected       = "VOUCHER_REJECTED"
	CodeVoucherChoiceRequired = "VOUCHER_CHOICE_REQUIRED"
	CodeVoucherLimit          = "VOUCHER_LIMIT"
)

var (
	// ErrVoucherAlreadyApplied: the voucher already paid a deposit. Applying
	// it again would charge the family twice.
	ErrVoucherAlreadyApplied = glerrors.New(glerrors.KindConflict, "este comprobante ya se aplicó a un pago")
	// ErrVoucherRejected: the office refused the voucher; it pays nothing.
	ErrVoucherRejected = glerrors.New(glerrors.KindConflict, "este comprobante fue rechazado")
	// ErrVoucherChoiceRequired: a payment registered by hand for a pupil with
	// more than one voucher waiting cannot guess which one it is.
	ErrVoucherChoiceRequired = glerrors.New(glerrors.KindConflict,
		"este alumno tiene varios comprobantes por revisar: elige cuál corresponde a este pago")
	// ErrVoucherLimit: the pupil already has MaxPendingVouchersPerStudent
	// vouchers waiting.
	ErrVoucherLimit = glerrors.New(glerrors.KindConflict,
		"ya hay 3 comprobantes por revisar para este alumno; espera a que el colegio los revise")
	// ErrVoucherNotOwned: the voucher does not exist, or is not one of the
	// pupils the caller may see. NotFound either way, so an id cannot be
	// probed for existence (ADR 0010).
	ErrVoucherNotOwned = glerrors.New(glerrors.KindNotFound, "comprobante no encontrado")
	// ErrVoucherNotApplied: only an applied voucher can be released by a void
	// or moved by a correction.
	ErrVoucherNotApplied = glerrors.New(glerrors.KindConflict, "el comprobante no está aplicado a ningún pago")
)

// VoucherAlreadyApplied is ErrVoucherAlreadyApplied naming the receipt it paid,
// so the office can find that payment.
func VoucherAlreadyApplied(folio string) error {
	if strings.TrimSpace(folio) == "" {
		return ErrVoucherAlreadyApplied
	}
	return glerrors.Wrap(glerrors.KindConflict, "este comprobante ya se aplicó al pago "+folio, ErrVoucherAlreadyApplied)
}

// VoucherCodeOf is err's voucher machine code, or "" when it is none of them.
func VoucherCodeOf(err error) string {
	switch {
	case errors.Is(err, ErrVoucherAlreadyApplied):
		return CodeVoucherAlreadyApplied
	case errors.Is(err, ErrVoucherRejected):
		return CodeVoucherRejected
	case errors.Is(err, ErrVoucherChoiceRequired):
		return CodeVoucherChoiceRequired
	case errors.Is(err, ErrVoucherLimit):
		return CodeVoucherLimit
	default:
		return ""
	}
}

// VoucherIdempotencyKey is the deposit idempotency key of a voucher's
// application. releases counts how many times a void handed the voucher back:
// the voided deposit keeps the key it was stored under, so the next
// application needs a key of its own or it would replay the voided money.
func VoucherIdempotencyKey(voucherID string, releases int) string {
	if releases == 0 {
		return voucherKeyPrefix + voucherID
	}
	return voucherKeyPrefix + voucherID + ":" + strconv.Itoa(releases)
}

// VoucherDraft is what a family says about a voucher. Both claims are
// optional and are never money: they only prefill what the office confirms.
type VoucherDraft struct {
	StudentID string
	// ClaimedAmountCents is the amount the family says it paid; 0 is none.
	ClaimedAmountCents int64
	// ClaimedPaidOn is the day the family says it paid; zero is none.
	ClaimedPaidOn time.Time
}

// VoucherFile is where a voucher's file lives in the object store, the type
// its bytes proved to be, its size and its digest. Not the bytes.
type VoucherFile struct {
	Key         string
	ContentType string
	SizeBytes   int
	SHA256      []byte
}

func (f VoucherFile) validate() error {
	if strings.TrimSpace(f.Key) == "" || f.ContentType == "" || f.SizeBytes <= 0 || f.SizeBytes > MaxReceiptBytes {
		return glerrors.New(glerrors.KindInvalid, "voucher file reference is incomplete")
	}
	if len(f.SHA256) != sha256Bytes {
		return glerrors.New(glerrors.KindInvalid, "voucher file digest must be a SHA-256")
	}
	return nil
}

// PaymentVoucher is a family's proof of a transfer, sent from the app for one
// pupil (ADR 0040): a photo or a PDF, and optionally what they say they paid
// and when. It is not money. The office reads it, confirms the amount and
// registers the payment — and the voucher then points at that deposit.
//
// It is immutable: every transition returns a re-validated copy.
//
// A voucher is linked to at most one deposit and a deposit to at most one
// voucher. That is the invariant that keeps a family from being charged twice
// for one transfer; the database holds it too (uq_payment_vouchers_deposit).
type PaymentVoucher struct {
	id              string
	studentID       string
	uploadedBy      Receiver
	claimedAmount   Cents
	claimedPaidOn   time.Time
	file            VoucherFile
	status          string
	depositID       string
	decidedBy       Receiver
	decidedAt       time.Time
	rejectionReason string
	releases        int
	depositFolio    string
	createdAt       time.Time
	updatedAt       time.Time
}

// NewPaymentVoucher builds a validated, pending voucher. today bounds the
// claimed day: a family cannot have paid tomorrow.
func NewPaymentVoucher(id string, d VoucherDraft, file VoucherFile, by Receiver, today time.Time) (PaymentVoucher, error) {
	if strings.TrimSpace(id) == "" {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "voucher id is required")
	}
	if strings.TrimSpace(d.StudentID) == "" {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "el alumno es obligatorio")
	}
	if strings.TrimSpace(by.UserID) == "" {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "voucher uploader is required")
	}
	amount, err := NewCents(d.ClaimedAmountCents)
	if err != nil {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "el importe del comprobante no es válido")
	}
	var paidOn time.Time
	if !d.ClaimedPaidOn.IsZero() {
		paidOn = CalendarDay(d.ClaimedPaidOn)
		if paidOn.After(CalendarDay(today)) {
			return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "la fecha de pago no puede ser futura")
		}
	}
	if err := file.validate(); err != nil {
		return PaymentVoucher{}, err
	}
	return PaymentVoucher{
		id: id, studentID: d.StudentID, uploadedBy: by,
		claimedAmount: amount, claimedPaidOn: paidOn, file: file, status: VoucherPending,
	}, nil
}

// refuseUnlessPending is the conflict of deciding a voucher already decided.
func (v PaymentVoucher) refuseUnlessPending() error {
	switch v.status {
	case VoucherApplied:
		return ErrVoucherAlreadyApplied
	case VoucherRejected:
		return ErrVoucherRejected
	default:
		return nil
	}
}

// Apply links the pending voucher to the deposit that registered its money.
func (v PaymentVoucher) Apply(depositID string, by Receiver, at time.Time) (PaymentVoucher, error) {
	if err := v.refuseUnlessPending(); err != nil {
		return PaymentVoucher{}, err
	}
	if strings.TrimSpace(depositID) == "" {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "an applied voucher needs its deposit")
	}
	v.status, v.depositID, v.decidedBy, v.decidedAt, v.updatedAt = VoucherApplied, depositID, by, at, at
	v.depositFolio = ""
	return v, nil
}

// ValidateVoucherRejectionReason trims a rejection reason and checks it is
// between MinVoucherRejectionReason and MaxVoucherRejectionReason characters.
func ValidateVoucherRejectionReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	n := utf8.RuneCountInString(trimmed)
	if n < MinVoucherRejectionReason || n > MaxVoucherRejectionReason {
		return "", glerrors.New(glerrors.KindInvalid, "el motivo del rechazo debe tener entre 3 y 300 caracteres")
	}
	return trimmed, nil
}

// Reject refuses the pending voucher with the reason the family will read.
func (v PaymentVoucher) Reject(reason string, by Receiver, at time.Time) (PaymentVoucher, error) {
	if err := v.refuseUnlessPending(); err != nil {
		return PaymentVoucher{}, err
	}
	trimmed, err := ValidateVoucherRejectionReason(reason)
	if err != nil {
		return PaymentVoucher{}, err
	}
	v.status, v.rejectionReason, v.decidedBy, v.decidedAt, v.updatedAt = VoucherRejected, trimmed, by, at, at
	return v, nil
}

// Release hands an applied voucher back to the queue because its deposit was
// voided: pending again, unlinked, and counted (see VoucherIdempotencyKey).
func (v PaymentVoucher) Release(at time.Time) (PaymentVoucher, error) {
	if v.status != VoucherApplied {
		return PaymentVoucher{}, ErrVoucherNotApplied
	}
	v.status, v.depositID, v.decidedBy, v.decidedAt, v.depositFolio = VoucherPending, "", Receiver{}, time.Time{}, ""
	v.releases++
	v.updatedAt = at
	return v, nil
}

// MoveTo re-points an applied voucher at the deposit that corrects the one it
// paid, so it always points at a live deposit.
func (v PaymentVoucher) MoveTo(depositID string, at time.Time) (PaymentVoucher, error) {
	if v.status != VoucherApplied {
		return PaymentVoucher{}, ErrVoucherNotApplied
	}
	depositID = strings.TrimSpace(depositID)
	if depositID == "" || depositID == v.depositID {
		return PaymentVoucher{}, glerrors.New(glerrors.KindInvalid, "a moved voucher needs another deposit")
	}
	v.depositID, v.depositFolio, v.updatedAt = depositID, "", at
	return v, nil
}

// VoucherState is a stored voucher's decision, for RehydratePaymentVoucher.
type VoucherState struct {
	Status          string
	DepositID       string
	DecidedBy       Receiver
	DecidedAt       time.Time
	RejectionReason string
	Releases        int
	// DepositFolio is the receipt number of DepositID, read with the row.
	DepositFolio string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RehydratePaymentVoucher rebuilds a stored voucher. For the repository only:
// the row passed every rule when it was written, and the CHECKs keep it so.
func RehydratePaymentVoucher(id string, d VoucherDraft, file VoucherFile, by Receiver, st VoucherState) PaymentVoucher {
	var paidOn time.Time
	if !d.ClaimedPaidOn.IsZero() {
		paidOn = CalendarDay(d.ClaimedPaidOn)
	}
	return PaymentVoucher{
		id: id, studentID: d.StudentID, uploadedBy: by,
		claimedAmount: Cents(d.ClaimedAmountCents), claimedPaidOn: paidOn, file: file,
		status: st.Status, depositID: st.DepositID, decidedBy: st.DecidedBy, decidedAt: st.DecidedAt,
		rejectionReason: st.RejectionReason, releases: st.Releases, depositFolio: st.DepositFolio,
		createdAt: st.CreatedAt, updatedAt: st.UpdatedAt,
	}
}

// ID is the voucher's identifier.
func (v PaymentVoucher) ID() string { return v.id }

// StudentID is the pupil the family sent it for.
func (v PaymentVoucher) StudentID() string { return v.studentID }

// UploadedBy is the guardian who sent it.
func (v PaymentVoucher) UploadedBy() Receiver { return v.uploadedBy }

// ClaimedAmount is the amount the family says it paid, and whether it said.
func (v PaymentVoucher) ClaimedAmount() (Cents, bool) { return v.claimedAmount, v.claimedAmount > 0 }

// ClaimedPaidOn is the day the family says it paid, and whether it said.
func (v PaymentVoucher) ClaimedPaidOn() (time.Time, bool) {
	return v.claimedPaidOn, !v.claimedPaidOn.IsZero()
}

// File is where the voucher's file lives.
func (v PaymentVoucher) File() VoucherFile { return v.file }

// Status is pending, applied or rejected.
func (v PaymentVoucher) Status() string { return v.status }

// Pending says whether the office still has to decide it.
func (v PaymentVoucher) Pending() bool { return v.status == VoucherPending }

// DepositID is the deposit it paid; empty unless applied.
func (v PaymentVoucher) DepositID() string { return v.depositID }

// DepositFolio is the receipt number of the deposit it paid, as read with
// the voucher; empty when it was built rather than read, or is not applied.
func (v PaymentVoucher) DepositFolio() string { return v.depositFolio }

// DecidedBy is who applied or rejected it.
func (v PaymentVoucher) DecidedBy() Receiver { return v.decidedBy }

// DecidedAt is when it was applied or rejected; zero while pending.
func (v PaymentVoucher) DecidedAt() time.Time { return v.decidedAt }

// RejectionReason is what the office told the family; empty unless rejected.
func (v PaymentVoucher) RejectionReason() string { return v.rejectionReason }

// Releases counts the voids that handed it back to the queue.
func (v PaymentVoucher) Releases() int { return v.releases }

// IdempotencyKey is the deposit key of its next (or current) application.
func (v PaymentVoucher) IdempotencyKey() string { return VoucherIdempotencyKey(v.id, v.releases) }

// CreatedAt is when it was sent (stamped by the database).
func (v PaymentVoucher) CreatedAt() time.Time { return v.createdAt }

// UpdatedAt is when it last changed.
func (v PaymentVoucher) UpdatedAt() time.Time { return v.updatedAt }

// Payment voucher event kinds: the append-only history of a voucher.
const (
	VoucherEventUploaded          = "uploaded"
	VoucherEventApplied           = "applied"
	VoucherEventAutoLinked        = "auto_linked"
	VoucherEventRejected          = "rejected"
	VoucherEventReleasedByVoid    = "released_by_void"
	VoucherEventMovedByCorrection = "moved_by_correction"
)

// VoucherEvent is one entry of a voucher's history.
type VoucherEvent struct {
	ID        string
	VoucherID string
	Kind      string
	DepositID string
	By        Receiver
	Reason    string
	At        time.Time
}
