package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.Repository = (*Repository)(nil)

const (
	uniqueViolation = "23505"
	checkViolation  = "23514"
	defaultPageSize = 20
	maxPageSize     = 100
	maxListLimit    = 500
	newIDExpr       = "gen_random_uuid()"
)

// Repository is the uniform shop's GORM-backed store. Every operation runs
// inside a transaction with app.current_tenant bound, so Row Level Security
// applies to reads AND writes (ADR 0001). The tenant is fixed at construction,
// as in every other service in this phase.
type Repository struct {
	db       *gorm.DB
	tenantID string
	now      func() time.Time
}

// NewRepository builds the repository scoped to tenantID. now is injected so the
// few places that stamp a row are as deterministic in tests as the domain is.
func NewRepository(db *gorm.DB, tenantID string, now func() time.Time) *Repository {
	return &Repository{db: db, tenantID: tenantID, now: now}
}

// withTenant runs fn in a transaction that has bound app.current_tenant,
// delegating to the shared primitive so tenant binding lives in exactly one
// place across every service.
func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	})
}

func isViolation(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "reading", err)
}

// --- catalogue ---------------------------------------------------------------

// CreateItem stores a garment.
func (r *Repository) CreateItem(ctx context.Context, item domain.Item) (domain.Item, error) {
	model := fromDomainItem(item)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "creating garment", err)
		}
		return nil
	})
	if err != nil {
		return domain.Item{}, err
	}
	return toDomainItem(model)
}

// UpdateItem persists the editable fields of a garment.
func (r *Repository) UpdateItem(ctx context.Context, item domain.Item) (domain.Item, error) {
	model := fromDomainItem(item)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&itemModel{}).Where("id = ?", item.ID()).Updates(map[string]any{
			"name":        model.Name,
			"description": model.Description,
			"level_id":    model.LevelID,
			"price_cents": model.PriceCents,
			"active":      model.Active,
			"updated_at":  gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating garment", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		// Saving any level but "unresolved" is an explicit re-scope: it clears
		// the old SEP key that marks an unresolved garment. The model maps that
		// column read-only, so the write is its own statement.
		if item.Level() != domain.LevelUnresolved {
			if err := tx.Exec("UPDATE uniform_items SET level = NULL WHERE id = ?", item.ID()).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "clearing the garment's old level", err)
			}
		}
		return nil
	})
	if err != nil {
		return domain.Item{}, err
	}
	saved, err := toDomainItem(model)
	if err != nil {
		return domain.Item{}, err
	}
	// The row model carries no old key, so an unresolved garment would come
	// back as every level — the state a client might send back next time.
	if item.Level() == domain.LevelUnresolved {
		saved = saved.WithUnresolvedLevel()
	}
	return saved, nil
}

// GetItem loads one garment.
func (r *Repository) GetItem(ctx context.Context, id string) (domain.Item, error) {
	var out domain.Item
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m itemModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		item, err := toDomainItem(m)
		if err != nil {
			return err
		}
		out = item
		return nil
	})
	return out, err
}

// ListItems returns the catalogue with every garment's size run, resolved in TWO
// queries rather than one per garment. Level narrows to what a pupil of that
// level wears: the garments scoped to it PLUS the ones scoped to none.
func (r *Repository) ListItems(ctx context.Context, f ports.ListItemsFilter) ([]ports.ItemWithSizes, error) {
	var out []ports.ItemWithSizes
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&itemModel{}).Order("name ASC, id ASC").Limit(maxListLimit)
		if !f.IncludeInactive {
			q = q.Where("active")
		}
		if f.Level != domain.LevelAny {
			// "Every level" is level_id NULL with no old key; a NULL level_id
			// beside an old key is an unresolved garment, offered to nobody.
			q = q.Where("(level_id IS NULL AND level IS NULL) OR level_id = ?", string(f.Level))
		}
		var items []itemModel
		if err := q.Find(&items).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing garments", err)
		}
		if len(items) == 0 {
			return nil
		}

		ids := make([]string, 0, len(items))
		for _, m := range items {
			ids = append(ids, m.ID)
		}
		var sizes []stockModel
		if err := tx.Where("item_id IN ?", ids).
			Order("item_id ASC, size_order ASC, size_label ASC").Find(&sizes).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing sizes", err)
		}
		byItem := make(map[string][]domain.StockLevel, len(items))
		for _, s := range sizes {
			level, err := toDomainStock(s)
			if err != nil {
				return err
			}
			byItem[s.ItemID] = append(byItem[s.ItemID], level)
		}

		out = make([]ports.ItemWithSizes, 0, len(items))
		for _, m := range items {
			item, err := toDomainItem(m)
			if err != nil {
				return err
			}
			out = append(out, ports.ItemWithSizes{Item: item, Sizes: byItem[m.ID]})
		}
		return nil
	})
	return out, err
}

