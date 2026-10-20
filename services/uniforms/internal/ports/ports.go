package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

// This file declares the seams the uniform use cases depend on. Interfaces live
// here, on the consumer side; the adapters (postgres, Connect clients to
// finance/students/config, the event publisher) implement them. The domain
// imports none of it — dependencies point inward (ADR 0007).

// ItemWithSizes pairs a garment with its size run. The two are always read
// together — a garment without its sizes cannot be ordered from and cannot be
// counted — so they travel together rather than costing a second query per row.
type ItemWithSizes struct {
	Item  domain.Item
	Sizes []domain.StockLevel
}

// OrderAggregate is an order with everything that hangs off it. Reads return
// this shape because a panel row is meaningless without its deliveries: the
// order has no status of its own, and its label is derived from these.
type OrderAggregate struct {
	Order        domain.Order
	Lines        []domain.OrderLine
	Fulfillments []domain.Fulfillment
	// Returns are the counter sales of this order that came back, by
	// fulfillment id (C2-015).
	Returns map[string]CounterSaleReturn
}

// How a returned counter sale's money went back (C2-015).
const (
	// RefundVoid: returned the day it was sold; its payments were voided.
	RefundVoid = "void"
	// RefundExpense: returned another day; refunded as an expense of that day.
	RefundExpense = "expense"
	// RefundNone: no money had reached finance (anonymous or flagged sale).
	RefundNone = "none"
)

// Where a return's refund stands (C2-015).
const (
	RefundDone    = "done"
	RefundPending = "pending"
)

// CounterSaleReturn is the record of a counter sale taken back (C2-015).
type CounterSaleReturn struct {
	FulfillmentID string
	OrderID       string
	Reason        string
	RefundMode    string
	RefundMethod  string
	RefundCents   int64
	RefundStatus  string
	ExpenseID     string
	DamagedUnits  int
	ActorUserID   string
	ActorName     string
	ReturnedAt    time.Time
}

// ReturnCounterSaleCommand is one return, already decided by the use case:
// the returned delivery and lines, which lines are written off, and the record.
type ReturnCounterSaleCommand struct {
	Fulfillment domain.Fulfillment
	Lines       []domain.OrderLine
	// Damaged are the line ids written off instead of put back on sale.
	Damaged map[string]bool
	Record  CounterSaleReturn
	// Folio names the sale in the ledger notes.
	Folio string
}

// RefundInput is the expense a return of an earlier day is refunded as.
type RefundInput struct {
	AmountCents int64
	Method      string
	SpentOn     string
	Description string
	ActorUserID string
	ActorName   string
}

// SizeSpec is one row of a size run being created or updated.
type SizeSpec struct {
	SizeLabel string
	SizeOrder int
	MinStock  int
}

// ReceiveLine is one line of a goods receipt.
type ReceiveLine struct {
	StockID string
	Qty     int
}

// ListItemsFilter narrows the catalogue. Level is the family-facing one: it
// returns the garments scoped to that level PLUS the ones scoped to none.
type ListItemsFilter struct {
	IncludeInactive bool
	Level           domain.Level
}

// ListOrdersFilter is the panel's query. StudentIDs is the security-bearing
// field: when set, the read is restricted to those students AT THE SERVICE, not
// merely at the edge, so a caller cannot reach another family's order by id.
type ListOrdersFilter struct {
	StudentIDs            []string
	Level                 domain.Level
	Query                 string
	Statuses              []domain.FulfillmentStatus
	OnlyRequiresAttention bool
	From, To              time.Time
	Sort                  string
	Descending            bool
	PageSize              int
	PageToken             string
}

