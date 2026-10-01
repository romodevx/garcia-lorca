//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// correctionFixture is a family with one month of 3,200.00 and a deposit keyed
// as 1,600.00 that should have been 3,200.00.
type correctionFixture struct {
	tenant, student string
	charge          domain.Charge
	original        domain.Deposit
	paidOn          time.Time
	repo            interface {
		ports.FinanceRepository
	}
	ids *int
}

func newCorrectionFixture(t *testing.T, suffix string, ids int) correctionFixture {
	t.Helper()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000c00"+suffix
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000c00"+suffix, "colegiatura corrección "+suffix,
		"61000000-0000-0000-0000-0000000c00"+suffix, student, "70000000-0000-0000-0000-0000000c00"+suffix, 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	paidOn := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	repo := appRepo(t, tenant)
	original := newDeposit(t, "62000000-0000-0000-0000-0000000c00"+suffix, tenant, student, 160000, paidOn, "")
	counter := ids
	stored, _, err := repo.RegisterDeposit(context.Background(), original,
		splitOldestFirst(t, tenant, &counter, 160000, paidOn), nil)
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	return correctionFixture{tenant: tenant, student: student, charge: charge, original: stored.Deposit, paidOn: paidOn, repo: repo, ids: &counter}
}

func (f correctionFixture) write(t *testing.T, correctedID, key string, amount int64, at time.Time) ports.DepositCorrectionWrite {
	t.Helper()
	corrected := newDeposit(t, correctedID, f.tenant, f.student, amount, at, key).
		WithReceiver(f.original.ReceivedBy()).WithCorrects(f.original.ID(), "")
	return ports.DepositCorrectionWrite{
		OriginalID: f.original.ID(), Corrected: corrected,
		CorrectionID: "63000000-0000-0000-0000-" + correctedID[len(correctedID)-12:],
		Reason:       "Se capturó $1,600 y fueron $3,200",
		By:           domain.Receiver{UserID: "64000000-0000-0000-0000-000000000001", Name: "Directora Ruiz"},
		At:           time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC),
	}
}

// T9, the whole point against real SQL: the original stays, superseded and
// counting for nothing; the corrected deposit pays what the family really
// handed over; the history keeps before, after, who and why.
func TestCorrectDepositSupersedesTheOriginalAndMovesTheMoney(t *testing.T) {
	f := newCorrectionFixture(t, "01", 0x10)
	ctx := context.Background()
	w := f.write(t, "62000000-0000-0000-0000-0000000c1001", "corr-key-01", 320000, f.paidOn)

	var announced ports.CorrectionRecord
	got, replayed, err := f.repo.CorrectDeposit(ctx, w, splitOldestFirst(t, f.tenant, f.ids, 320000, f.paidOn),
		func(r ports.CorrectionRecord) []ports.OutboxEvent {
			announced = r
			return []ports.OutboxEvent{{Type: ports.OutboxPaymentCorrected, Payload: []byte(`{}`)}}
		})
	if err != nil || replayed {
		t.Fatalf("CorrectDeposit: replayed=%v err=%v", replayed, err)
	}
	if announced.Corrected.Deposit.ID() != w.Corrected.ID() {
		t.Error("the outbox was not handed the stored correction")
	}

	charge, err := f.repo.GetCharge(ctx, f.charge.ID())
	if err != nil {
		t.Fatal(err)
	}
	if charge.PaidCents != 320000 || charge.Charge.Status() != domain.StatusPaid {
		t.Errorf("charge paid %d (%s) after the correction, want 320000 paid", charge.PaidCents, charge.Charge.Status())
	}

	original, err := f.repo.GetDeposit(ctx, f.original.ID())
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := f.repo.GetDeposit(ctx, w.Corrected.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !original.Superseded() || original.SupersededBy() != corrected.ID() || original.SupersededByFolio() != corrected.Folio() {
		t.Errorf("original: superseded=%v by %q (%q)", original.Superseded(), original.SupersededBy(), original.SupersededByFolio())
	}
	if original.Amount().Int64() != 160000 || original.Folio() != f.original.Folio() {
		t.Error("the original must be kept exactly as it was recorded")
	}
	if corrected.Corrects() != original.ID() || corrected.CorrectsFolio() != original.Folio() {
		t.Errorf("corrected names %q (%q)", corrected.Corrects(), corrected.CorrectsFolio())
	}
	if corrected.Folio() == "" || corrected.Folio() == original.Folio() {
		t.Errorf("the correction gets a folio of its own, got %q", corrected.Folio())
	}
	if got.Original.SupersededBy() != corrected.ID() || got.Corrected.Deposit.Folio() != corrected.Folio() {
		t.Errorf("record = %+v", got)
	}

	// The superseded deposit is not the family's credit.
	if credit, err := f.repo.UnallocatedCents(ctx, f.student); err != nil || credit != 0 {
		t.Errorf("credit = %d (%v), want 0", credit, err)
	}
	// Its receipt prints no lines and no credit: its money moved.
	receipt, err := f.repo.DepositReceipt(ctx, original.ID())
	if err != nil || len(receipt.Lines) != 0 || receipt.UnallocatedCents != 0 {
		t.Errorf("superseded receipt = %+v (%v)", receipt, err)
	}

	history, err := f.repo.ListDepositCorrections(ctx, f.student)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %d (%v), want 1", len(history), err)
	}
	h := history[0]
	if h.Reason() != w.Reason || h.By() != w.By || h.OriginalID() != original.ID() || h.CorrectedID() != corrected.ID() {
		t.Errorf("history row = %+v", h)
	}
	if h.Before().AmountCents != 160000 || h.After().AmountCents != 320000 {
		t.Errorf("before/after = %d/%d", h.Before().AmountCents, h.After().AmountCents)
	}
	if len(h.Before().Lines) != 1 || h.Before().Lines[0].AmountCents != 160000 || h.After().Lines[0].AmountCents != 320000 {
		t.Errorf("lines before %+v after %+v", h.Before().Lines, h.After().Lines)
	}
	if h.Before().Folio != original.Folio() || h.After().Folio != corrected.Folio() || len(h.Before().Tenders) != 1 {
		t.Errorf("snapshots = %+v / %+v", h.Before(), h.After())
	}

	// The outbox is shared by every test here: leave nothing for another
	// test's drain to count.
	t.Cleanup(func() {
		superDB.Exec(`UPDATE finance_outbox SET published_at = now() WHERE event_type = ? AND published_at IS NULL`, ports.OutboxPaymentCorrected)
	})
	var queued int64
	if err := superDB.Raw(`SELECT count(*) FROM finance_outbox WHERE event_type = ?`, ports.OutboxPaymentCorrected).Scan(&queued).Error; err != nil || queued < 1 {
		t.Errorf("PaymentCorrected queued %d (%v)", queued, err)
	}
}