// UpsertSizes writes a garment's size run, reporting created and updated
// separately so the panel can say what actually happened instead of upserting in
// silence. Existing counts are never touched: a size run is the SHAPE of the
// catalogue, and stock moves only through receipts, sales and adjustments, each
// of which leaves a ledger entry.
func (r *Repository) UpsertSizes(ctx context.Context, itemID string, specs []ports.SizeSpec) (int, int, []domain.StockLevel, error) {
	var created, updated int
	var out []domain.StockLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var item itemModel
		if err := tx.Where("id = ?", itemID).First(&item).Error; err != nil {
			return translateFindErr(err)
		}
		for _, spec := range specs {
			res := tx.Model(&stockModel{}).
				Where("item_id = ? AND size_label = ?", itemID, spec.SizeLabel).
				Updates(map[string]any{
					"size_order": spec.SizeOrder,
					"min_stock":  spec.MinStock,
					"updated_at": gorm.Expr("now()"),
				})
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInternal, "updating size", res.Error)
			}
			if res.RowsAffected > 0 {
				updated++
				continue
			}
			row := stockModel{
				TenantID: r.tenantID, ItemID: itemID, SizeLabel: spec.SizeLabel,
				SizeOrder: spec.SizeOrder, MinStock: spec.MinStock,
			}
			if err := tx.Omit("id", "created_at", "updated_at").Create(&row).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "creating size", err)
			}
			created++
		}
		var rows []stockModel
		if err := tx.Where("item_id = ?", itemID).
			Order("size_order ASC, size_label ASC").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading sizes", err)
		}
		levels, err := toDomainStockAll(rows)
		if err != nil {
			return err
		}
		out = levels
		return nil
	})
	if err != nil {
		return 0, 0, nil, err
	}
	return created, updated, out, nil
}

// --- inventory ---------------------------------------------------------------

// ListStock returns stock by garment and size.
func (r *Repository) ListStock(ctx context.Context, itemID string, onlyBelowMinimum bool) ([]domain.StockLevel, error) {
	var out []domain.StockLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&stockModel{}).Order("item_id ASC, size_order ASC, size_label ASC").Limit(maxListLimit)
		if itemID != "" {
			q = q.Where("item_id = ?", itemID)
		}
		if onlyBelowMinimum {
			// Measured on what can still be sold, not what is on the shelf:
			// five pieces all spoken for is an empty shelf to the next parent.
			q = q.Where("on_hand - reserved <= min_stock")
		}
		var rows []stockModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing stock", err)
		}
		levels, err := toDomainStockAll(rows)
		if err != nil {
			return err
		}
		out = levels
		return nil
	})
	return out, err
}

// GetStockBySize loads one size's position.
func (r *Repository) GetStockBySize(ctx context.Context, itemID, sizeLabel string) (domain.StockLevel, error) {
	var out domain.StockLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m stockModel
		if err := tx.Where("item_id = ? AND size_label = ?", itemID, sizeLabel).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		level, err := toDomainStock(m)
		if err != nil {
			return err
		}
		out = level
		return nil
	})
	return out, err
}

