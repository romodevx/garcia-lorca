package usecase

import (
	"context"
	"regexp"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// Service orchestrates the uniform shop. It validates cross-service references,
// prices everything from the catalogue, decides nothing the domain can decide,
// and hands the writes that must be atomic to the repository as one command.
type Service struct {
	repo     ports.Repository
	students ports.StudentReader
	finance  ports.FinanceWriter
	events   ports.EventPublisher
	tenantID string
	newID    func() string
	now      func() time.Time
}

// NewService wires the shop. newID and now are injected so every test is
// deterministic and nothing in here reads a real clock.
func NewService(
	repo ports.Repository,
	students ports.StudentReader,
	finance ports.FinanceWriter,
	events ports.EventPublisher,
	tenantID string,
	newID func() string,
	now func() time.Time,
) *Service {
	return &Service{repo: repo, students: students, finance: finance, events: events,
		tenantID: tenantID, newID: newID, now: now}
}

// --- catalogue ---------------------------------------------------------------

// CreateItemInput is a new garment.
type CreateItemInput struct {
	Name        string
	Description string
	Level       domain.Level
	PriceCents  int64
	// Category is uniform, book, supply or other. Empty means uniform, which is
	// what every item was before the module sold anything else.
	Category string
}

// CreateItem puts a garment on sale.
func (s *Service) CreateItem(ctx context.Context, in CreateItemInput) (domain.Item, error) {
	item, err := domain.NewItem(s.newID(), s.tenantID, in.Name, in.Description, in.Level, in.PriceCents)
	if err != nil {
		return domain.Item{}, err
	}
	if in.Category != "" {
		item, err = item.WithCategory(in.Category)
		if err != nil {
			return domain.Item{}, err
		}
	}
	return s.repo.CreateItem(ctx, item)
}

// UpdateItemInput edits a garment.
type UpdateItemInput struct {
	ID          string
	Name        string
	Description string
	Category    string
	Level       domain.Level
	PriceCents  int64
	Active      bool
}

// UpdateItem edits a garment. Repricing changes what the NEXT order costs;
// orders already placed keep the price they were quoted, because their lines
// snapshot it.
func (s *Service) UpdateItem(ctx context.Context, in UpdateItemInput) (domain.Item, error) {
	current, err := s.repo.GetItem(ctx, in.ID)
	if err != nil {
		return domain.Item{}, err
	}
	updated, err := current.WithDetails(in.Name, in.Description, in.Level, in.PriceCents)
	if err != nil {
		return domain.Item{}, err
	}
	if in.Category != "" {
		updated, err = updated.WithCategory(in.Category)
		if err != nil {
			return domain.Item{}, err
		}
	}
	if in.Active {
		updated = updated.Restore()
	} else {
		updated = updated.Retire()
	}
	return s.repo.UpdateItem(ctx, updated)
}

// ListItems returns the catalogue, each garment with its size run.
func (s *Service) ListItems(ctx context.Context, f ports.ListItemsFilter) ([]ports.ItemWithSizes, error) {
	if !f.Level.IsValid() {
		return nil, glerrors.New(glerrors.KindInvalid, "unknown school level")
	}
	return s.repo.ListItems(ctx, f)
}

// UpsertSizes creates or updates a garment's whole size run in one call.
func (s *Service) UpsertSizes(ctx context.Context, itemID string, specs []ports.SizeSpec) (int, int, []domain.StockLevel, error) {
	if strings.TrimSpace(itemID) == "" {
		return 0, 0, nil, glerrors.New(glerrors.KindInvalid, "item id is required")
	}
	if len(specs) == 0 {
		return 0, 0, nil, glerrors.New(glerrors.KindInvalid, "give at least one size")
	}
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		label := strings.TrimSpace(spec.SizeLabel)
		if label == "" {
			return 0, 0, nil, glerrors.New(glerrors.KindInvalid, "a size needs a label")
		}
		if seen[label] {
			return 0, 0, nil, glerrors.New(glerrors.KindInvalid, "size "+label+" is listed twice")
		}
		seen[label] = true
		if spec.MinStock < 0 {
			return 0, 0, nil, glerrors.New(glerrors.KindInvalid, "a minimum cannot be negative")
		}
	}
	return s.repo.UpsertSizes(ctx, itemID, specs)
}

