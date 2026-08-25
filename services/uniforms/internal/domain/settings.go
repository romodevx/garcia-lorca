package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Settings are the numbers a school is allowed to argue about, which is exactly
// why none of them is a constant in the code.
//
// The ceilings are not paperwork. Without them one family can reserve the last
// unit of every size for ten working days, let them all lapse, and start again —
// at no cost, forever. They are the cheapest defence the module has, and the
// only one that does not need a person watching.
type Settings struct {
	tenantID                 string
	pickupWindowBusinessDays int
	autoCancelEnabled        bool
	maxExtensions            int
	maxOpenOrdersPerStudent  int
	maxLinesPerOrder         int
	maxUnitsPerLine          int
	maxLineExpiries          int
}

// DefaultSettings is what a school gets before it has opened the settings screen:
// a working shop, and the ten working days the owner asked for.
func DefaultSettings(tenantID string) Settings {
	return Settings{
		tenantID:                 tenantID,
		pickupWindowBusinessDays: 10,
		autoCancelEnabled:        true,
		maxExtensions:            2,
		maxOpenOrdersPerStudent:  5,
		maxLinesPerOrder:         20,
		maxUnitsPerLine:          5,
		maxLineExpiries:          2,
	}
}

// NewSettings builds a validated set. The bounds mirror the table's CHECKs, so a
// value that would be refused by the database is refused here first, with a
// message a person can act on.
func NewSettings(tenantID string, window int, autoCancel bool, maxExtensions, maxOpenOrders, maxLines, maxUnits, maxExpiries int) (Settings, error) {
	if strings.TrimSpace(tenantID) == "" {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if window < 1 || window > 90 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "the pickup window must be between 1 and 90 working days")
	}
	if maxExtensions < 0 || maxExtensions > 5 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "extensions must be between 0 and 5")
	}
	if maxOpenOrders < 1 || maxOpenOrders > 50 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "open orders per student must be between 1 and 50")
	}
	if maxLines < 1 || maxLines > 100 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "lines per order must be between 1 and 100")
	}
	if maxUnits < 1 || maxUnits > 20 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "units per line must be between 1 and 20")
	}
	if maxExpiries < 0 || maxExpiries > 10 {
		return Settings{}, glerrors.New(glerrors.KindInvalid, "lapses per line must be between 0 and 10")
	}
	return Settings{
		tenantID: tenantID, pickupWindowBusinessDays: window, autoCancelEnabled: autoCancel,
		maxExtensions: maxExtensions, maxOpenOrdersPerStudent: maxOpenOrders,
		maxLinesPerOrder: maxLines, maxUnitsPerLine: maxUnits, maxLineExpiries: maxExpiries,
	}, nil
}

// CheckOrderShape rejects an order that is too big before any stock is touched.
// The family is told at the door, not with a 4xx after keying everything in.
func (s Settings) CheckOrderShape(lineCount, maxUnitsOnAnyLine int) error {
	if lineCount < 1 {
		return glerrors.New(glerrors.KindInvalid, "an order needs at least one garment")
	}
	if lineCount > s.maxLinesPerOrder {
		return glerrors.New(glerrors.KindInvalid, "this order has more garments than the school allows")
	}
	if maxUnitsOnAnyLine > s.maxUnitsPerLine {
		return glerrors.New(glerrors.KindInvalid, "this order has more of one size than the school allows")
	}
	return nil
}

// CheckOpenOrders refuses a new order once the family is at its ceiling.
func (s Settings) CheckOpenOrders(currentOpen int) error {
	if currentOpen >= s.maxOpenOrdersPerStudent {
		return glerrors.New(glerrors.KindConflict, "this student already has as many open orders as the school allows")
	}
	return nil
}

// Accessors.
func (s Settings) TenantID() string              { return s.tenantID }
func (s Settings) PickupWindowBusinessDays() int { return s.pickupWindowBusinessDays }
func (s Settings) AutoCancelEnabled() bool       { return s.autoCancelEnabled }
func (s Settings) MaxExtensions() int            { return s.maxExtensions }
func (s Settings) MaxOpenOrdersPerStudent() int  { return s.maxOpenOrdersPerStudent }
func (s Settings) MaxLinesPerOrder() int         { return s.maxLinesPerOrder }
func (s Settings) MaxUnitsPerLine() int          { return s.maxUnitsPerLine }
func (s Settings) MaxLineExpiries() int          { return s.maxLineExpiries }