// ReceiveStock books a supplier delivery and allocates it to waiting lines in the
// SAME transaction. Splitting the two would let a counter sale take, in between,
// the piece a family has been waiting three weeks for.
func (r *Repository) ReceiveStock(ctx context.Context, lines []ports.ReceiveLine, note, actorUserID string) ([]domain.StockLevel, []string, error) {
	var out []domain.StockLevel
	var allocated []string
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, line := range lines {
			if err := tx.Model(&stockModel{}).Where("id = ?", line.StockID).Updates(map[string]any{
				"on_hand":    gorm.Expr("on_hand + ?", line.Qty),
				"updated_at": gorm.Expr("now()"),
			}).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "receiving stock", err)
			}
			if err := r.appendMove(tx, line.StockID, "", "receive", line.Qty, note, actorUserID); err != nil {
				return err
			}
			ids, err := r.allocateWaitingLines(tx, line.StockID)
			if err != nil {
				return err
			}
			allocated = append(allocated, ids...)
		}

		ids := make([]string, 0, len(lines))
		for _, line := range lines {
			ids = append(ids, line.StockID)
		}
		var rows []stockModel
		if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading stock", err)
		}
		if len(rows) != len(lines) {
			return glerrors.ErrNotFound
		}
		levels, err := toDomainStockAll(rows)
		if err != nil {
			return err
		}
		out = levels
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, allocated, nil
}

// allocateWaitingLines hands newly arrived stock to the lines that have been
// waiting for it, OLDEST FIRST. The order is the fairness rule of the whole
// shop: a family that ordered three weeks ago gets the piece before a family
// that ordered yesterday, and before the counter can sell it.
//
// It stops at the first line it cannot cover rather than skipping ahead to a
// smaller one, because skipping is precisely how the oldest order never gets
// filled.
func (r *Repository) allocateWaitingLines(tx *gorm.DB, stockID string) ([]string, error) {
	var stock stockModel
	if err := tx.Where("id = ?", stockID).First(&stock).Error; err != nil {
		return nil, translateFindErr(err)
	}
	var waiting []orderLineModel
	if err := tx.Where("item_id = ? AND size_label = ? AND status = ?",
		stock.ItemID, stock.SizeLabel, string(domain.LineAwaitingStock)).
		Order("created_at ASC, id ASC").Limit(maxListLimit).Find(&waiting).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the waiting list", err)
	}

	var allocated []string
	for _, line := range waiting {
		res := tx.Model(&stockModel{}).
			Where("id = ? AND on_hand - reserved >= ?", stockID, line.Qty).
			Updates(map[string]any{
				"reserved":   gorm.Expr("reserved + ?", line.Qty),
				"updated_at": gorm.Expr("now()"),
			})
		if res.Error != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "reserving for a waiting line", res.Error)
		}
		if res.RowsAffected == 0 {
			// Nothing left for this one, so nothing is left for the ones behind
			// it either. Stopping here is what keeps the queue a queue.
			break
		}
		if err := tx.Model(&orderLineModel{}).Where("id = ?", line.ID).Updates(map[string]any{
			"status":     string(domain.LineReserved),
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "reserving a waiting line", err)
		}
		if err := r.appendMove(tx, stockID, line.ID, "reserve", line.Qty, "surtido de pedido en espera", ""); err != nil {
			return nil, err
		}
		allocated = append(allocated, line.ID)
	}
	return allocated, nil
}

// AdjustStock corrects a count. The check that it cannot fall below what is held
// is expressed in the WHERE clause, so the decision is made by the database in
// one statement rather than read-then-write across two.
func (r *Repository) AdjustStock(ctx context.Context, stockID string, delta int, note, actorUserID string) (domain.StockLevel, error) {
	var out domain.StockLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&stockModel{}).
			Where("id = ? AND on_hand + ? >= reserved AND on_hand + ? >= 0", stockID, delta, delta).
			Updates(map[string]any{
				"on_hand":    gorm.Expr("on_hand + ?", delta),
				"updated_at": gorm.Expr("now()"),
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "adjusting stock", res.Error)
		}
		if res.RowsAffected == 0 {
			// The size does not exist, or the correction would go below zero,
			// or it would strip a family's hold. Tell them apart, because the
			// fixes differ.
			var current stockModel
			if err := tx.Where("id = ?", stockID).First(&current).Error; err != nil {
				return translateFindErr(err)
			}
			return adjustRefusal(current, delta)
		}
		if err := r.appendMove(tx, stockID, "", "adjust", delta, note, actorUserID); err != nil {
			return err
		}
		var m stockModel
		if err := tx.Where("id = ?", stockID).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		level, err := toDomainStock(m)
		if err != nil {
			return err
		}
		out = level
		return nil
	})
	return out, err
}