// --- inventory ---------------------------------------------------------------

// ListStock returns stock by garment and size.
func (s *Service) ListStock(ctx context.Context, itemID string, onlyBelowMinimum bool) ([]domain.StockLevel, error) {
	return s.repo.ListStock(ctx, itemID, onlyBelowMinimum)
}

// ReceiveStock books a supplier delivery. The repository allocates it to waiting
// lines inside the same transaction; splitting the two would let a counter sale
// take, in between, the piece a family has been waiting three weeks for.
func (s *Service) ReceiveStock(ctx context.Context, lines []ports.ReceiveLine, note, actorUserID string) ([]domain.StockLevel, []string, error) {
	if len(lines) == 0 {
		return nil, nil, glerrors.New(glerrors.KindInvalid, "a receipt needs at least one line")
	}
	for _, l := range lines {
		if strings.TrimSpace(l.StockID) == "" {
			return nil, nil, glerrors.New(glerrors.KindInvalid, "each line names a size")
		}
		if l.Qty < 1 {
			return nil, nil, glerrors.New(glerrors.KindInvalid, "a receipt line must be positive")
		}
	}
	return s.repo.ReceiveStock(ctx, lines, strings.TrimSpace(note), actorUserID)
}

// AdjustStock corrects a count. The note is required and enforced here rather
// than at the edge: this is the one write that can conceal a shortfall, and the
// ledger entry it leaves is worthless without a reason.
func (s *Service) AdjustStock(ctx context.Context, stockID string, delta int, note, actorUserID string) (domain.StockLevel, error) {
	if strings.TrimSpace(stockID) == "" {
		return domain.StockLevel{}, glerrors.New(glerrors.KindInvalid, "stock id is required")
	}
	if len(strings.TrimSpace(note)) < 3 {
		return domain.StockLevel{}, glerrors.New(glerrors.KindInvalid, "an adjustment needs a reason")
	}
	return s.repo.AdjustStock(ctx, stockID, delta, strings.TrimSpace(note), actorUserID)
}

// --- the counter sale --------------------------------------------------------

// SaleLineInput is one garment being sold. Note what is NOT here: a price.
type SaleLineInput struct {
	ItemID    string
	SizeLabel string
	Qty       int
}

// CounterSaleInput is the parent at the window.
type CounterSaleInput struct {
	// StudentID empty is a sale to someone who is not a pupil. Allowed, but then
	// Note must say who bought it.
	StudentID string
	Lines     []SaleLineInput
	// Method is cash | card | transfer | credit_note: the single method.
	// Tenders, when given, replace it (and Reference).
	Method    string
	Reference string
	// Tenders combine methods (T8): a credit note completed with cash. They
	// must add up exactly to the sale's total as the shop prices it.
	Tenders     []Tender
	Note        string
	ActorUserID string
	// IdempotencyKey is REQUIRED, and minted when the till drawer opens rather
	// than when the button is pressed.
	IdempotencyKey string
}

var validMethods = map[string]bool{"cash": true, "card": true, "transfer": true, "credit_note": true}

// Tender is one method of a combined payment at the counter (T8).
type Tender struct {
	Method      string
	AmountCents int64
	Reference   string
	// CardType is debit or credit, only on a card.
	CardType string
}

// maxTenders is how many methods one sale may combine.
const maxTenders = 4

// checkTenders refuses tenders the counter cannot have meant: an unknown
// method, an amount of nothing, more than four. Whether they add up is
// checked once the shop has priced the sale (tendersFor).
func checkTenders(tenders []Tender, anonymous bool) error {
	if len(tenders) > maxTenders {
		return glerrors.New(glerrors.KindInvalid, "una venta admite como máximo cuatro formas de pago")
	}
	for _, t := range tenders {
		if err := checkTender(t, anonymous); err != nil {
			return err
		}
	}
	return nil
}

