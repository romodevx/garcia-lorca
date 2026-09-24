package postgres

import (
	"strings"
	"time"

	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

// Translation between the GORM rows and the domain. It is the only place that
// knows both, which is what keeps tags and nullable columns out of the rules.
//
// Every toDomain* rebuilds through the domain's constructor rather than assigning
// fields, so a row that has drifted out of the invariant is refused at the door
// instead of poisoning every operation after it.

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromNullable(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func fromNullableTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// --- catalogue ---------------------------------------------------------------

func toDomainItem(m itemModel) (domain.Item, error) {
	item, err := domain.NewItem(m.ID, m.TenantID, m.Code, m.Name, m.Description,
		domain.Level(fromNullable(m.LevelID)), m.PriceCents)
	if err != nil {
		return domain.Item{}, err
	}
	if m.LevelID == nil && fromNullable(m.LegacyLevel) != "" {
		item = item.WithUnresolvedLevel()
	}
	if m.Category != "" {
		item, err = item.WithCategory(m.Category)
		if err != nil {
			return domain.Item{}, err
		}
	}
	if !m.Active {
		item = item.Retire()
	}
	return item, nil
}

func fromDomainItem(i domain.Item) itemModel {
	return itemModel{
		ID: i.ID(), TenantID: i.TenantID(), Code: i.Code(), Name: i.Name(),
		Description: i.Description(), LevelID: levelIDColumn(i.Level()),
		PriceCents: i.PriceCents(), Active: i.Active(), Category: i.Category(),
	}
}

// --- inventory ---------------------------------------------------------------

func toDomainStock(m stockModel) (domain.StockLevel, error) {
	return domain.NewStockLevel(m.ID, m.TenantID, m.ItemID, m.SizeLabel,
		m.SizeOrder, m.OnHand, m.Reserved, m.MinStock)
}

func toDomainStockAll(ms []stockModel) ([]domain.StockLevel, error) {
	out := make([]domain.StockLevel, 0, len(ms))
	for _, m := range ms {
		s, err := toDomainStock(m)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// --- orders ------------------------------------------------------------------

func toDomainOrder(m orderModel) (domain.Order, error) {
	order, err := domain.NewOrder(m.ID, m.TenantID, m.Folio, domain.OrderStudent{
		ID:         fromNullable(m.StudentID),
		FullName:   m.StudentName,
		Level:      domain.Level(fromNullable(m.StudentLevelID)),
		LevelName:  studentLevelName(m),
		GradeGroup: m.StudentGradeGroup,
		CycleID:    fromNullable(m.CycleID),
	}, domain.OrderSource(m.Source), fromNullable(m.CreatedByUserID), m.CreatedAt)
	if err != nil {
		return domain.Order{}, err
	}
	if m.ClosedAt != nil {
		order = order.Close(*m.ClosedAt)
	}
	return order, nil
}

func fromDomainOrder(o domain.Order) orderModel {
	return orderModel{
		ID: o.ID(), TenantID: o.TenantID(), Folio: o.Folio(),
		StudentID: nullable(o.StudentID()), StudentName: o.StudentName(),
		StudentLevelID: nullable(string(o.StudentLevel())), StudentLevelName: o.StudentLevelName(),
		StudentGradeGroup: o.StudentGradeGroup(),
		CycleID:           nullable(o.CycleID()), CreatedByUserID: nullable(o.CreatedByUserID()),
		Source: string(o.Source()), CreatedAt: o.CreatedAt(), ClosedAt: nullableTime(o.ClosedAt()),
	}
}

func fromDomainLine(l domain.OrderLine) orderLineModel {
	return orderLineModel{
		ID: l.ID(), TenantID: l.TenantID(), OrderID: l.OrderID(), ItemID: l.ItemID(),
		ItemName: l.ItemName(), SizeLabel: l.SizeLabel(), Qty: l.Qty(),
		UnitPriceCents: l.UnitPriceCents(), Status: string(l.Status()),
		FulfillmentID: nullable(l.FulfillmentID()), ExpiryCount: l.ExpiryCount(),
	}
}

func toDomainLine(m orderLineModel) (domain.OrderLine, error) {
	line, err := domain.NewOrderLine(m.ID, m.TenantID, m.OrderID, m.ItemID, m.ItemName,
		m.SizeLabel, m.Qty, m.UnitPriceCents, domain.LineStatus(m.Status))
	if err != nil {
		return domain.OrderLine{}, err
	}
	if m.FulfillmentID != nil {
		line, err = line.AssignTo(*m.FulfillmentID)
		if err != nil {
			return domain.OrderLine{}, err
		}
	}
	return line, nil
}

// fromDomainFulfillment projects a delivery. The status is written as the domain
// left it — the adapter never invents one, because the transition table is the
// domain's to own.
func fromDomainFulfillment(f domain.Fulfillment) fulfillmentModel {
	return fulfillmentModel{
		ID: f.ID(), TenantID: f.TenantID(), OrderID: f.OrderID(), Seq: f.Seq(),
		Status: string(f.Status()), AmountCents: f.AmountCents(),
		FinanceChargeID: nullable(f.FinanceChargeID()),
		ReadyAt:         nullableTime(f.ReadyAt()), PickupDeadline: nullableTime(f.PickupDeadline()),
		DeadlineEstimated: f.DeadlineEstimated(), ExtensionSeq: f.ExtensionSeq(),
		RequiresAttention: f.RequiresAttention(), AttentionReason: f.AttentionReason(),
		PaidAt: nullableTime(f.PaidAt()), DispatchedAt: nullableTime(f.DispatchedAt()),
		CancelledAt: nullableTime(f.CancelledAt()), CancelReason: f.CancelReason(),
	}
}

// toDomainFulfillment rebuilds a delivery from its row.
//
// It does NOT replay the state machine to get there: a stored delivery is a fact
// about the past, and asking the transition table to re-approve it would make
// loading a row fail for something that already happened. The table governs what
// happens NEXT, which is the only place it can prevent anything.
func toDomainFulfillment(m fulfillmentModel) (domain.Fulfillment, error) {
	return domain.RestoreFulfillment(domain.FulfillmentSnapshot{
		ID: m.ID, TenantID: m.TenantID, OrderID: m.OrderID, Seq: m.Seq,
		Status: domain.FulfillmentStatus(m.Status), AmountCents: m.AmountCents,
		FinanceChargeID: fromNullable(m.FinanceChargeID),
		ReadyAt:         fromNullableTime(m.ReadyAt), PickupDeadline: fromNullableTime(m.PickupDeadline),
		DeadlineEstimated: m.DeadlineEstimated, ExtensionSeq: m.ExtensionSeq,
		RequiresAttention: m.RequiresAttention, AttentionReason: m.AttentionReason,
		PaidAt: fromNullableTime(m.PaidAt), DispatchedAt: fromNullableTime(m.DispatchedAt),
		CancelledAt: fromNullableTime(m.CancelledAt), CancelReason: m.CancelReason,
	})
}

// --- settings ----------------------------------------------------------------

func toDomainSettings(m settingsModel) (domain.Settings, error) {
	return domain.NewSettings(m.TenantID, m.PickupWindowBusinessDays, m.AutoCancelEnabled,
		m.MaxExtensions, m.MaxOpenOrdersPerStudent, m.MaxLinesPerOrder, m.MaxUnitsPerLine, m.MaxLineExpiries)
}

func fromDomainSettings(s domain.Settings) settingsModel {
	return settingsModel{
		TenantID: s.TenantID(), PickupWindowBusinessDays: s.PickupWindowBusinessDays(),
		AutoCancelEnabled: s.AutoCancelEnabled(), MaxExtensions: s.MaxExtensions(),
		MaxOpenOrdersPerStudent: s.MaxOpenOrdersPerStudent(), MaxLinesPerOrder: s.MaxLinesPerOrder(),
		MaxUnitsPerLine: s.MaxUnitsPerLine(), MaxLineExpiries: s.MaxLineExpiries(),
	}
}

// studentLevelName is the stored snapshot, or — for an order the previous
// revision wrote after migration 0007 ran, which carries only the old SEP key —
// that key capitalized ("secundaria" → "Secundaria").
func studentLevelName(m orderModel) string {
	if m.StudentLevelName != "" || m.LegacyStudentLevel == nil || *m.LegacyStudentLevel == "" {
		return m.StudentLevelName
	}
	key := *m.LegacyStudentLevel
	return strings.ToUpper(key[:1]) + key[1:]
}

// levelIDColumn stores every-level and unresolved alike as NULL: an unresolved
// garment keeps its old key in the legacy column, which is what marks it.
func levelIDColumn(l domain.Level) *string {
	if l == domain.LevelUnresolved {
		return nil
	}
	return nullable(string(l))
}