// adjustRefusal explains why the conditional UPDATE refused a correction by
// replaying it on the row as it stands: the domain names the cause (below zero
// versus below what families hold). When the row would now accept it, a
// concurrent write moved the count in between; the refusal stands as a hold.
func adjustRefusal(current stockModel, delta int) error {
	level, err := toDomainStock(current)
	if err != nil {
		return err
	}
	if _, err := level.Adjust(delta); err != nil {
		return err
	}
	return glerrors.New(glerrors.KindConflict, "an adjustment cannot take the count below what is held")
}

// appendMove writes one entry in the append-only stock ledger. Every movement of
// every unit goes through here, because a physical count months from now can only
// be audited against a record nobody was able to rewrite.
func (r *Repository) appendMove(tx *gorm.DB, stockID, orderLineID, kind string, qty int, note, actorUserID string) error {
	move := stockMoveModel{
		TenantID: r.tenantID, StockID: stockID, OrderLineID: nullable(orderLineID),
		Kind: kind, Qty: qty, ActorUserID: nullable(actorUserID), Note: note,
	}
	if err := tx.Omit("id", "created_at").Create(&move).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "writing the stock ledger", err)
	}
	return nil
}

// --- the counter sale --------------------------------------------------------

// CounterSale writes a completed sale as ONE transaction: the folio, the order,
// its lines, the terminal delivery, the stock deduction and the ledger entries.
//
// The deduction is a CONDITIONAL UPDATE rather than a read, a decision and a
// write:
//
//	UPDATE uniform_stock SET on_hand = on_hand - $1
//	 WHERE id = $2 AND on_hand - reserved >= $1
//
// One statement, decided by the database, and the answer is RowsAffected. It
// needs no explicit row lock, which matters for more than elegance: a multi-line
// sale takes several stock rows, and taking them under FOR UPDATE without a
// deterministic order is how two tills deadlock during the back-to-school rush.
// It also keeps the counter from selling into a hold — the predicate says
// available, not on hand.
func (r *Repository) CounterSale(ctx context.Context, cmd ports.CounterSaleCommand) (ports.OrderAggregate, bool, error) {
	var out ports.OrderAggregate
	var replayed bool
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		// A retry of a sale that already happened returns what it produced, so a
		// double click at the till cannot sell the same garment twice.
		if cmd.IdempotencyKey != "" {
			prior, found, err := r.saleByIdempotencyKey(tx, cmd.IdempotencyKey)
			if err != nil {
				return err
			}
			if found {
				out, replayed = prior, true
				return nil
			}
		}
		if cmd.Admit != nil {
			if err := cmd.Admit(); err != nil {
				return err
			}
		}

		folio, err := r.nextFolio(tx, cmd.FolioYear)
		if err != nil {
			return err
		}
		order, err := domain.NewOrder(cmd.OrderID, cmd.TenantID, folio, domain.OrderStudent{
			ID: cmd.StudentID, FullName: cmd.StudentName, Level: cmd.StudentLevel, LevelName: cmd.StudentLevelName,
			GradeGroup: cmd.StudentGradeGroup, CycleID: cmd.CycleID,
		}, domain.SourceCounter, cmd.ActorUserID, cmd.SoldAt)
		if err != nil {
			return err
		}
		// Sold and handed over in one act, so it is closed the moment it opens.
		order = order.Close(cmd.SoldAt)
		orderRow := fromDomainOrder(order)
		if err := tx.Create(&orderRow).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "writing the sale", err)
		}

		fulfillment, err := domain.NewCounterSaleFulfillment(cmd.FulfillmentID, cmd.TenantID,
			cmd.OrderID, 1, cmd.TotalCents, cmd.SoldAt)
		if err != nil {
			return err
		}
		fulRow := fromDomainFulfillment(fulfillment)
		if err := tx.Create(&fulRow).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "writing the delivery", err)
		}

		for _, line := range cmd.Lines {
			assigned, err := line.AssignTo(cmd.FulfillmentID)
			if err != nil {
				return err
			}
			lineRow := fromDomainLine(assigned)
			if err := tx.Create(&lineRow).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "writing a sale line", err)
			}
			if err := r.sellFromShelf(tx, assigned, cmd.Note, cmd.ActorUserID); err != nil {
				return err
			}
		}

		if cmd.IdempotencyKey != "" {
			if err := r.rememberSale(tx, cmd.IdempotencyKey, cmd.OrderID); err != nil {
				return err
			}
		}
		out = ports.OrderAggregate{Order: order, Lines: cmd.Lines, Fulfillments: []domain.Fulfillment{fulfillment}}
		return nil
	})
	if err != nil {
		return ports.OrderAggregate{}, false, err
	}
	return out, replayed, nil
}

