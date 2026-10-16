package domain

import (
	"bytes"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Expense statuses (T10). Cancelled is a state, never a deleted row: the day's
// cut and the month's report must still be able to explain themselves.
const (
	ExpenseActive    = "active"
	ExpenseCancelled = "cancelled"
)

const (
	// MaxExpenseCategoryName bounds a category's name, in characters.
	MaxExpenseCategoryName = 80
	// MaxExpenseDescription bounds what the office writes about an expense.
	MaxExpenseDescription = 500
	// MaxExpenseReason bounds the reason of an edit or a cancel.
	MaxExpenseReason = 500
	// MaxReceiptBytes is the largest receipt (comprobante) an expense keeps: a
	// phone photo of a ticket or a supplier's PDF invoice fits with room.
	MaxReceiptBytes = 5 << 20
)

// ExpenseCategory is a drawer of the school's filing cabinet: Papelería,
// Mantenimiento, Nómina… The school writes its own list. A category is
// deactivated, never deleted: recorded expenses keep pointing at it.
type ExpenseCategory struct {
	id     string
	name   string
	active bool
}

// NewExpenseCategory builds a validated category. The name is trimmed,
// required, and at most MaxExpenseCategoryName characters.
func NewExpenseCategory(id, name string, active bool) (ExpenseCategory, error) {
	trimmed, err := validCategoryName(name)
	if err != nil {
		return ExpenseCategory{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ExpenseCategory{}, glerrors.New(glerrors.KindInvalid, "category id is required")
	}
	return ExpenseCategory{id: id, name: trimmed, active: active}, nil
}

func validCategoryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", glerrors.New(glerrors.KindInvalid, "el nombre de la categoría es obligatorio")
	}
	if len([]rune(trimmed)) > MaxExpenseCategoryName {
		return "", glerrors.New(glerrors.KindInvalid, "el nombre de la categoría admite como máximo 80 caracteres")
	}
	return trimmed, nil
}

// WithUpdate returns the category renamed and (de)activated.
func (c ExpenseCategory) WithUpdate(name string, active bool) (ExpenseCategory, error) {
	return NewExpenseCategory(c.id, name, active)
}

// ID is the category's identifier.
func (c ExpenseCategory) ID() string { return c.id }

// Name is what the office calls it.
func (c ExpenseCategory) Name() string { return c.name }

// Active says whether new expenses may be filed under it.
func (c ExpenseCategory) Active() bool { return c.active }

// ExpenseDraft is what the office types for an expense: the fields it may set
// on create and change on edit.
type ExpenseDraft struct {
	// SpentOn is the day the money left. Only its calendar day matters.
	SpentOn     time.Time
	CategoryID  string
	AmountCents int64
	// Method is cash, card or transfer. A credit note is something the school
	// gives a family, not a way of paying a supplier.
	Method      string
	Description string
}

// ExpenseReceiptRef is where an expense's receipt lives in the object store:
// the key, the type its bytes proved to be, and its size. Not the bytes.
type ExpenseReceiptRef struct {
	Key         string
	ContentType string
	SizeBytes   int
}

// Expense is money the school paid out (T10).
//
// It is immutable: Edit, Cancel and the receipt methods return a re-validated
// copy, and the use case records the before and after of every change in the
// expense's history.
type Expense struct {
	id           string
	spentOn      time.Time
	categoryID   string
	categoryName string
	amount       Cents
	method       string
	description  string
	status       string
	cancelReason string
	cancelledAt  time.Time
	receipt      ExpenseReceiptRef
	createdBy    Receiver
	createdAt    time.Time
	updatedAt    time.Time
}

// NewExpense builds a validated, active expense. today bounds the date: an
// expense is recorded after the money left, never ahead of it.
func NewExpense(id string, d ExpenseDraft, today time.Time, by Receiver) (Expense, error) {
	if strings.TrimSpace(id) == "" {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "expense id is required")
	}
	e := Expense{id: id, status: ExpenseActive, createdBy: by}
	return e.apply(d, today)
}