// Repository is the uniform shop's store. Everything is tenant-scoped for RLS;
// the adapter binds app.current_tenant per transaction.
type Repository interface {
	// --- catalogue ---
	CreateItem(ctx context.Context, item domain.Item) (domain.Item, error)
	UpdateItem(ctx context.Context, item domain.Item) (domain.Item, error)
	GetItem(ctx context.Context, id string) (domain.Item, error)
	ListItems(ctx context.Context, f ListItemsFilter) ([]ItemWithSizes, error)
	// UpsertSizes adds or updates a garment's size run in one call, reporting
	// created and updated separately so the panel can say "8 created, 2 already
	// existed" rather than upserting in silence.
	UpsertSizes(ctx context.Context, itemID string, specs []SizeSpec) (created, updated int, sizes []domain.StockLevel, err error)

	// --- inventory ---
	ListStock(ctx context.Context, itemID string, onlyBelowMinimum bool) ([]domain.StockLevel, error)
	GetStockBySize(ctx context.Context, itemID, sizeLabel string) (domain.StockLevel, error)
	// ReceiveStock books a supplier delivery and, in the SAME transaction,
	// allocates it to the lines that have been waiting for those sizes, oldest
	// first. Splitting the two would let a counter sale take, between them, the
	// piece a family has been waiting three weeks for.
	ReceiveStock(ctx context.Context, lines []ReceiveLine, note, actorUserID string) ([]domain.StockLevel, []string, error)
	AdjustStock(ctx context.Context, stockID string, delta int, note, actorUserID string) (domain.StockLevel, error)

	// --- orders ---
	// CounterSale writes the whole sale as ONE transaction: the order, its lines,
	// the terminal delivery, the stock deduction and the ledger entries. The
	// charge and payment in finance follow it and are eventually consistent over
	// it, because the goods have already left the storeroom.
	CounterSale(ctx context.Context, cmd CounterSaleCommand) (OrderAggregate, bool, error)
	// RecordCounterSaleCharge attaches the finance charge to a sale already
	// written. It is a second step because the goods left the storeroom in the
	// first one: the sale is the fact, and the ledger entry catches up to it.
	RecordCounterSaleCharge(ctx context.Context, fulfillmentID, chargeID string) error
	// FlagFulfillment hands a delivery to a person. Used when the money side of
	// a completed sale could not be recorded: the goods are gone, so the answer
	// is never to pretend, and never to unwind.
	FlagFulfillment(ctx context.Context, fulfillmentID, reason string) error
	// ReturnCounterSale applies a return in one transaction: the delivery and
	// lines become returned, every unit comes back to the shelf ('return'), a
	// damaged one is written off at once ('adjust'), and the record is kept.
	// A delivery already returned is answered as it stands (C2-015).
	ReturnCounterSale(ctx context.Context, cmd ReturnCounterSaleCommand) (OrderAggregate, error)
	// SettleReturnRefund marks a pending refund as recorded by its expense.
	SettleReturnRefund(ctx context.Context, fulfillmentID, expenseID string) error
	ListOrders(ctx context.Context, f ListOrdersFilter) ([]OrderAggregate, string, int, error)
	GetOrder(ctx context.Context, id string, studentIDs []string) (OrderAggregate, error)

	// --- settings ---
	GetSettings(ctx context.Context) (domain.Settings, error)
	UpdateSettings(ctx context.Context, s domain.Settings) (domain.Settings, error)
}

// CounterSaleCommand is a priced, validated sale ready to be written. The use
// case builds it — pricing every line from the catalogue — so the repository
// never decides what anything costs.
type CounterSaleCommand struct {
	OrderID           string
	FulfillmentID     string
	TenantID          string
	StudentID         string
	StudentName       string
	StudentLevel      domain.Level
	StudentLevelName  string
	StudentGradeGroup string
	CycleID           string
	Lines             []domain.OrderLine
	TotalCents        int64
	Note              string
	ActorUserID       string
	IdempotencyKey    string
	SoldAt            time.Time
	// FolioYear is the year the folio numbers under: the school's calendar
	// year of the sale (the day its receipt prints), not SoldAt's UTC year —
	// a sale at 19:00 on 31 December in Morelia is already 2027 in UTC.
	FolioYear int
	// Admit runs after the replay check and before anything is written; its
	// error refuses the sale (T8: tenders that do not add up). Nil admits.
	Admit func() error
}