// sellFromShelf takes units the counter is handing over. The predicate is
// `on_hand - reserved >= qty`, so it can never spend a hold that belongs to a
// family who is waiting.
func (r *Repository) sellFromShelf(tx *gorm.DB, line domain.OrderLine, note, actorUserID string) error {
	var stock stockModel
	if err := tx.Where("item_id = ? AND size_label = ?", line.ItemID(), line.SizeLabel()).
		First(&stock).Error; err != nil {
		return translateFindErr(err)
	}
	res := tx.Model(&stockModel{}).
		Where("id = ? AND on_hand - reserved >= ?", stock.ID, line.Qty()).
		Updates(map[string]any{
			"on_hand":    gorm.Expr("on_hand - ?", line.Qty()),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		if isViolation(res.Error, checkViolation) {
			return glerrors.Wrap(glerrors.KindConflict, "no quedan suficientes piezas de esa talla", res.Error)
		}
		return glerrors.Wrap(glerrors.KindInternal, "deducting stock", res.Error)
	}
	if res.RowsAffected == 0 {
		// Carries the sentinel so the edge can say "that size just ran out"
		// rather than the sentence it shows for any other conflict.
		return glerrors.Wrap(glerrors.KindConflict,
			"no quedan suficientes "+line.ItemName()+" en talla "+line.SizeLabel(),
			domain.ErrInsufficientStock)
	}
	return r.appendMove(tx, stock.ID, line.ID(), "sell", -line.Qty(), note, actorUserID)
}

// nextFolio allocates the school's next order number for the year.
//
// A Postgres SEQUENCE would be per database and would ignore RLS, so two schools
// would share one run of numbers and each could infer the other's volume from the
// gaps. A counter row bumped with UPDATE ... RETURNING is per school, tenant
// -isolated like everything else, and serialises on its own row.
func (r *Repository) nextFolio(tx *gorm.DB, year int) (string, error) {
	var seq int
	err := tx.Raw(`INSERT INTO uniform_folio_counters (tenant_id, year, next_seq)
	               VALUES (?, ?, 2)
	               ON CONFLICT (tenant_id, year)
	               DO UPDATE SET next_seq = uniform_folio_counters.next_seq + 1
	               RETURNING next_seq - 1`, r.tenantID, year).Scan(&seq).Error
	if err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "allocating a folio", err)
	}
	return domain.FormatFolio(year, seq), nil
}

