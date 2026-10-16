package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

const (
	testTenant  = "11111111-1111-1111-1111-111111111111"
	testStudent = "22222222-2222-2222-2222-222222222222"
	testCycle   = "33333333-3333-3333-3333-333333333333"
	testActor   = "44444444-4444-4444-4444-444444444444"
)

var fixedNow = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// --- repository --------------------------------------------------------------

type fakeRepo struct {
	items    map[string]domain.Item
	stock    map[string]domain.StockLevel // keyed by id
	settings *domain.Settings
	sales    []ports.CounterSaleCommand
	byKey    map[string]ports.OrderAggregate // idempotency key -> what it produced

	// Recorded so tests can assert on the call rather than only its effect.
	chargesRecorded map[string]string
	flagged         map[string]string

	failCounterSale error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:           map[string]domain.Item{},
		stock:           map[string]domain.StockLevel{},
		byKey:           map[string]ports.OrderAggregate{},
		chargesRecorded: map[string]string{},
		flagged:         map[string]string{},
	}
}

func (r *fakeRepo) CreateItem(_ context.Context, item domain.Item) (domain.Item, error) {
	r.items[item.ID()] = item
	return item, nil
}

func (r *fakeRepo) UpdateItem(_ context.Context, item domain.Item) (domain.Item, error) {
	if _, ok := r.items[item.ID()]; !ok {
		return domain.Item{}, glerrors.ErrNotFound
	}
	r.items[item.ID()] = item
	return item, nil
}

func (r *fakeRepo) GetItem(_ context.Context, id string) (domain.Item, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.Item{}, glerrors.ErrNotFound
	}
	return item, nil
}

func (r *fakeRepo) ListItems(_ context.Context, f ports.ListItemsFilter) ([]ports.ItemWithSizes, error) {
	var out []ports.ItemWithSizes
	for _, item := range r.items {
		if !item.Active() && !f.IncludeInactive {
			continue
		}
		if f.Level != domain.LevelAny && !item.AppliesTo(f.Level) {
			continue
		}
		out = append(out, ports.ItemWithSizes{Item: item, Sizes: r.sizesOf(item.ID())})
	}
	return out, nil
}

func (r *fakeRepo) sizesOf(itemID string) []domain.StockLevel {
	var out []domain.StockLevel
	for _, s := range r.stock {
		if s.ItemID() == itemID {
			out = append(out, s)
		}
	}
	return out
}

func (r *fakeRepo) UpsertSizes(_ context.Context, itemID string, specs []ports.SizeSpec) (int, int, []domain.StockLevel, error) {
	if _, ok := r.items[itemID]; !ok {
		return 0, 0, nil, glerrors.ErrNotFound
	}
	var created, updated int
	for _, spec := range specs {
		if existing, ok := r.stockBySize(itemID, spec.SizeLabel); ok {
			next, err := domain.NewStockLevel(existing.ID(), testTenant, itemID, spec.SizeLabel,
				spec.SizeOrder, existing.OnHand(), existing.Reserved(), spec.MinStock)
			if err != nil {
				return 0, 0, nil, err
			}
			r.stock[existing.ID()] = next
			updated++
			continue
		}
		id := fmt.Sprintf("stk-%s-%s", itemID, spec.SizeLabel)
		s, err := domain.NewStockLevel(id, testTenant, itemID, spec.SizeLabel, spec.SizeOrder, 0, 0, spec.MinStock)
		if err != nil {
			return 0, 0, nil, err
		}
		r.stock[id] = s
		created++
	}
	return created, updated, r.sizesOf(itemID), nil
}

func (r *fakeRepo) stockBySize(itemID, size string) (domain.StockLevel, bool) {
	for _, s := range r.stock {
		if s.ItemID() == itemID && s.SizeLabel() == size {
			return s, true
		}
	}
	return domain.StockLevel{}, false
}