// A correction retried under its key is the same correction: nothing moves twice.
func TestCorrectDepositIsIdempotent(t *testing.T) {
	f := newCorrectionFixture(t, "02", 0x20)
	ctx := context.Background()
	w := f.write(t, "62000000-0000-0000-0000-0000000c1002", "corr-key-02", 320000, f.paidOn)
	if _, _, err := f.repo.CorrectDeposit(ctx, w, splitOldestFirst(t, f.tenant, f.ids, 320000, f.paidOn), nil); err != nil {
		t.Fatalf("first: %v", err)
	}

	retry := f.write(t, "62000000-0000-0000-0000-0000000c1003", "corr-key-02", 320000, f.paidOn)
	got, replayed, err := f.repo.CorrectDeposit(ctx, retry, splitOldestFirst(t, f.tenant, f.ids, 320000, f.paidOn), nil)
	if err != nil || !replayed {
		t.Fatalf("retry: replayed=%v err=%v", replayed, err)
	}
	if got.Corrected.Deposit.ID() != w.Corrected.ID() || got.Correction.CorrectedID() != w.Corrected.ID() {
		t.Errorf("replay returned %q, want the first correction %q", got.Corrected.Deposit.ID(), w.Corrected.ID())
	}
	if history, _ := f.repo.ListDepositCorrections(ctx, f.student); len(history) != 1 {
		t.Errorf("history = %d rows, want 1", len(history))
	}
}

// A deposit is corrected once; a cancelled one, never. And a superseded
// deposit cannot be voided: its money already moved.
func TestCorrectDepositRefusesWhatCannotBeCorrected(t *testing.T) {
	f := newCorrectionFixture(t, "03", 0x30)
	ctx := context.Background()
	w := f.write(t, "62000000-0000-0000-0000-0000000c1004", "corr-key-03", 320000, f.paidOn)
	if _, _, err := f.repo.CorrectDeposit(ctx, w, splitOldestFirst(t, f.tenant, f.ids, 320000, f.paidOn), nil); err != nil {
		t.Fatalf("first: %v", err)
	}

	again := f.write(t, "62000000-0000-0000-0000-0000000c1005", "corr-key-03b", 300000, f.paidOn)
	if _, _, err := f.repo.CorrectDeposit(ctx, again, splitOldestFirst(t, f.tenant, f.ids, 300000, f.paidOn), nil); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second correction of the same deposit: %v, want conflict", err)
	}

	// The stale screen: it still holds the original as live and voids it.
	if err := f.repo.VoidDeposit(ctx, mustVoid(t, f.original)); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("voiding a superseded deposit: %v, want conflict", err)
	}

	g := newCorrectionFixture(t, "04", 0x40)
	if err := g.repo.VoidDeposit(ctx, mustVoid(t, g.original)); err != nil {
		t.Fatal(err)
	}
	cancelled := g.write(t, "62000000-0000-0000-0000-0000000c1006", "corr-key-04", 320000, g.paidOn)
	if _, _, err := g.repo.CorrectDeposit(ctx, cancelled, splitOldestFirst(t, g.tenant, g.ids, 320000, g.paidOn), nil); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("correcting a cancelled deposit: %v, want conflict", err)
	}
}

