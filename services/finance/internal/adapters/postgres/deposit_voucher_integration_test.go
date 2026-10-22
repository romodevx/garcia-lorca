//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// ADR 0040 §4 against real SQL: linking a voucher to money happens inside the
// deposit transaction, under the pupil's charges lock. Ids: pupils 88…,
// concepts 68…, charges 681…, deposits 682…, vouchers b8…, allocations 98…
// (minted from a counter so no two deposits share one).

var vlAllocations atomic.Int64

func vlID(prefix string, n int) string { return fmt.Sprintf("%s-0000-0000-0000-%012d", prefix, n) }

// vlFamily is one pupil owing two months of 1,000.00, September and October.
type vlFamily struct {
	student string
	sep     domain.Charge
	oct     domain.Charge
	repo    *pgadapter.Repository
}

func newVLFamily(t *testing.T, n int) vlFamily {
	t.Helper()
	student := vlID("88000000", n)
	sep := seedConceptAndCharge(t, tenantA, vlID("68000000", n), fmt.Sprintf("vl concept %d", n),
		vlID("68100000", n*10+1), student, cycleVoucher, 100000, 0, time.Date(2032, 9, 10, 0, 0, 0, 0, time.UTC))
	oct := newCharge(t, vlID("68100000", n*10+2), tenantA, student, cycleVoucher, sep.ConceptID(), 100000, 0,
		time.Date(2032, 10, 10, 0, 0, 0, 0, time.UTC))
	oct, err := oct.WithExternalRef(fmt.Sprintf("tuition:vl-%d:2032-10", n))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appRepo(t, tenantA).CreateCharge(context.Background(), oct); err != nil {
		t.Fatalf("seed october: %v", err)
	}
	return vlFamily{student: student, sep: sep, oct: oct, repo: appRepo(t, tenantA)}
}

func (f vlFamily) voucher(t *testing.T, n int) domain.PaymentVoucher {
	t.Helper()
	v := vchNew(t, vlID("b8000000", n), f.student, fmt.Sprintf("vl-%d", n), 100000)
	stored, _, err := f.repo.CreatePaymentVoucher(context.Background(), v, vchUploaded(v))
	if err != nil {
		t.Fatalf("CreatePaymentVoucher: %v", err)
	}
	return stored
}

// deposit builds a transfer from the pupil; key and choice say how it was taken.
func (f vlFamily) deposit(t *testing.T, id string, amount int64, key string, choice domain.VoucherChoice) domain.Deposit {
	t.Helper()
	d, err := domain.NewDeposit(id, tenantA, f.student, amount, domain.MethodTransfer, "", vchDay, key, "cal-test")
	if err != nil {
		t.Fatal(err)
	}
	return d.WithPaidOn(vchDay).WithReceiver(vchCashier).WithVoucherChoice(choice)
}

// split fills the locked charges oldest first, minting allocation ids from
// the shared counter.
func vlSplit(t *testing.T, amount int64) func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
	return func(open []ports.ChargeWithPaid) (ports.DepositWrite, error) {
		remaining := amount
		var payments []domain.Payment
		for _, c := range open {
			owed := c.Charge.Effective().Int64() - c.PaidCents
			if owed <= 0 || remaining <= 0 {
				continue
			}
			take := min(owed, remaining)
			payments = append(payments, newPayment(t, vlID("98000000", int(vlAllocations.Add(1))), tenantA, c.Charge.ID(), take, vchDay))
			remaining -= take
		}
		return ports.DepositWrite{Allocations: payments, UnallocatedCents: remaining}, nil
	}
}

func (f vlFamily) register(t *testing.T, d domain.Deposit) (ports.DepositRecord, bool, error) {
	t.Helper()
	return f.repo.RegisterDeposit(context.Background(), d, vlSplit(t, d.Amount().Int64()), nil)
}

func (f vlFamily) reread(t *testing.T, id string) domain.PaymentVoucher {
	t.Helper()
	v, err := f.repo.GetPaymentVoucher(context.Background(), id)
	if err != nil {
		t.Fatalf("GetPaymentVoucher: %v", err)
	}
	return v
}