func (r *fakeRepo) ListStock(_ context.Context, itemID string, onlyBelow bool) ([]domain.StockLevel, error) {
	var out []domain.StockLevel
	for _, s := range r.stock {
		if itemID != "" && s.ItemID() != itemID {
			continue
		}
		if onlyBelow && !s.BelowMinimum() {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeRepo) GetStockBySize(_ context.Context, itemID, size string) (domain.StockLevel, error) {
	s, ok := r.stockBySize(itemID, size)
	if !ok {
		return domain.StockLevel{}, glerrors.ErrNotFound
	}
	return s, nil
}

func (r *fakeRepo) ReceiveStock(_ context.Context, lines []ports.ReceiveLine, _, _ string) ([]domain.StockLevel, []string, error) {
	var out []domain.StockLevel
	for _, l := range lines {
		s, ok := r.stock[l.StockID]
		if !ok {
			return nil, nil, glerrors.ErrNotFound
		}
		next, err := s.Receive(l.Qty)
		if err != nil {
			return nil, nil, err
		}
		r.stock[l.StockID] = next
		out = append(out, next)
	}
	return out, nil, nil
}

func (r *fakeRepo) AdjustStock(_ context.Context, stockID string, delta int, _, _ string) (domain.StockLevel, error) {
	s, ok := r.stock[stockID]
	if !ok {
		return domain.StockLevel{}, glerrors.ErrNotFound
	}
	next, err := s.Adjust(delta)
	if err != nil {
		return domain.StockLevel{}, err
	}
	r.stock[stockID] = next
	return next, nil
}

func (r *fakeRepo) CounterSale(_ context.Context, cmd ports.CounterSaleCommand) (ports.OrderAggregate, bool, error) {
	if r.failCounterSale != nil {
		return ports.OrderAggregate{}, false, r.failCounterSale
	}
	if prior, ok := r.byKey[cmd.IdempotencyKey]; ok && cmd.IdempotencyKey != "" {
		return prior, true, nil
	}
	if cmd.Admit != nil {
		if err := cmd.Admit(); err != nil {
			return ports.OrderAggregate{}, false, err
		}
	}
	// The order is built BEFORE anything moves, in the same order as the real
	// transaction. It matters: Postgres rolls the whole thing back when the order
	// is refused, and a fake that had already emptied the shelf by then would let
	// a refused sale look like it moved goods.
	order, err := domain.NewOrder(cmd.OrderID, cmd.TenantID, domain.FormatFolio(cmd.FolioYear, len(r.sales)+1),
		domain.OrderStudent{
			ID: cmd.StudentID, FullName: cmd.StudentName, Level: cmd.StudentLevel,
			GradeGroup: cmd.StudentGradeGroup, CycleID: cmd.CycleID,
		}, domain.SourceCounter, cmd.ActorUserID, cmd.SoldAt)
	if err != nil {
		return ports.OrderAggregate{}, false, err
	}
	ful, err := domain.NewCounterSaleFulfillment(cmd.FulfillmentID, cmd.TenantID, cmd.OrderID, 1, cmd.TotalCents, cmd.SoldAt)
	if err != nil {
		return ports.OrderAggregate{}, false, err
	}
	// Deduct exactly as the real transaction does, so a test that oversells the
	// shelf fails here rather than only against Postgres.
	for _, line := range cmd.Lines {
		s, ok := r.stockBySize(line.ItemID(), line.SizeLabel())
		if !ok {
			return ports.OrderAggregate{}, false, glerrors.ErrNotFound
		}
		next, err := s.SellFromShelf(line.Qty())
		if err != nil {
			return ports.OrderAggregate{}, false, err
		}
		r.stock[s.ID()] = next
	}
	agg := ports.OrderAggregate{Order: order, Lines: cmd.Lines, Fulfillments: []domain.Fulfillment{ful}}
	r.sales = append(r.sales, cmd)
	if cmd.IdempotencyKey != "" {
		r.byKey[cmd.IdempotencyKey] = agg
	}
	return agg, false, nil
}

func (r *fakeRepo) RecordCounterSaleCharge(_ context.Context, fulfillmentID, chargeID string) error {
	r.chargesRecorded[fulfillmentID] = chargeID
	return nil
}

func (r *fakeRepo) FlagFulfillment(_ context.Context, fulfillmentID, reason string) error {
	r.flagged[fulfillmentID] = reason
	return nil
}

func (r *fakeRepo) ListOrders(_ context.Context, _ ports.ListOrdersFilter) ([]ports.OrderAggregate, string, int, error) {
	return nil, "", 0, nil
}

func (r *fakeRepo) GetOrder(_ context.Context, _ string, _ []string) (ports.OrderAggregate, error) {
	return ports.OrderAggregate{}, glerrors.ErrNotFound
}

func (r *fakeRepo) GetSettings(_ context.Context) (domain.Settings, error) {
	if r.settings == nil {
		return domain.DefaultSettings(testTenant), nil
	}
	return *r.settings, nil
}

func (r *fakeRepo) UpdateSettings(_ context.Context, s domain.Settings) (domain.Settings, error) {
	r.settings = &s
	return s, nil
}

// --- students ----------------------------------------------------------------

type fakeStudents struct {
	byID map[string]ports.StudentRef
}

func (f *fakeStudents) ByID(_ context.Context, id string) (ports.StudentRef, error) {
	s, ok := f.byID[id]
	if !ok {
		return ports.StudentRef{}, glerrors.ErrNotFound
	}
	return s, nil
}

// --- finance -----------------------------------------------------------------

type fakeFinance struct {
	conceptID string
	charges   []ports.CreateChargeInput
	payments  []string
	// tenders are the methods of each recorded payment, in order (T8).
	tenders [][]ports.Tender
	// Injected failures, one per step, so a test can kill the call exactly where
	// it wants to see what the system is left holding.
	failConcept error
	failCharge  error
	failPayment error
	nextID      int
}

func (f *fakeFinance) EnsureConcept(context.Context) (string, error) {
	if f.failConcept != nil {
		return "", f.failConcept
	}
	if f.conceptID == "" {
		f.conceptID = "concept-uniformes"
	}
	return f.conceptID, nil
}

func (f *fakeFinance) CreateCharge(_ context.Context, in ports.CreateChargeInput) (ports.ChargeRef, error) {
	if f.failCharge != nil {
		return ports.ChargeRef{}, f.failCharge
	}
	// Mirror the real idempotency: the same external ref returns the original.
	for i, prior := range f.charges {
		if prior.ExternalRef == in.ExternalRef {
			return ports.ChargeRef{ID: fmt.Sprintf("chg-%d", i+1)}, nil
		}
	}
	f.charges = append(f.charges, in)
	f.nextID++
	return ports.ChargeRef{ID: fmt.Sprintf("chg-%d", f.nextID)}, nil
}

func (f *fakeFinance) RegisterPayment(_ context.Context, chargeID string, _ int64, tenders []ports.Tender, _ string) error {
	if f.failPayment != nil {
		return f.failPayment
	}
	f.payments = append(f.payments, chargeID)
	f.tenders = append(f.tenders, tenders)
	return nil
}

func (f *fakeFinance) CancelChargeIfUnpaid(context.Context, string) error { return nil }

// --- events ------------------------------------------------------------------

type spyPublisher struct{ events []ports.Event }

func (s *spyPublisher) Publish(_ context.Context, e ports.Event) error {
	s.events = append(s.events, e)
	return nil
}

// --- fixture -----------------------------------------------------------------

type fixture struct {
	svc      *usecase.Service
	repo     *fakeRepo
	students *fakeStudents
	finance  *fakeFinance
	pub      *spyPublisher
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo: newFakeRepo(),
		students: &fakeStudents{byID: map[string]ports.StudentRef{
			testStudent: {
				ID: testStudent, FullName: "Zoe Villalobos", Level: domain.Level("c1000000-0000-0000-0000-000000000002"),
				GradeGroup: "3 B", CycleID: testCycle, Status: "enrolled",
			},
		}},
		finance: &fakeFinance{},
		pub:     &spyPublisher{},
	}
	f.svc = usecase.NewService(f.repo, f.students, f.finance, f.pub, testTenant,
		seqID("id"), func() time.Time { return fixedNow }, time.UTC)
	return f
}

// seedItem puts a garment on sale with one size in stock.
func (f *fixture) seedItem(t *testing.T, name, size string, price int64, onHand int) domain.Item {
	t.Helper()
	item, err := f.svc.CreateItem(context.Background(), usecase.CreateItemInput{
		Name: name, PriceCents: price,
	})
	if err != nil {
		t.Fatalf("seed item %q: %v", name, err)
	}
	if _, _, _, err := f.svc.UpsertSizes(context.Background(), item.ID(),
		[]ports.SizeSpec{{SizeLabel: size, SizeOrder: 1}}); err != nil {
		t.Fatalf("seed size: %v", err)
	}
	stock, err := f.repo.GetStockBySize(context.Background(), item.ID(), size)
	if err != nil {
		t.Fatalf("read seeded stock: %v", err)
	}
	if onHand > 0 {
		if _, _, err := f.repo.ReceiveStock(context.Background(),
			[]ports.ReceiveLine{{StockID: stock.ID(), Qty: onHand}}, "seed", testActor); err != nil {
			t.Fatalf("seed stock: %v", err)
		}
	}
	return item
}

func seqID(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s-%d", prefix, n)
	}
}

func requireKind(t *testing.T, err error, want glerrors.Kind) {
	t.Helper()
	if got := glerrors.KindOf(err); got != want {
		t.Fatalf("kind = %v, want %v (err: %v)", got, want, err)
	}
}