// saleIdempotencyModel remembers which sale an idempotency key produced.
type saleIdempotencyModel struct {
	TenantID       string    `gorm:"column:tenant_id;primaryKey"`
	IdempotencyKey string    `gorm:"column:idempotency_key;primaryKey"`
	OrderID        string    `gorm:"column:order_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (saleIdempotencyModel) TableName() string { return "uniform_sale_keys" }

func (r *Repository) saleByIdempotencyKey(tx *gorm.DB, key string) (ports.OrderAggregate, bool, error) {
	var row saleIdempotencyModel
	err := tx.Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ports.OrderAggregate{}, false, nil
	}
	if err != nil {
		return ports.OrderAggregate{}, false, glerrors.Wrap(glerrors.KindInternal, "checking for a replay", err)
	}
	agg, err := r.loadAggregate(tx, row.OrderID)
	if err != nil {
		return ports.OrderAggregate{}, false, err
	}
	return agg, true, nil
}

func (r *Repository) rememberSale(tx *gorm.DB, key, orderID string) error {
	row := saleIdempotencyModel{TenantID: r.tenantID, IdempotencyKey: key, OrderID: orderID}
	if err := tx.Omit("created_at").Create(&row).Error; err != nil {
		if isViolation(err, uniqueViolation) {
			// Two tills pressed the same key at once. The one that lost is a
			// replay, not a second sale.
			return glerrors.Wrap(glerrors.KindConflict, "this sale was already registered", err)
		}
		return glerrors.Wrap(glerrors.KindInternal, "recording the sale key", err)
	}
	return nil
}

// RecordCounterSaleCharge attaches the finance charge to a sale already written.
func (r *Repository) RecordCounterSaleCharge(ctx context.Context, fulfillmentID, chargeID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&fulfillmentModel{}).Where("id = ?", fulfillmentID).Updates(map[string]any{
			"finance_charge_id": chargeID,
			"updated_at":        gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "linking the charge", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// FlagFulfillment hands a delivery to a person, with the reason on it.
func (r *Repository) FlagFulfillment(ctx context.Context, fulfillmentID, reason string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&fulfillmentModel{}).Where("id = ?", fulfillmentID).Updates(map[string]any{
			"requires_attention": true,
			"attention_reason":   reason,
			"updated_at":         gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "flagging the delivery", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// --- reads -------------------------------------------------------------------

// ListOrders is the panel's page.
//
// Ordering always breaks ties on id. Without that, paging is not stable: the
// expiry sweep mutates rows underneath a staff member who is on page three, and
// two rows sharing a timestamp can swap places between requests, so a row is
// shown twice and another is never shown at all.
func (r *Repository) ListOrders(ctx context.Context, f ports.ListOrdersFilter) ([]ports.OrderAggregate, string, int, error) {
	var out []ports.OrderAggregate
	var total int
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&orderModel{})
		if len(f.StudentIDs) > 0 {
			q = q.Where("student_id IN ?", f.StudentIDs)
		}
		if f.Level != domain.LevelAny {
			q = q.Where("student_level_id = ?", string(f.Level))
		}
		if term := strings.TrimSpace(f.Query); term != "" {
			if domain.LooksLikeFolio(term) {
				q = q.Where("folio = ?", strings.ToUpper(folioOf(term)))
			} else {
				// Wildcards in the user's text are escaped, not interpreted: a
				// search for "100%" is a search for "100%".
				q = q.Where(`student_name ILIKE ? ESCAPE '\'`, "%"+escapeLike(term)+"%")
			}
		}
		if !f.From.IsZero() {
			q = q.Where("created_at >= ?", f.From)
		}
		if !f.To.IsZero() {
			q = q.Where("created_at < ?", f.To.AddDate(0, 0, 1))
		}
		if len(f.Statuses) > 0 || f.OnlyRequiresAttention {
			sub := tx.Model(&fulfillmentModel{}).Select("order_id")
			if len(f.Statuses) > 0 {
				statuses := make([]string, 0, len(f.Statuses))
				for _, s := range f.Statuses {
					statuses = append(statuses, string(s))
				}
				sub = sub.Where("status IN ?", statuses)
			}
			if f.OnlyRequiresAttention {
				sub = sub.Where("requires_attention")
			}
			q = q.Where("id IN (?)", sub)
		}

		var count int64
		if err := q.Count(&count).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting orders", err)
		}
		total = int(count)

		size := f.PageSize
		if size <= 0 {
			size = defaultPageSize
		}
		if size > maxPageSize {
			size = maxPageSize
		}
		direction := "ASC"
		if f.Descending {
			direction = "DESC"
		}
		var rows []orderModel
		if err := q.Order(fmt.Sprintf("created_at %s, id %s", direction, direction)).
			Limit(size).Offset(offsetOf(f.PageToken)).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing orders", err)
		}
		for _, row := range rows {
			agg, err := r.loadAggregate(tx, row.ID)
			if err != nil {
				return err
			}
			out = append(out, agg)
		}
		return nil
	})
	if err != nil {
		return nil, "", 0, err
	}
	next := ""
	if len(out) == pageSizeOf(f) {
		next = fmt.Sprint(offsetOf(f.PageToken) + len(out))
	}
	return out, next, total, nil
}