// checkTender applies finance's rules for one tender here, where a refusal
// still costs nothing: finance refusing it later would leave the garments gone
// and the payment unrecorded.
func checkTender(t Tender, anonymous bool) error {
	switch {
	case !validMethods[t.Method]:
		return glerrors.New(glerrors.KindInvalid, "unknown payment method")
	case t.AmountCents <= 0:
		return glerrors.New(glerrors.KindInvalid, "cada forma de pago necesita un monto mayor a cero")
	case t.CardType != "" && (t.Method != "card" || (t.CardType != "debit" && t.CardType != "credit")):
		return glerrors.New(glerrors.KindInvalid, "el tipo de tarjeta es débito o crédito, y sólo en un pago con tarjeta")
	case anonymous && t.Method == creditNote:
		// A credit note is a family's; a sale to nobody in particular never
		// reaches a family's account, so it would vanish from every report.
		return glerrors.New(glerrors.KindInvalid, "una venta sin alumno no se puede pagar con nota de crédito")
	}
	return nil
}

// creditNote is the method of a credit note the school issued by hand.
const creditNote = "credit_note"

// tendersFor is how the sale's total is paid: the counter's tenders, which
// must add up to it exactly (the rule the school approved — a sale whose
// methods do not cover the total is not made), or the single method for all
// of it.
func tendersFor(in CounterSaleInput, total int64) ([]ports.Tender, error) {
	if len(in.Tenders) == 0 {
		return []ports.Tender{{Method: in.Method, AmountCents: total, Reference: in.Reference}}, nil
	}
	out := make([]ports.Tender, 0, len(in.Tenders))
	var sum int64
	for _, t := range in.Tenders {
		sum += t.AmountCents
		out = append(out, ports.Tender{Method: t.Method, AmountCents: t.AmountCents, Reference: strings.TrimSpace(t.Reference), CardType: t.CardType})
	}
	if sum != total {
		return nil, glerrors.New(glerrors.KindInvalid, "la suma de las formas de pago no coincide con el total de la venta")
	}
	return out, nil
}

// CounterSale sells and hands over in one act.
//
// The order of operations is the whole design, and it is deliberate:
//
//  1. price and validate everything, touching nothing
//  2. ONE transaction: the order, its lines, a terminal delivery, the stock
//     deduction and the ledger entries
//  3. finance: raise the charge, record the payment
//
// Step 3 comes last because by then the goods have already left the storeroom.
// If it fails there is nothing to unwind — unwinding would be a lie about the
// shelf — so the delivery is flagged for a person instead, with the reason on
// it. Loud and wrong beats quiet and wrong; the one thing not allowed is silence.
func (s *Service) CounterSale(ctx context.Context, in CounterSaleInput) (ports.OrderAggregate, error) {
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return ports.OrderAggregate{}, glerrors.New(glerrors.KindInvalid,
			"a counter sale needs an idempotency key: it moves goods and money at once")
	}
	anonymous := strings.TrimSpace(in.StudentID) == ""
	if len(in.Tenders) == 0 {
		if err := checkTender(Tender{Method: in.Method, AmountCents: 1}, anonymous); err != nil {
			return ports.OrderAggregate{}, err
		}
	}
	if err := checkTenders(in.Tenders, anonymous); err != nil {
		return ports.OrderAggregate{}, err
	}
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	if err := checkShape(settings, in.Lines); err != nil {
		return ports.OrderAggregate{}, err
	}

	student, err := s.resolveBuyer(ctx, in)
	if err != nil {
		return ports.OrderAggregate{}, err
	}

	orderID := s.newID()
	fulfillmentID := s.newID()
	lines, total, err := s.priceLines(ctx, orderID, in.Lines)
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	// Tenders that do not cover the total refuse the sale while the garments
	// are still on the shelf. Checked by the repository AFTER its replay check:
	// a retry of a sale that went through returns it even if the price moved.
	tenders, tenderErr := tendersFor(in, total)

	saved, replayed, err := s.repo.CounterSale(ctx, ports.CounterSaleCommand{
		OrderID: orderID, FulfillmentID: fulfillmentID, TenantID: s.tenantID,
		StudentID: student.ID, StudentName: student.FullName, StudentLevel: student.Level, StudentLevelName: student.LevelName,
		StudentGradeGroup: student.GradeGroup, CycleID: student.CycleID,
		Lines: lines, TotalCents: total, Note: strings.TrimSpace(in.Note),
		ActorUserID: in.ActorUserID, IdempotencyKey: in.IdempotencyKey, SoldAt: s.now(),
		Admit: func() error { return tenderErr },
	})
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	if replayed {
		// The till already sold this. Nothing moved, so nothing is billed again.
		return saved, nil
	}

	s.recordMoney(ctx, saved, tenders, total)
	return saved, nil
}

