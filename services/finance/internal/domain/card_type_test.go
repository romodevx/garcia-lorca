package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// CC-205 (QA 2026-10-17): money taken at the counter on a card says whether it
// was debit or credit — the bank's cut separates them. The counter always
// preselected debit, so a card with no type only came in by API, and it was
// stored. A deposit being TAKEN now requires it; one read back from before
// the rule (NewTender, the mapper) still may lack it.
func TestADepositTakenOnACardNeedsItsType(t *testing.T) {
	paidAt := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	tender := func(method, cardType string, cents int64) domain.Tender {
		t.Helper()
		tn, err := domain.NewTender(method, cents, "", cardType)
		require.NoError(t, err)
		return tn
	}
	for _, tc := range []struct {
		name    string
		tenders []domain.Tender
		wantErr bool
	}{
		{"cash needs no type", []domain.Tender{tender(domain.MethodCash, "", 25000)}, false},
		{"a debit card", []domain.Tender{tender(domain.MethodCard, domain.CardDebit, 25000)}, false},
		{"a credit card", []domain.Tender{tender(domain.MethodCard, domain.CardCredit, 25000)}, false},
		{"a card with no type", []domain.Tender{tender(domain.MethodCard, "", 25000)}, true},
		{"cash plus a card with no type", []domain.Tender{
			tender(domain.MethodCash, "", 10000), tender(domain.MethodCard, "", 15000)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := domain.NewDeposit("d-1", "t-1", "s-1", 25000, tc.tenders[0].Method(), "", paidAt, "", "")
			require.NoError(t, err)
			d, err = d.WithTenders(tc.tenders)
			require.NoError(t, err)

			err = d.RequireCardType()

			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Contains(t, glerrors.SafeMessage(err), "débito o crédito")
		})
	}
}

// The single-method deposit (no tenders) is held to the same rule.
func TestASingleMethodCardDepositNeedsItsType(t *testing.T) {
	d, err := domain.NewDeposit("d-1", "t-1", "s-1", 25000, domain.MethodCard, "",
		time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), "", "")
	require.NoError(t, err)
	assert.Error(t, d.RequireCardType(), "no type")

	typed, err := d.WithCardType(domain.CardCredit)
	require.NoError(t, err)
	assert.NoError(t, typed.RequireCardType())
}