// apply validates a draft and copies it onto e.
func (e Expense) apply(d ExpenseDraft, today time.Time) (Expense, error) {
	if d.SpentOn.IsZero() {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "la fecha del egreso es obligatoria")
	}
	day := CalendarDay(d.SpentOn)
	if day.After(CalendarDay(today)) {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "la fecha del egreso no puede ser futura")
	}
	if strings.TrimSpace(d.CategoryID) == "" {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "la categoría es obligatoria")
	}
	if d.AmountCents <= 0 {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "el monto debe ser mayor a cero")
	}
	amount, err := NewCents(d.AmountCents)
	if err != nil {
		return Expense{}, err
	}
	if err := validateExpenseMethod(d.Method); err != nil {
		return Expense{}, err
	}
	description := strings.TrimSpace(d.Description)
	if len([]rune(description)) > MaxExpenseDescription {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "la descripción admite como máximo 500 caracteres")
	}
	if e.categoryID != d.CategoryID {
		e.categoryName = ""
	}
	e.spentOn, e.categoryID, e.amount, e.method, e.description = day, d.CategoryID, amount, d.Method, description
	return e, nil
}

// validateExpenseMethod admits the three ways money leaves the school.
func validateExpenseMethod(method string) error {
	switch method {
	case MethodCash, MethodCard, MethodTransfer:
		return nil
	default:
		return glerrors.New(glerrors.KindInvalid, "la forma de pago del egreso debe ser efectivo, tarjeta o transferencia")
	}
}

// CalendarDay is t's calendar day at midnight UTC: the day a STORED date stands
// for (a spent_on, a counter-dated paid_at, a parsed YYYY-MM-DD). It is not how
// "today" is found — that is SchoolDate through the service's clock (ADR 0035).
func CalendarDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ErrExpenseCancelled refuses any change to a cancelled expense.
var ErrExpenseCancelled = glerrors.New(glerrors.KindConflict, "el egreso está cancelado")

// Edit returns the expense with the draft's fields. A cancelled expense is
// final: editing it is a conflict.
func (e Expense) Edit(d ExpenseDraft, today time.Time) (Expense, error) {
	if e.Cancelled() {
		return Expense{}, ErrExpenseCancelled
	}
	return e.apply(d, today)
}

// ValidateExpenseReason checks an optional reason's length and trims it.
func ValidateExpenseReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if len([]rune(trimmed)) > MaxExpenseReason {
		return "", glerrors.New(glerrors.KindInvalid, "el motivo admite como máximo 500 caracteres")
	}
	return trimmed, nil
}

// Cancel marks the expense cancelled with its reason (required). It then
// counts for nothing, and stays.
func (e Expense) Cancel(reason string, at time.Time) (Expense, error) {
	if e.Cancelled() {
		return Expense{}, ErrExpenseCancelled
	}
	trimmed, err := ValidateExpenseReason(reason)
	if err != nil {
		return Expense{}, err
	}
	if trimmed == "" {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "el motivo de la cancelación es obligatorio")
	}
	e.status, e.cancelReason, e.cancelledAt = ExpenseCancelled, trimmed, at
	return e, nil
}

// WithReceipt returns the expense with its receipt attached (or replaced).
func (e Expense) WithReceipt(ref ExpenseReceiptRef) (Expense, error) {
	if e.Cancelled() {
		return Expense{}, ErrExpenseCancelled
	}
	if ref.Key == "" || ref.ContentType == "" || ref.SizeBytes <= 0 {
		return Expense{}, glerrors.New(glerrors.KindInvalid, "receipt reference is incomplete")
	}
	e.receipt = ref
	return e, nil
}

// WithoutReceipt returns the expense with no receipt.
func (e Expense) WithoutReceipt() Expense {
	e.receipt = ExpenseReceiptRef{}
	return e
}

// RehydrateExpense rebuilds a stored expense. For the repository only: the
// row passed every rule when it was written, and the database CHECKs keep it so.
func RehydrateExpense(id string, d ExpenseDraft, categoryName, status, cancelReason string, cancelledAt time.Time,
	receipt ExpenseReceiptRef, createdBy Receiver, createdAt, updatedAt time.Time,
) Expense {
	return Expense{
		id: id, spentOn: CalendarDay(d.SpentOn), categoryID: d.CategoryID, categoryName: categoryName,
		amount: Cents(d.AmountCents), method: d.Method, description: d.Description,
		status: status, cancelReason: cancelReason, cancelledAt: cancelledAt, receipt: receipt,
		createdBy: createdBy, createdAt: createdAt, updatedAt: updatedAt,
	}
}