// resolveBuyer identifies who is buying. A named pupil is validated against the
// students service, which is also where the order's snapshot comes from. An
// anonymous sale is legitimate only with a note: the till has to reconcile
// against something, and "someone" is not something.
func (s *Service) resolveBuyer(ctx context.Context, in CounterSaleInput) (ports.StudentRef, error) {
	if strings.TrimSpace(in.StudentID) == "" {
		if strings.TrimSpace(in.Note) == "" {
			return ports.StudentRef{}, glerrors.New(glerrors.KindInvalid,
				"a sale with no student needs a note saying who bought it")
		}
		return ports.StudentRef{}, nil
	}
	// The shape is checked HERE, before the id leaves this service. A caller is
	// not a UI: sending a typed name where an id belongs used to travel all the
	// way to students and come back as a 500 — an answer that blames the server
	// for the caller's input and pages somebody at night for a typo.
	if !looksLikeUUID(in.StudentID) {
		return ports.StudentRef{}, glerrors.New(glerrors.KindInvalid,
			"student id must be an identifier, not a name")
	}
	student, err := s.students.ByID(ctx, in.StudentID)
	if err != nil {
		return ports.StudentRef{}, err
	}
	// The counter sells to pupils: enrolled, or registered and not yet placed
	// in a group (task 10) — the latter have no cycle, and finance charges them
	// with none. A rejected applicant or a withdrawn pupil is not a pupil; an
	// allowlist, so a status nobody named is refused too.
	if !buyerStatuses[student.Status] {
		return ports.StudentRef{}, glerrors.New(glerrors.KindInvalid,
			"esa persona ya no es alumno del colegio (baja o solicitud rechazada): no se le puede vender")
	}
	return student, nil
}

// buyerStatuses are the admission statuses the counter sells to.
var buyerStatuses = map[string]bool{"pending": true, "accepted": true, "enrolled": true}

// uuidPattern is the canonical 8-4-4-4-12 form. It is deliberately a shape check
// and not a parse: this service does not care which UUID version it is, only
// that the value is an identifier at all.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func looksLikeUUID(s string) bool { return uuidPattern.MatchString(strings.TrimSpace(s)) }

// priceLines turns what the caller asked for into priced lines, reading every
// amount from the catalogue. A garment off sale cannot start a new line, and a
// size the shelf cannot cover is refused before anything is written.
func (s *Service) priceLines(ctx context.Context, orderID string, inputs []SaleLineInput) ([]domain.OrderLine, int64, error) {
	lines := make([]domain.OrderLine, 0, len(inputs))
	var total int64
	for _, want := range inputs {
		item, err := s.repo.GetItem(ctx, want.ItemID)
		if err != nil {
			return nil, 0, err
		}
		stock, err := s.repo.GetStockBySize(ctx, want.ItemID, want.SizeLabel)
		if err != nil {
			return nil, 0, err
		}
		if want.Qty > stock.Available() {
			return nil, 0, glerrors.Wrap(glerrors.KindConflict,
				"no quedan suficientes "+item.Name()+" en talla "+want.SizeLabel,
				domain.ErrInsufficientStock)
		}
		// Sold, not reserved: at the counter the garment leaves as it is sold.
		line, err := item.LineFor(s.newID(), orderID, want.SizeLabel, want.Qty, domain.LineSold)
		if err != nil {
			return nil, 0, err
		}
		lines = append(lines, line)
		total += line.SubtotalCents()
	}
	return lines, total, nil
}