func countDepositsOf(t *testing.T, student string) int64 {
	t.Helper()
	var n int64
	if err := superDB.Raw("SELECT count(*) FROM deposits WHERE student_id = ?", student).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// A. The same voucher applied twice AT ONCE (two tabs, a double click) is ONE
// deposit: the derived key and the voucher's row lock serialise them, and the
// second is answered as a replay.
func TestApplyVoucherTwiceConcurrentlyIsOneDeposit(t *testing.T) {
	f := newVLFamily(t, 1)
	v := f.voucher(t, 1)
	const attempts = 4
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		replays  int
		failures []error
		deposits = map[string]bool{}
	)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := appRepo(t, tenantA)
			d := f.deposit(t, vlID("68200000", 10+i), 100000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()})
			record, replayed, err := repo.RegisterDeposit(context.Background(), d, vlSplit(t, 100000), nil)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err)
				return
			}
			deposits[record.Deposit.ID()] = true
			if replayed {
				replays++
			}
			if record.LinkedVoucher == nil || record.LinkedVoucher.ID() != v.ID() {
				failures = append(failures, fmt.Errorf("attempt %d: linked voucher = %+v", i, record.LinkedVoucher))
			}
		}(i)
	}
	wg.Wait()
	if len(failures) > 0 || len(deposits) != 1 || replays != attempts-1 {
		t.Fatalf("deposits=%v replays=%d failures=%v", deposits, replays, failures)
	}
	if n := countDepositsOf(t, f.student); n != 1 {
		t.Fatalf("stored deposits = %d, want 1", n)
	}
	got := f.reread(t, v.ID())
	if got.Status() != domain.VoucherApplied || !deposits[got.DepositID()] || got.DecidedBy() != vchCashier || got.DepositFolio() == "" {
		t.Fatalf("voucher = %+v", got)
	}
	sep, _ := f.repo.GetCharge(context.Background(), f.sep.ID())
	if sep.PaidCents != 100000 {
		t.Fatalf("september paid %d, want 100000 once", sep.PaidCents)
	}
}

// A. A retry under the derived key replays and keeps answering the voucher.
func TestApplyVoucherReplayUnderDerivedKey(t *testing.T) {
	f := newVLFamily(t, 2)
	v := f.voucher(t, 2)
	choice := domain.VoucherChoice{VoucherID: v.ID()}
	first, replayed, err := f.register(t, f.deposit(t, vlID("68200000", 20), 100000, v.IdempotencyKey(), choice))
	if err != nil || replayed || first.LinkedVoucher == nil || first.LinkedVoucher.Status() != domain.VoucherApplied {
		t.Fatalf("first: %+v replayed=%v err=%v", first.LinkedVoucher, replayed, err)
	}
	again, replayed, err := f.register(t, f.deposit(t, vlID("68200000", 21), 100000, v.IdempotencyKey(), choice))
	if err != nil || !replayed || again.Deposit.ID() != first.Deposit.ID() || again.LinkedVoucher == nil ||
		again.LinkedVoucher.ID() != v.ID() {
		t.Fatalf("retry: deposit %s replayed=%v err=%v linked=%+v", again.Deposit.ID(), replayed, err, again.LinkedVoucher)
	}
	events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID())
	if len(events) != 2 || events[1].Kind != domain.VoucherEventApplied || events[1].DepositID != first.Deposit.ID() {
		t.Fatalf("history = %+v", events)
	}
}

