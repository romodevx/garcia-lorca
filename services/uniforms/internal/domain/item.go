package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Level is a school level, mirroring the students service's own.
//
// LevelAny is the important one. The requirement is that a school MAY dress
// kinder differently from prepa — not that it must. Most garments are the same
// throughout, and making a school say so four times would be busywork, so a
// garment with no level applies to every level.
type Level string

const (
	LevelAny          Level = ""
	LevelKinder       Level = "kinder"
	LevelPrimaria     Level = "primaria"
	LevelSecundaria   Level = "secundaria"
	LevelPreparatoria Level = "preparatoria"
)

// IsValid reports whether l is a level this system knows.
func (l Level) IsValid() bool {
	switch l {
	case LevelAny, LevelKinder, LevelPrimaria, LevelSecundaria, LevelPreparatoria:
		return true
	}
	return false
}

// Item is one garment in the catalogue.
//
// Price lives here and only here. Whatever a client sends in a request body, the
// amount a family is charged is read from this record — which is what stops a
// crafted order buying a jumper for one peso.
type Item struct {
	id          string
	tenantID    string
	code        string
	name        string
	description string
	level       Level
	priceCents  int64
	active      bool
	category    string
}

const (
	maxCodeLen        = 40
	maxDescriptionLen = 500
)

// NewItem builds a validated garment, on sale.
// What kind of thing this is. The module sells whatever the school sells: the
// stock rules, the ledger and the counter sale were never about clothing, only
// the vocabulary was.
const (
	// CategoryUniform is a garment. Everything stored before categories existed
	// is one, which is the truth about it.
	CategoryUniform = "uniform"
	// CategoryBook is a textbook or workbook.
	CategoryBook = "book"
	// CategorySupply is stationery and other school supplies.
	CategorySupply = "supply"
	// CategoryOther is anything else the school decides to sell.
	CategoryOther = "other"
)

func validCategory(category string) bool {
	switch category {
	case CategoryUniform, CategoryBook, CategorySupply, CategoryOther:
		return true
	default:
		return false
	}
}

// WithCategory returns a copy filed under another category.
func (i Item) WithCategory(category string) (Item, error) {
	if !validCategory(category) {
		return Item{}, glerrors.New(glerrors.KindInvalid, "unknown item category")
	}
	i.category = category
	return i, nil
}

// Category is what kind of thing this is: uniform, book, supply or other.
func (i Item) Category() string { return i.category }

func NewItem(id, tenantID, code, name, description string, level Level, priceCents int64) (Item, error) {
	if strings.TrimSpace(id) == "" {
		return Item{}, glerrors.New(glerrors.KindInvalid, "item id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Item{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" || len(trimmedCode) > maxCodeLen {
		return Item{}, glerrors.New(glerrors.KindInvalid, "item code must be 1 to 40 characters")
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" || len(trimmedName) > maxItemNameLen {
		return Item{}, glerrors.New(glerrors.KindInvalid, "item name must be 1 to 120 characters")
	}
	trimmedDesc := strings.TrimSpace(description)
	if len(trimmedDesc) > maxDescriptionLen {
		return Item{}, glerrors.New(glerrors.KindInvalid, "item description is too long")
	}
	if !level.IsValid() {
		return Item{}, glerrors.New(glerrors.KindInvalid, "unknown school level")
	}
	if priceCents < 0 {
		return Item{}, glerrors.New(glerrors.KindInvalid, "price cannot be negative")
	}
	return Item{
		id: id, tenantID: tenantID, code: trimmedCode, name: trimmedName,
		description: trimmedDesc, level: level, priceCents: priceCents, active: true,
		category: CategoryUniform,
	}, nil
}

// AppliesTo reports whether a pupil of this level wears this garment. A garment
// with no level of its own applies to all of them; a garment scoped to one level
// applies to that one alone, and to no pupil whose level is unknown.
func (i Item) AppliesTo(level Level) bool {
	if i.level == LevelAny {
		return true
	}
	return i.level == level
}

// Retire takes a garment off sale WITHOUT deleting it. Orders already placed
// still point at it and must still be deliverable; deleting would orphan them.
// It is the same rule the finance concept catalogue keeps.
func (i Item) Retire() Item {
	i.active = false
	return i
}

// Restore puts a garment back on sale.
func (i Item) Restore() Item {
	i.active = true
	return i
}

// WithDetails returns a copy with the editable fields replaced, re-validated.
func (i Item) WithDetails(name, description string, level Level, priceCents int64) (Item, error) {
	updated, err := NewItem(i.id, i.tenantID, i.code, name, description, level, priceCents)
	if err != nil {
		return Item{}, err
	}
	updated.active = i.active
	return updated, nil
}

// LineFor builds an order line for this garment, priced from the catalogue. It
// is the ONLY way a line gets a price, which is the whole point: the caller
// supplies what and how many, never how much.
//
// A retired garment cannot start a new line — retired means retired from new
// billing, not merely hidden from the list.
func (i Item) LineFor(lineID, orderID, sizeLabel string, qty int, status LineStatus) (OrderLine, error) {
	if !i.active {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "this garment is no longer on sale")
	}
	return NewOrderLine(lineID, i.tenantID, orderID, i.id, i.name, sizeLabel, qty, i.priceCents, status)
}

// Accessors.
func (i Item) ID() string          { return i.id }
func (i Item) TenantID() string    { return i.tenantID }
func (i Item) Code() string        { return i.code }
func (i Item) Name() string        { return i.name }
func (i Item) Description() string { return i.description }
func (i Item) Level() Level        { return i.level }
func (i Item) PriceCents() int64   { return i.priceCents }
func (i Item) Active() bool        { return i.active }