func mustVoid(t *testing.T, d domain.Deposit) domain.Deposit {
	t.Helper()
	v, err := d.Void("capturado mal", time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// The early-payment reward the original earned by its date goes with it: the
// corrected date decides again. A reward the school GRANTED by hand stays.
func TestCorrectDepositReleasesTheRewardTheOriginalEarned(t *testing.T) {
	for name, tc := range map[string]struct {
		suffix, reason string
		wantDiscount   int64
	}{
		"earned by the date": {"05", domain.ReasonEarlyPayment, 0},
		"granted by hand":    {"06", domain.ReasonEarlyPaymentGranted, 20000},
	} {
		t.Run(name, func(t *testing.T) {
			tenant, student := tenantA, "80000000-0000-0000-0000-0000000c00"+tc.suffix
			charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000c00"+tc.suffix, "colegiatura pronto "+tc.suffix,
				"61000000-0000-0000-0000-0000000c00"+tc.suffix, student, "70000000-0000-0000-0000-0000000c00"+tc.suffix, 320000, 0,
				time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
			rewarded, err := charge.WithDiscount(20000, 0, tc.reason)
			if err != nil {
				t.Fatal(err)
			}
			paidOn := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
			repo := appRepo(t, tenant)
			original := newDeposit(t, "62000000-0000-0000-0000-0000000c00"+tc.suffix, tenant, student, 300000, paidOn, "")
			payment := newPayment(t, "90000000-0000-0000-0000-0000000c00"+tc.suffix, tenant, charge.ID(), 300000, paidOn)
			if _, _, err := repo.RegisterDeposit(context.Background(), original,
				func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
					return ports.DepositWrite{Discounted: []domain.Charge{rewarded}, Allocations: []domain.Payment{payment}}, nil
				}, nil); err != nil {
				t.Fatal(err)
			}

			var seen []ports.ChargeWithPaid
			corrected := newDeposit(t, "62000000-0000-0000-0000-0000000c20"+tc.suffix, tenant, student, 100000, paidOn, "").WithCorrects(original.ID(), "")
			_, _, err = repo.CorrectDeposit(context.Background(), ports.DepositCorrectionWrite{
				OriginalID: original.ID(), Corrected: corrected, CorrectionID: "63000000-0000-0000-0000-0000000c20" + tc.suffix,
				Reason: "monto", At: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
			}, func(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
				seen = open
				return ports.DepositWrite{UnallocatedCents: 100000}, nil
			}, nil)
			if err != nil {
				t.Fatalf("CorrectDeposit: %v", err)
			}
			// The planner sees the month as the release left it.
			if len(seen) != 1 || seen[0].PaidCents != 0 || seen[0].Charge.Discount().Int64() != tc.wantDiscount {
				t.Errorf("planner saw %+v, want unpaid with discount %d", seen, tc.wantDiscount)
			}
			after, err := repo.GetCharge(context.Background(), charge.ID())
			if err != nil {
				t.Fatal(err)
			}
			if after.Charge.Discount().Int64() != tc.wantDiscount || after.Charge.Status() != domain.StatusPending {
				t.Errorf("after: discount %d %s, want %d pending", after.Charge.Discount().Int64(), after.Charge.Status(), tc.wantDiscount)
			}
		})
	}
}

// RLS: another school can neither correct this school's payment nor read its
// history.
func TestRLSIsolatesCorrections(t *testing.T) {
	f := newCorrectionFixture(t, "07", 0x50)
	ctx := context.Background()
	other := appRepo(t, tenantB)
	w := f.write(t, "62000000-0000-0000-0000-0000000c1007", "corr-key-07", 320000, f.paidOn)
	w.Corrected = newDeposit(t, w.Corrected.ID(), tenantB, f.student, 320000, f.paidOn, "corr-key-07").WithCorrects(f.original.ID(), "")
	if _, _, err := other.CorrectDeposit(ctx, w, splitOldestFirst(t, tenantB, f.ids, 320000, f.paidOn), nil); !isNotFound(err) {
		t.Errorf("tenant B corrected tenant A's deposit: %v", err)
	}

	mine := f.write(t, "62000000-0000-0000-0000-0000000c1008", "corr-key-08", 320000, f.paidOn)
	if _, _, err := f.repo.CorrectDeposit(ctx, mine, splitOldestFirst(t, f.tenant, f.ids, 320000, f.paidOn), nil); err != nil {
		t.Fatal(err)
	}
	if rows, err := other.ListDepositCorrections(ctx, f.student); err != nil || len(rows) != 0 {
		t.Errorf("tenant B read %d corrections (%v)", len(rows), err)
	}
}
