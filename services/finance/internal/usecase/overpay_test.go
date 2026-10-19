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

// CC-202 (QA 2026-10-17): a deposit may not exceed what it is allowed to
// apply. Money nothing claims used to be stored as a credit that no charge
// ever used: the cut counted it, the statement said "a favor" and kept asking
// "a pagar hoy" at the same time. The counter already capped the amount; the
// server is the authority and now refuses it too, with nothing written.
func TestADepositMayNotExceedWhatItCanApply(t *testing.T) {
	// The year is $32,000 of tuition plus September's $300 surcharge
	// (TestTheStatementShowsTheFamilysCredit's arithmetic): $32,300 settles
	// everything, the months not yet open included — paying ahead is allowed.
	const wholeYear = 3230000
	for _, tc := range []struct {
		name    string
		amount  int64
		refused bool
	}{
		{"the whole year, paid ahead, is accepted", wholeYear, false},
		{"one peso more is refused", wholeYear + 100, true},
		{"the $99,999 of the QA is refused", 9999900, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := depositFixture(t)
			before := len(f.repo.deposits)

			got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: tc.amount, Method: domain.MethodCash,
				PaidAt: time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC), IdempotencyKey: "overpay",
			})

			if !tc.refused {
				require.NoError(t, err)
				assert.Zero(t, got.UnallocatedCents, "a deposit leaves no unapplied money")
				return
			}
			require.Error(t, err)
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Contains(t, glerrors.SafeMessage(err), "excede")
			assert.Len(t, f.repo.deposits, before, "a refused deposit stores nothing")
		})
	}
}

// The realistic path to an orphan credit: two cashiers take the same month at
// once. The second one finds it paid under the lock and must be told so — a
// 409 in Spanish — instead of keeping the money as a credit nobody applies.
func TestADepositOnAChosenChargeAlreadyPaidIsRefused(t *testing.T) {
	f := depositFixture(t)
	september := chargeDueIn(t, f, time.September)
	// The first cashier: September, its surcharge and October.
	deposit(t, f, 670000, "2026-10-10")
	before := len(f.repo.deposits)

	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 320000, Method: domain.MethodCash,
		PaidAt:    time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
		ChargeIDs: []string{september}, IdempotencyKey: "second-cashier",
	})

	require.Error(t, err)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), "ya está pagado")
	assert.Len(t, f.repo.deposits, before, "the second cashier's deposit is not stored")
}

// A chosen charge may take exactly what it owes; past that, the rest would be
// credit, so it is refused even though other months are still owed: the
// cashier chose where the money goes.
func TestADepositBeyondTheChosenChargesIsRefused(t *testing.T) {
	f := depositFixture(t)
	september := chargeDueIn(t, f, time.September)

	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 670000, Method: domain.MethodCash,
		PaidAt:    time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
		ChargeIDs: []string{september}, IdempotencyKey: "beyond-chosen",
	})

	require.Error(t, err)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), "conceptos elegidos")
}

// The preview runs the same rule: it never promises a credit the deposit
// would refuse.
func TestThePreviewRefusesWhatTheDepositWould(t *testing.T) {
	f := depositFixture(t)

	_, err := f.svc.PreviewDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 9999900, Method: domain.MethodCash,
		PaidAt: time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
	})

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
}

// A replay of a deposit that went through is the ORIGINAL, even though its
// charges are paid now: the refusal is for new money only.
func TestAReplayIsNotRefusedAsAnOverpayment(t *testing.T) {
	f := depositFixture(t)
	first := deposit(t, f, 670000, "2026-10-10")

	again := deposit(t, f, 670000, "2026-10-10")

	assert.True(t, again.Replayed)
	assert.Equal(t, first.Deposit.ID(), again.Deposit.ID())
}

// legacyCredit stores money no charge took, the way deposits taken before
// CC-202 left it: the use case no longer writes such a row, but the ledger
// still has to read the ones that exist.
func legacyCredit(t *testing.T, f *fixture, cents int64, on string) {
	t.Helper()
	day, err := time.Parse("2006-01-02", on)
	require.NoError(t, err)
	d, err := domain.NewDeposit("legacy-credit-"+on, testTenant, testStudent, cents,
		domain.MethodCash, "", day, "", "cal-test")
	require.NoError(t, err)
	f.repo.deposits = append(f.repo.deposits, d.WithPaidOn(day))
}

// chargeDueIn is the id of the student's tuition charge due in that month.
func chargeDueIn(t *testing.T, f *fixture, month time.Month) string {
	t.Helper()
	charges, err := allCharges(context.Background(), f.svc, testStudent, "")
	require.NoError(t, err)
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	for _, c := range charges {
		if c.Charge.ConceptID() == tuition && c.Charge.DueDate().Month() == month {
			return c.Charge.ID()
		}
	}
	t.Fatalf("no tuition charge due in %s", month)
	return ""
}