// ID is the expense's identifier.
func (e Expense) ID() string { return e.id }

// SpentOn is the day the money left (midnight UTC).
func (e Expense) SpentOn() time.Time { return e.spentOn }

// CategoryID is the category it is filed under.
func (e Expense) CategoryID() string { return e.categoryID }

// CategoryName is the category's name as read with the expense; empty when
// the expense was built rather than read.
func (e Expense) CategoryName() string { return e.categoryName }

// Amount is how much left.
func (e Expense) Amount() Cents { return e.amount }

// Method is cash, card or transfer.
func (e Expense) Method() string { return e.method }

// Description is what the office wrote about it.
func (e Expense) Description() string { return e.description }

// Status is active or cancelled.
func (e Expense) Status() string { return e.status }

// Cancelled says whether the expense counts for nothing.
func (e Expense) Cancelled() bool { return e.status == ExpenseCancelled }

// CancelReason is why it was cancelled.
func (e Expense) CancelReason() string { return e.cancelReason }

// CancelledAt is when it was cancelled; zero while active.
func (e Expense) CancelledAt() time.Time { return e.cancelledAt }

// Receipt is the attached receipt's reference, and whether there is one.
func (e Expense) Receipt() (ExpenseReceiptRef, bool) { return e.receipt, e.receipt.Key != "" }

// CreatedBy is who recorded it.
func (e Expense) CreatedBy() Receiver { return e.createdBy }

// CreatedAt is when it was recorded (stamped by the database).
func (e Expense) CreatedAt() time.Time { return e.createdAt }

// UpdatedAt is when it last changed (stamped by the database).
func (e Expense) UpdatedAt() time.Time { return e.updatedAt }

// Expense event kinds: one row of the history per change.
const (
	ExpenseEventCreated         = "created"
	ExpenseEventEdited          = "edited"
	ExpenseEventCancelled       = "cancelled"
	ExpenseEventReceiptAttached = "receipt_attached"
	ExpenseEventReceiptRemoved  = "receipt_removed"
)

// ExpenseEvent is one entry of an expense's history: what changed, the
// expense before and after, who, when and (optionally) why.
type ExpenseEvent struct {
	ID        string
	ExpenseID string
	Kind      string
	// Before is the zero Expense on create.
	Before Expense
	After  Expense
	Reason string
	By     Receiver
	At     time.Time
}

// ReceiptFile is an expense's receipt: a JPEG, PNG or PDF, judged by its
// bytes, at most MaxReceiptBytes.
type ReceiptFile struct {
	contentType string
	data        []byte
}

var (
	receiptJPEG = []byte{0xFF, 0xD8, 0xFF}
	receiptPNG  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	receiptPDF  = []byte("%PDF-")
)

// NewReceiptFile accepts a JPEG, PNG or PDF by the file's own leading bytes —
// never by a name or a client-supplied content type, so a page or a script
// renamed to .pdf is refused — and at most MaxReceiptBytes.
func NewReceiptFile(data []byte) (ReceiptFile, error) {
	if len(data) == 0 {
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante está vacío")
	}
	if len(data) > MaxReceiptBytes {
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante pesa más de 5 MB")
	}
	switch {
	case bytes.HasPrefix(data, receiptJPEG):
		return ReceiptFile{contentType: "image/jpeg", data: data}, nil
	case bytes.HasPrefix(data, receiptPNG):
		return ReceiptFile{contentType: "image/png", data: data}, nil
	case bytes.HasPrefix(data, receiptPDF):
		return ReceiptFile{contentType: "application/pdf", data: data}, nil
	default:
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante debe ser una imagen JPG o PNG, o un PDF")
	}
}

// ContentType is image/jpeg, image/png or application/pdf, as the bytes say.
func (f ReceiptFile) ContentType() string { return f.contentType }

// Data is the file itself.
func (f ReceiptFile) Data() []byte { return f.data }
