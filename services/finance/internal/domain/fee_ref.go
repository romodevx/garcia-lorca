package domain

import "strings"

// The refs a written-down surcharge carries, naming the month it rides on. The
// deposit planner writes and reads them, the billing run writes the monthly
// one, and a receipt marks a line as a late fee by them — one definition, so
// none of the three can drift.
const (
	// DailyFeeRefPrefix: a daily surcharge a deposit wrote,
	// "dailyfee:<chargeID>:<depositID>".
	DailyFeeRefPrefix = "dailyfee:"
	// LateFeeRefPrefix: a monthly surcharge the billing run issued,
	// "latefee:<chargeID>:<YYYY-MM>".
	LateFeeRefPrefix = "latefee:"
	// EnrollmentRefPrefix: an inscription line the billing run issued,
	// "enrollment:<cycle>:<student>" (single amount) or
	// "enrollment:<cycle>:<student>:<slug>" (one concept).
	EnrollmentRefPrefix = "enrollment:"
	// FeeRefPrefix: a share of a plan's fee the billing run issued,
	// "fee:<cycle>:<student>:<slug>:<YYYY-MM>".
	FeeRefPrefix = "fee:"
)

// IsFeeRef reports whether a charge's ref names a late fee, daily or monthly.
func IsFeeRef(ref string) bool {
	return strings.HasPrefix(ref, DailyFeeRefPrefix) || strings.HasPrefix(ref, LateFeeRefPrefix)
}
