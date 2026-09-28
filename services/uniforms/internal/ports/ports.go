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
	// RegisterPayment records money taken at the counter.
	RegisterPayment(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string) error
	// CancelChargeIfUnpaid voids a charge ONLY if nothing was collected, decided
	// under the charge's row lock. It is what makes the expiry sweep safe to run
	// while a cashier is collecting.
	CancelChargeIfUnpaid(ctx context.Context, chargeID string) error
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
}