// B then A. Registered by hand, the pupil's single pending voucher links
// itself; applying that voucher afterwards is a 409 naming the receipt.
func TestManualDepositAutoLinksTheOnlyPendingVoucher(t *testing.T) {
	f := newVLFamily(t, 3)
	v := f.voucher(t, 3)
	manual, _, err := f.register(t, f.deposit(t, vlID("68200000", 30), 100000, "manual-3", domain.VoucherChoice{}))
	if err != nil {
		t.Fatalf("manual: %v", err)
	}
	if manual.LinkedVoucher == nil || manual.LinkedVoucher.ID() != v.ID() || manual.LinkedVoucher.DepositFolio() != manual.Deposit.Folio() {
		t.Fatalf("linked = %+v", manual.LinkedVoucher)
	}
	events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID())
	if len(events) != 2 || events[1].Kind != domain.VoucherEventAutoLinked {
		t.Fatalf("history = %+v", events)
	}

	_, _, err = f.register(t, f.deposit(t, vlID("68200000", 31), 100000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if !errors.Is(err, domain.ErrVoucherAlreadyApplied) || !strings.Contains(glerrors.SafeMessage(err), manual.Deposit.Folio()) {
		t.Fatalf("apply after manual: want ErrVoucherAlreadyApplied naming %s, got %v", manual.Deposit.Folio(), err)
	}
	if n := countDepositsOf(t, f.student); n != 1 {
		t.Fatalf("stored deposits = %d, want 1", n)
	}
}

// B. Two pending vouchers: the office must choose, and nothing is taken until
// it does; a declined voucher stays pending and is never linked.
func TestManualDepositWithTwoPendingVouchers(t *testing.T) {
	f := newVLFamily(t, 4)
	a := f.voucher(t, 41)
	b := f.voucher(t, 42)

	_, _, err := f.register(t, f.deposit(t, vlID("68200000", 40), 100000, "", domain.VoucherChoice{}))
	if !errors.Is(err, domain.ErrVoucherChoiceRequired) {
		t.Fatalf("two pending: want ErrVoucherChoiceRequired, got %v", err)
	}
	if n := countDepositsOf(t, f.student); n != 0 {
		t.Fatalf("nothing is taken while the office has to choose: %d deposits", n)
	}

	record, _, err := f.register(t, f.deposit(t, vlID("68200000", 41), 100000, "", domain.VoucherChoice{Declined: []string{a.ID()}}))
	if err != nil || record.LinkedVoucher == nil || record.LinkedVoucher.ID() != b.ID() {
		t.Fatalf("declined a: err=%v linked=%+v", err, record.LinkedVoucher)
	}
	if got := f.reread(t, a.ID()); got.Status() != domain.VoucherPending {
		t.Fatalf("the declined voucher stays pending: %+v", got)
	}

	// Declining the last one links nothing.
	record, _, err = f.register(t, f.deposit(t, vlID("68200000", 42), 100000, "", domain.VoucherChoice{Declined: []string{a.ID()}}))
	if err != nil || record.LinkedVoucher != nil {
		t.Fatalf("declined the only pending: err=%v linked=%+v", err, record.LinkedVoucher)
	}
	if got := f.reread(t, a.ID()); got.Status() != domain.VoucherPending {
		t.Fatalf("still pending: %+v", got)
	}
}

// Never across pupils: another pupil's voucher is NotFound by id, and a manual
// deposit never looks at anybody else's queue.
func TestVoucherNeverCrossesPupils(t *testing.T) {
	mine := newVLFamily(t, 5)
	theirs := newVLFamily(t, 6)
	v := theirs.voucher(t, 6)

	_, _, err := mine.register(t, mine.deposit(t, vlID("68200000", 50), 100000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if !errors.Is(err, domain.ErrVoucherNotOwned) || !isNotFound(err) {
		t.Fatalf("another pupil's voucher: want ErrVoucherNotOwned, got %v", err)
	}
	for _, bad := range []string{"not-a-uuid", vlID("b8000000", 999)} {
		_, _, err := mine.register(t, mine.deposit(t, vlID("68200000", 51), 100000, domain.VoucherIdempotencyKey(bad, 0), domain.VoucherChoice{VoucherID: bad}))
		if !isNotFound(err) {
			t.Fatalf("voucher %q: want NotFound, got %v", bad, err)
		}
	}
	record, _, err := mine.register(t, mine.deposit(t, vlID("68200000", 52), 100000, "", domain.VoucherChoice{}))
	if err != nil || record.LinkedVoucher != nil {
		t.Fatalf("manual deposit for another pupil: err=%v linked=%+v", err, record.LinkedVoucher)
	}
	if got := theirs.reread(t, v.ID()); got.Status() != domain.VoucherPending {
		t.Fatalf("their voucher is untouched: %+v", got)
	}
}

// A rejected voucher pays nothing.
func TestApplyRejectedVoucher(t *testing.T) {
	f := newVLFamily(t, 7)
	v := f.voucher(t, 7)
	if _, err := f.repo.UpdatePaymentVoucher(context.Background(), v.ID(), change(domain.VoucherEventRejected,
		func(v domain.PaymentVoucher) (domain.PaymentVoucher, error) {
			return v.Reject("ilegible", vchCashier, vchDay)
		})); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.register(t, f.deposit(t, vlID("68200000", 70), 100000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if !errors.Is(err, domain.ErrVoucherRejected) {
		t.Fatalf("want ErrVoucherRejected, got %v", err)
	}
	if n := countDepositsOf(t, f.student); n != 0 {
		t.Fatalf("stored deposits = %d", n)
	}
}

// A key that is not the voucher's current one (it was released in between) is
// refused rather than stored.
func TestApplyVoucherWithStaleKey(t *testing.T) {
	f := newVLFamily(t, 8)
	v := f.voucher(t, 8)
	_, _, err := f.register(t, f.deposit(t, vlID("68200000", 80), 100000, domain.VoucherIdempotencyKey(v.ID(), 3), domain.VoucherChoice{VoucherID: v.ID()}))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("stale key: want KindConflict, got %v", err)
	}
	if n := countDepositsOf(t, f.student); n != 0 {
		t.Fatalf("stored deposits = %d", n)
	}
}

// A partial transfer applied by voucher leaves the balance as any deposit does.
func TestApplyVoucherPartialLeavesTheBalance(t *testing.T) {
	f := newVLFamily(t, 9)
	v := f.voucher(t, 9)
	if _, _, err := f.register(t, f.deposit(t, vlID("68200000", 90), 40000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()})); err != nil {
		t.Fatal(err)
	}
	sep, _ := f.repo.GetCharge(context.Background(), f.sep.ID())
	if sep.PaidCents != 40000 || sep.Charge.Status() != domain.StatusPartiallyPaid {
		t.Fatalf("september = %d %s", sep.PaidCents, sep.Charge.Status())
	}
	if got := f.reread(t, v.ID()); got.Status() != domain.VoucherApplied {
		t.Fatalf("voucher = %+v", got)
	}
}

// C. A void hands the voucher back to the queue; it can then be applied again
// under a key of its own, which is a NEW deposit, not a replay of the voided.
func TestVoidReleasesTheVoucher(t *testing.T) {
	f := newVLFamily(t, 10)
	v := f.voucher(t, 10)
	first, _, err := f.register(t, f.deposit(t, vlID("68200000", 100), 100000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if err != nil {
		t.Fatal(err)
	}
	voided, err := first.Deposit.Void("se registró al alumno equivocado", vchDay.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.VoidDeposit(context.Background(), voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}
	released := f.reread(t, v.ID())
	if released.Status() != domain.VoucherPending || released.DepositID() != "" || released.Releases() != 1 {
		t.Fatalf("released = %+v", released)
	}
	events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID())
	last := events[len(events)-1]
	if last.Kind != domain.VoucherEventReleasedByVoid || last.DepositID != first.Deposit.ID() || last.Reason != "se registró al alumno equivocado" {
		t.Fatalf("last event = %+v", last)
	}

	// The old key replays the voided money and must be refused, not answered.
	if _, _, err := f.register(t, f.deposit(t, vlID("68200000", 101), 100000, first.Deposit.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()})); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("old key after the void: want KindConflict, got %v", err)
	}
	again, replayed, err := f.register(t, f.deposit(t, vlID("68200000", 102), 100000, released.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if err != nil || replayed || again.Deposit.ID() == first.Deposit.ID() || again.LinkedVoucher == nil {
		t.Fatalf("re-apply: replayed=%v err=%v deposit=%s", replayed, err, again.Deposit.ID())
	}
	if got := f.reread(t, v.ID()); got.DepositID() != again.Deposit.ID() {
		t.Fatalf("voucher points at %s, want %s", got.DepositID(), again.Deposit.ID())
	}
}

// C. A correction moves the voucher onto the deposit that replaces the
// original, so the voucher always points at live money.
func TestCorrectionMovesTheVoucher(t *testing.T) {
	f := newVLFamily(t, 11)
	v := f.voucher(t, 11)
	original, _, err := f.register(t, f.deposit(t, vlID("68200000", 110), 40000, v.IdempotencyKey(), domain.VoucherChoice{VoucherID: v.ID()}))
	if err != nil {
		t.Fatal(err)
	}
	corrected := f.deposit(t, vlID("68200000", 111), 100000, "corr-vl-11", domain.VoucherChoice{}).WithCorrects(original.Deposit.ID(), "")
	record, _, err := f.repo.CorrectDeposit(context.Background(), ports.DepositCorrectionWrite{
		OriginalID: original.Deposit.ID(), Corrected: corrected, CorrectionID: vlID("68300000", 111),
		Reason: "Se capturó $400 y fueron $1,000", By: vchCashier, At: vchDay.Add(13 * time.Hour),
	}, vlSplit(t, 100000), nil)
	if err != nil {
		t.Fatalf("CorrectDeposit: %v", err)
	}
	moved := f.reread(t, v.ID())
	if moved.Status() != domain.VoucherApplied || moved.DepositID() != corrected.ID() || moved.DepositFolio() != record.Corrected.Deposit.Folio() {
		t.Fatalf("moved = %+v", moved)
	}
	if record.Corrected.LinkedVoucher == nil || record.Corrected.LinkedVoucher.ID() != v.ID() {
		t.Fatalf("the corrected record names its voucher: %+v", record.Corrected.LinkedVoucher)
	}
	events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID())
	last := events[len(events)-1]
	if last.Kind != domain.VoucherEventMovedByCorrection || last.DepositID != corrected.ID() || last.By != vchCashier {
		t.Fatalf("last event = %+v", last)
	}
}

// A deposit with no voucher at all still voids and corrects as before.
func TestVoidWithoutVoucher(t *testing.T) {
	f := newVLFamily(t, 12)
	plain, _, err := f.register(t, f.deposit(t, vlID("68200000", 120), 100000, "", domain.VoucherChoice{}))
	if err != nil || plain.LinkedVoucher != nil {
		t.Fatalf("plain: %v %+v", err, plain.LinkedVoucher)
	}
	voided, _ := plain.Deposit.Void("prueba", vchDay.Add(time.Hour))
	if err := f.repo.VoidDeposit(context.Background(), voided); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}
}