// recordMoney raises the charge and records the payment for a sale that has
// already happened. Every failure ends the same way: the delivery is flagged,
// because the goods are gone and no automatic answer is honest.
func (s *Service) recordMoney(ctx context.Context, saved ports.OrderAggregate, tenders []ports.Tender, total int64) {
	if len(saved.Fulfillments) == 0 {
		return
	}
	fulfillmentID := saved.Fulfillments[0].ID()

	conceptID, err := s.finance.EnsureConcept(ctx)
	if err != nil {
		s.flag(ctx, fulfillmentID, "no se pudo preparar el concepto de cobro en Finanzas")
		return
	}
	charge, err := s.finance.CreateCharge(ctx, ports.CreateChargeInput{
		StudentID:   saved.Order.StudentID(),
		CycleID:     saved.Order.CycleID(),
		ConceptID:   conceptID,
		Description: "Uniformes · " + saved.Order.Folio(),
		AmountCents: total,
		DueDate:     s.now().Format("2006-01-02"),
		// The delivery IS the reference: it makes the call idempotent and lets
		// reconciliation later ask finance about a delivery by name (ADR 0008).
		ExternalRef: externalRef(fulfillmentID),
	})
	if err != nil {
		s.flag(ctx, fulfillmentID, "la mercancía se entregó pero el cobro no quedó registrado en Finanzas")
		return
	}
	if err := s.repo.RecordCounterSaleCharge(ctx, fulfillmentID, charge.ID); err != nil {
		s.flag(ctx, fulfillmentID, "el cobro se creó pero no quedó ligado a la venta")
		return
	}
	if err := s.finance.RegisterPayment(ctx, charge.ID, total, tenders, fulfillmentID); err != nil {
		// Worse than the previous case, not better: the family now owes for
		// something they already paid for and carried out.
		s.flag(ctx, fulfillmentID, "se cobró en caja pero el pago no quedó registrado en Finanzas")
	}
}

// externalRef is how a delivery is named inside finance.
func externalRef(fulfillmentID string) string { return "uniforms:" + fulfillmentID }

// flag hands a delivery to a person. A failure to flag is the end of the line;
// there is nothing left to escalate to, so it is swallowed here and caught by
// the reconciliation sweep.
func (s *Service) flag(ctx context.Context, fulfillmentID, reason string) {
	_ = s.repo.FlagFulfillment(ctx, fulfillmentID, reason)
}

// checkShape applies the school's ceilings before anything is touched, so a
// family is told at the door rather than after keying everything in.
func checkShape(settings domain.Settings, lines []SaleLineInput) error {
	maxUnits := 0
	for _, l := range lines {
		if l.Qty > maxUnits {
			maxUnits = l.Qty
		}
	}
	return settings.CheckOrderShape(len(lines), maxUnits)
}

// --- orders and settings -----------------------------------------------------

// ListOrders returns the panel's page.
func (s *Service) ListOrders(ctx context.Context, f ports.ListOrdersFilter) ([]ports.OrderAggregate, string, int, error) {
	if !f.Level.IsValid() {
		return nil, "", 0, glerrors.New(glerrors.KindInvalid, "unknown school level")
	}
	return s.repo.ListOrders(ctx, f)
}

// GetOrder reads one order. studentIDs, when set, restricts which orders this
// caller may see AT THE SERVICE — never merely at the edge.
func (s *Service) GetOrder(ctx context.Context, id string, studentIDs []string) (ports.OrderAggregate, error) {
	if strings.TrimSpace(id) == "" {
		return ports.OrderAggregate{}, glerrors.New(glerrors.KindInvalid, "order id is required")
	}
	return s.repo.GetOrder(ctx, id, studentIDs)
}

// GetSettings reads the school's limits.
func (s *Service) GetSettings(ctx context.Context) (domain.Settings, error) {
	return s.repo.GetSettings(ctx)
}

// UpdateSettingsInput is the school's limits as they arrive from the edge: a bag
// of numbers that has not been validated yet, and that carries no tenant. The
// tenant is the service's, never the caller's — accepting one from the wire is
// how a request talks its way into another school's settings.
type UpdateSettingsInput struct {
	PickupWindowBusinessDays int
	AutoCancelEnabled        bool
	MaxExtensions            int
	MaxOpenOrdersPerStudent  int
	MaxLinesPerOrder         int
	MaxUnitsPerLine          int
	MaxLineExpiries          int
}

// UpdateSettings replaces the school's limits, validated on the way in.
func (s *Service) UpdateSettings(ctx context.Context, in UpdateSettingsInput) (domain.Settings, error) {
	next, err := domain.NewSettings(s.tenantID,
		in.PickupWindowBusinessDays, in.AutoCancelEnabled, in.MaxExtensions,
		in.MaxOpenOrdersPerStudent, in.MaxLinesPerOrder, in.MaxUnitsPerLine, in.MaxLineExpiries)
	if err != nil {
		return domain.Settings{}, err
	}
	return s.repo.UpdateSettings(ctx, next)
}
