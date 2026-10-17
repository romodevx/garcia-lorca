package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// CC-205: a deposit taken on a card without debit/credit is refused, by its
// tenders or by its single method, and nothing is stored. With the type it
// goes through.
func TestADepositOnACardWithoutItsTypeIsRefused(t *testing.T) {
	paidAt := time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		in      usecase.DepositInput
		wantErr bool
	}{
		{"a card tender with no type", usecase.DepositInput{
			AmountCents: 320000, Tenders: []usecase.TenderInput{{Method: domain.MethodCard, AmountCents: 320000}},
		}, true},
		{"cash and a card with no type", usecase.DepositInput{
			AmountCents: 320000, Tenders: []usecase.TenderInput{
				{Method: domain.MethodCash, AmountCents: 20000}, {Method: domain.MethodCard, AmountCents: 300000}},
		}, true},
		{"a single card method with no type", usecase.DepositInput{AmountCents: 320000, Method: domain.MethodCard}, true},
		{"a debit card tender", usecase.DepositInput{
			AmountCents: 320000, Tenders: []usecase.TenderInput{
				{Method: domain.MethodCard, AmountCents: 320000, CardType: domain.CardDebit}},
		}, false},
		{"a single credit card", usecase.DepositInput{AmountCents: 320000, Method: domain.MethodCard, CardType: domain.CardCredit}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := depositFixture(t)
			before := len(f.repo.deposits)
			in := tc.in
			in.StudentID, in.PaidAt, in.IdempotencyKey = testStudent, paidAt, "card-"+tc.name

			_, err := f.svc.RegisterDeposit(context.Background(), in)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Len(t, f.repo.deposits, before, "nothing stored")
		})
	}
}