// GetOrder reads one order. studentIDs, when set, restricts what this caller may
// see AT THE SERVICE. An order outside that set is reported as NOT FOUND rather
// than forbidden: a 403 on somebody else's id confirms that it exists, which is
// half of what an attacker wanted.
func (r *Repository) GetOrder(ctx context.Context, id string, studentIDs []string) (ports.OrderAggregate, error) {
	var out ports.OrderAggregate
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&orderModel{}).Where("id = ?", id)
		if len(studentIDs) > 0 {
			q = q.Where("student_id IN ?", studentIDs)
		}
		var row orderModel
		if err := q.First(&row).Error; err != nil {
			return translateFindErr(err)
		}
		agg, err := r.loadAggregate(tx, row.ID)
		if err != nil {
			return err
		}
		out = agg
		return nil
	})
	return out, err
}

// loadAggregate reads an order with its lines and deliveries. A panel row is
// meaningless without them: the order has no status of its own, and its label is
// derived from exactly these.
func (r *Repository) loadAggregate(tx *gorm.DB, orderID string) (ports.OrderAggregate, error) {
	var orderRow orderModel
	if err := tx.Where("id = ?", orderID).First(&orderRow).Error; err != nil {
		return ports.OrderAggregate{}, translateFindErr(err)
	}
	order, err := toDomainOrder(orderRow)
	if err != nil {
		return ports.OrderAggregate{}, err
	}

	var lineRows []orderLineModel
	if err := tx.Where("order_id = ?", orderID).Order("created_at ASC, id ASC").
		Find(&lineRows).Error; err != nil {
		return ports.OrderAggregate{}, glerrors.Wrap(glerrors.KindInternal, "reading order lines", err)
	}
	lines := make([]domain.OrderLine, 0, len(lineRows))
	for _, m := range lineRows {
		line, err := toDomainLine(m)
		if err != nil {
			return ports.OrderAggregate{}, err
		}
		lines = append(lines, line)
	}

	var fulRows []fulfillmentModel
	if err := tx.Where("order_id = ?", orderID).Order("seq ASC").Find(&fulRows).Error; err != nil {
		return ports.OrderAggregate{}, glerrors.Wrap(glerrors.KindInternal, "reading deliveries", err)
	}
	fulfillments := make([]domain.Fulfillment, 0, len(fulRows))
	for _, m := range fulRows {
		f, err := toDomainFulfillment(m)
		if err != nil {
			return ports.OrderAggregate{}, err
		}
		fulfillments = append(fulfillments, f)
	}

	return ports.OrderAggregate{Order: order, Lines: lines, Fulfillments: fulfillments}, nil
}

// --- settings ----------------------------------------------------------------

// GetSettings reads the school's limits, creating the defaults the first time.
func (r *Repository) GetSettings(ctx context.Context) (domain.Settings, error) {
	var out domain.Settings
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m settingsModel
		err := tx.Where("tenant_id = ?", r.tenantID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A school that has never opened the settings screen still gets a
			// working shop.
			out = domain.DefaultSettings(r.tenantID)
			return nil
		}
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading settings", err)
		}
		settings, err := toDomainSettings(m)
		if err != nil {
			return err
		}
		out = settings
		return nil
	})
	return out, err
}

// UpdateSettings replaces the school's limits.
func (r *Repository) UpdateSettings(ctx context.Context, s domain.Settings) (domain.Settings, error) {
	model := fromDomainSettings(s)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}},
			UpdateAll: true,
		}).Create(&model).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving settings", err)
		}
		return nil
	})
	if err != nil {
		return domain.Settings{}, err
	}
	return toDomainSettings(model)
}

// --- helpers -----------------------------------------------------------------

// escapeLike neutralises the wildcards a user may type, so searching for "100%"
// searches for "100%" and not for "everything starting with 100".
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// folioOf strips a delivery suffix, so searching "U-2026-000123-B" finds the
// order that delivery belongs to.
func folioOf(term string) string {
	parts := strings.Split(strings.TrimSpace(term), "-")
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "-")
	}
	return term
}

func offsetOf(token string) int {
	var n int
	if _, err := fmt.Sscanf(token, "%d", &n); err != nil || n < 0 {
		return 0
	}
	return n
}

func pageSizeOf(f ports.ListOrdersFilter) int {
	switch {
	case f.PageSize <= 0:
		return defaultPageSize
	case f.PageSize > maxPageSize:
		return maxPageSize
	default:
		return f.PageSize
	}
}
