package postgres

import "time"

// GORM rows for the uniform shop. These are the OUTER model of the double-model
// pattern: they carry tags, nullable columns and persistence concerns, and the
// domain never sees one. Mapping both ways lives in mapper.go.

// itemModel is the `uniform_items` row. Level is a pointer because NULL is
// meaningful — it means the garment applies to every level — and an empty string
// would be a different, wrong answer.
type itemModel struct {
	ID          string  `gorm:"column:id;primaryKey"`
	TenantID    string  `gorm:"column:tenant_id"`
	Code        string  `gorm:"column:code"`
	Name        string  `gorm:"column:name"`
	Description string  `gorm:"column:description"`
	LevelID     *string `gorm:"column:level_id"`
	// LegacyLevel is the pre-0007 SEP key, read (never written): a scoped
	// garment that never got a level_id must not read as every level.
	LegacyLevel *string `gorm:"column:level;<-:false"`
	PriceCents  int64   `gorm:"column:price_cents"`
	Active      bool    `gorm:"column:active"`
	// What kind of thing this is: uniform, book, supply or other. A row written
	// before the module sold anything but clothing holds 'uniform'.
	Category  string    `gorm:"column:category"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (itemModel) TableName() string { return "uniform_items" }

// stockModel is the `uniform_stock` row: one garment in one size. OnHand and
// Reserved are the pair the CHECK constraint governs; Available is derived and
// deliberately absent.
type stockModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	ItemID    string    `gorm:"column:item_id"`
	SizeLabel string    `gorm:"column:size_label"`
	SizeOrder int       `gorm:"column:size_order"`
	OnHand    int       `gorm:"column:on_hand"`
	Reserved  int       `gorm:"column:reserved"`
	MinStock  int       `gorm:"column:min_stock"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (stockModel) TableName() string { return "uniform_stock" }

// stockMoveModel is the `uniform_stock_moves` row. Append-only: there is no
// update path here and none in the database either, because the ledger's whole
// value is that a physical count can be audited against it months later.
type stockMoveModel struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TenantID    string    `gorm:"column:tenant_id"`
	StockID     string    `gorm:"column:stock_id"`
	OrderLineID *string   `gorm:"column:order_line_id"`
	Kind        string    `gorm:"column:kind"`
	Qty         int       `gorm:"column:qty"`
	ActorUserID *string   `gorm:"column:actor_user_id"`
	Note        string    `gorm:"column:note"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (stockMoveModel) TableName() string { return "uniform_stock_moves" }

// orderModel is the `uniform_orders` row. StudentID, Level and CycleID are
// pointers because an anonymous counter sale legitimately has none of them.
type orderModel struct {
	ID               string  `gorm:"column:id;primaryKey"`
	TenantID         string  `gorm:"column:tenant_id"`
	Folio            string  `gorm:"column:folio"`
	StudentID        *string `gorm:"column:student_id"`
	StudentName      string  `gorm:"column:student_name"`
	StudentLevelID   *string `gorm:"column:student_level_id"`
	StudentLevelName string  `gorm:"column:student_level_name"`
	// LegacyStudentLevel is the pre-0007 SEP key, read (never written) so an
	// order the previous revision wrote mid-deploy still shows a level.
	LegacyStudentLevel *string    `gorm:"column:student_level;<-:false"`
	StudentGradeGroup  string     `gorm:"column:student_grade_group"`
	CycleID            *string    `gorm:"column:cycle_id"`
	CreatedByUserID    *string    `gorm:"column:created_by_user_id"`
	Source             string     `gorm:"column:source"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	ClosedAt           *time.Time `gorm:"column:closed_at"`
}

func (orderModel) TableName() string { return "uniform_orders" }

// fulfillmentModel is the `uniform_fulfillments` row: one delivery, and where
// the state machine is stored.
type fulfillmentModel struct {
	ID                string     `gorm:"column:id;primaryKey"`
	TenantID          string     `gorm:"column:tenant_id"`
	OrderID           string     `gorm:"column:order_id"`
	Seq               int        `gorm:"column:seq"`
	Status            string     `gorm:"column:status"`
	AmountCents       int64      `gorm:"column:amount_cents"`
	FinanceChargeID   *string    `gorm:"column:finance_charge_id"`
	ReadyAt           *time.Time `gorm:"column:ready_at"`
	PickupDeadline    *time.Time `gorm:"column:pickup_deadline"`
	DeadlineEstimated bool       `gorm:"column:deadline_estimated"`
	ExtendedDays      int        `gorm:"column:extended_days"`
	ExtensionSeq      int        `gorm:"column:extension_seq"`
	RequiresAttention bool       `gorm:"column:requires_attention"`
	AttentionReason   string     `gorm:"column:attention_reason"`
	PaidAt            *time.Time `gorm:"column:paid_at"`
	DispatchedAt      *time.Time `gorm:"column:dispatched_at"`
	CancelledAt       *time.Time `gorm:"column:cancelled_at"`
	CancelReason      string     `gorm:"column:cancel_reason"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (fulfillmentModel) TableName() string { return "uniform_fulfillments" }

// orderLineModel is the `uniform_order_lines` row.
type orderLineModel struct {
	ID             string    `gorm:"column:id;primaryKey"`
	TenantID       string    `gorm:"column:tenant_id"`
	OrderID        string    `gorm:"column:order_id"`
	ItemID         string    `gorm:"column:item_id"`
	ItemName       string    `gorm:"column:item_name"`
	SizeLabel      string    `gorm:"column:size_label"`
	Qty            int       `gorm:"column:qty"`
	UnitPriceCents int64     `gorm:"column:unit_price_cents"`
	Status         string    `gorm:"column:status"`
	FulfillmentID  *string   `gorm:"column:fulfillment_id"`
	ExpiryCount    int       `gorm:"column:expiry_count"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (orderLineModel) TableName() string { return "uniform_order_lines" }

// settingsModel is the `uniform_settings` row: one per school.
type settingsModel struct {
	TenantID                 string    `gorm:"column:tenant_id;primaryKey"`
	PickupWindowBusinessDays int       `gorm:"column:pickup_window_business_days"`
	AutoCancelEnabled        bool      `gorm:"column:auto_cancel_enabled"`
	MaxExtensions            int       `gorm:"column:max_extensions"`
	MaxOpenOrdersPerStudent  int       `gorm:"column:max_open_orders_per_student"`
	MaxLinesPerOrder         int       `gorm:"column:max_lines_per_order"`
	MaxUnitsPerLine          int       `gorm:"column:max_units_per_line"`
	MaxLineExpiries          int       `gorm:"column:max_line_expiries"`
	UpdatedAt                time.Time `gorm:"column:updated_at"`
}

func (settingsModel) TableName() string { return "uniform_settings" }

// There is deliberately no model for `uniform_folio_counters`. Allocating a
// folio is one UPSERT ... RETURNING that has to read and bump the counter in a
// single statement; a struct round-trip would turn that into a read, a decision
// and a write, which is the shape that hands two tills the same number.
