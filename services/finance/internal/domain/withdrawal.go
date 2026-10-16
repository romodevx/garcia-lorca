package domain

import (
	"strings"
	"time"
)

// WithdrawalPlan is the financial projection for a student's dated withdrawal.
type WithdrawalPlan struct {
	ToCancel           []Charge
	ToCancelCents      int64
	RemainingDebtCents int64
}

// PlanWithdrawal selects future charges that have received no money and sums
// the unpaid effective balance of charges that remain. Charges must already be
// priced through the same pricing rules used by the account statement.
func PlanWithdrawal(charges []Charge, paidByCharge map[string]int64, on time.Time) WithdrawalPlan {
	plan := WithdrawalPlan{ToCancel: make([]Charge, 0)}
	for _, charge := range charges {
		paid := paidByCharge[charge.ID()]
		if charge.Status() == StatusCancelled {
			continue
		}
		if charge.DueDate().After(on) && paid == 0 && charge.Status() == StatusPending && IsBillingLedgerRef(charge.ExternalRef()) {
			plan.ToCancel = append(plan.ToCancel, charge)
			plan.ToCancelCents += charge.Effective().Int64()
			continue
		}
		plan.RemainingDebtCents += max(0, charge.Effective().Int64()-paid)
	}
	return plan
}

// IsBillingLedgerRef reports whether billing created the charge. Withdrawals
// may cancel only charges owned by the tuition, fee, enrollment, and activity
// billing flows; manually entered and other-module charges remain debt.
func IsBillingLedgerRef(ref string) bool {
	for _, prefix := range []string{TuitionRefPrefix, FeeRefPrefix, EnrollmentRefPrefix, ActivityRefPrefix} {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}