// StudentRef is the uniform shop's view of a students record: who they are, what
// level they are in and whether the enrolment is live. Enough to price and label
// an order, and no PII beyond the name the panel must show.
type StudentRef struct {
	ID         string
	FullName   string
	Level      domain.Level
	LevelName  string
	GradeGroup string
	CycleID    string
	Status     string
}

// StudentReader validates and describes a student, by RPC. A missing student is
// glerrors.ErrNotFound.
type StudentReader interface {
	ByID(ctx context.Context, id string) (StudentRef, error)
}

// ChargeRef is what finance answers about a charge the shop raised.
type ChargeRef struct {
	ID             string
	PaidCents      int64
	RemainingCents int64
	Status         string
}

// Tender is one method a counter sale was paid by (T8): cash | card |
// transfer | credit_note, with its amount. A credit note is one the school
// issued by hand; finance keeps it out of the takings.
type Tender struct {
	Method      string
	AmountCents int64
	Reference   string
	// CardType is debit or credit, only on a card.
	CardType string
}

// FinanceWriter raises and settles the money side of the shop. It is a WRITE
// seam into another bounded context, which is unusual here and deliberate: the
// alternative was a second till, and a school cannot reconcile two.
type FinanceWriter interface {
	// EnsureConcept returns the id of the "Uniformes" charge concept, creating
	// it if the school has not got one yet.
	EnsureConcept(ctx context.Context) (string, error)
	// CreateCharge raises a charge. externalRef makes the call idempotent: a
	// retry after a lost response returns the original charge instead of billing
	// the family twice (ADR 0008).
	CreateCharge(ctx context.Context, in CreateChargeInput) (ChargeRef, error)
	// RegisterPayment records money taken at the counter, by the methods it
	// came in (T8): one tender, or a credit note completed with cash. The
	// tenders add up to amountCents.
	RegisterPayment(ctx context.Context, chargeID string, amountCents int64, tenders []Tender, idempotencyKey string) error
	// CancelChargeIfUnpaid voids a charge ONLY if nothing was collected, decided
	// under the charge's row lock. It is what makes the expiry sweep safe to run
	// while a cashier is collecting.
	CancelChargeIfUnpaid(ctx context.Context, chargeID string) error
	// VoidSalePayments voids a counter sale's payments and cancels its charge,
	// for a sale returned the day it was sold (C2-015). Idempotent.
	VoidSalePayments(ctx context.Context, chargeID, reason, actorUserID string) error
	// RecordRefund records the refund of a sale returned on a later day as an
	// expense of that day, and returns the expense id (C2-015).
	RecordRefund(ctx context.Context, in RefundInput) (string, error)
}

// CreateChargeInput is one charge the shop raises against a family.
type CreateChargeInput struct {
	StudentID   string
	CycleID     string
	ConceptID   string
	Description string
	AmountCents int64
	DueDate     string
	ExternalRef string
}

// EventPublisher emits the shop's domain events. Publishing is best-effort at
// the call site and durable through the outbox; a broker outage must never fail
// a sale that already happened.
type EventPublisher interface {
	Publish(ctx context.Context, e Event) error
}

// Event is one thing worth telling other services about.
type Event struct {
	Type     string
	TenantID string
	Payload  any
}

// LevelArchiveRepository is uniforms' half of the level-deletion trash (ADR
// 0021). A garment scoped to a deleted level is KEPT: its level is snapshotted
// in the archive and the garment is left unresolved (offered to nobody) until
// the level is restored or staff re-scope it. Every method runs in ONE
// tenant-bound transaction; archive and restore are idempotent per batch.
type LevelArchiveRepository interface {
	PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error)
	ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error)
	PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
}
