package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

var (
	readyAt   = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	deadline  = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	settledAt = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
)

func picking(t *testing.T) domain.Fulfillment {
	t.Helper()
	f, err := domain.NewFulfillment("ful-1", "t-1", "ord-1", 1, 89000)
	if err != nil {
		t.Fatalf("NewFulfillment: %v", err)
	}
	return f
}

// A delivery is born mid-transition on purpose: the row exists before the charge
// does, so a crash between the two leaves something to resume rather than a
// charge nobody owns.
func TestNewFulfillmentStartsPicking(t *testing.T) {
	f := picking(t)
	if f.Status() != domain.StatusPicking {
		t.Errorf("status = %q, want picking", f.Status())
	}
	if f.Folio("U-2026-000123") != "U-2026-000123-A" {
		t.Errorf("folio = %q, want the -A suffix", f.Folio("U-2026-000123"))
	}
}

// TestFulfillmentFolioSuffixPastTwentySix guards the reason seq is a number and
// not a letter: an order is not capped at 26 deliveries.
func TestFulfillmentFolioSuffixPastTwentySix(t *testing.T) {
	f, err := domain.NewFulfillment("ful-27", "t-1", "ord-1", 27, 1000)
	if err != nil {
		t.Fatalf("NewFulfillment: %v", err)
	}
	if got := f.Folio("U-2026-000123"); got != "U-2026-000123-AA" {
		t.Errorf("27th delivery folio = %q, want the AA suffix", got)
	}
}

func TestFulfillmentHappyPath(t *testing.T) {
	f := picking(t)

	ready, err := f.MarkReady("chg-1", readyAt, deadline, false)
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if ready.Status() != domain.StatusReadyForPickup || ready.FinanceChargeID() != "chg-1" {
		t.Fatalf("after MarkReady: %+v", ready)
	}

	paid, err := ready.MarkPaid(settledAt)
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if paid.Status() != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", paid.Status())
	}

	dispatched, err := paid.Dispatch(settledAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatched.Status() != domain.StatusDispatched {
		t.Errorf("status = %q, want dispatched", dispatched.Status())
	}
}

// TestFulfillmentForbiddenTransitions is the table that keeps the module honest.
// Every one of these is a way to lose money or lose goods, and none of them is
// reachable by asking the aggregate.
func TestFulfillmentForbiddenTransitions(t *testing.T) {
	ready := mustReady(t, picking(t))
	paid := mustPaid(t, ready)
	dispatched := mustDispatched(t, paid)
	expired := mustExpired(t, ready)
	cancelled := mustCancelled(t, picking(t))
	counter := mustCounterSale(t)

	cases := []struct {
		name string
		call func() error
	}{
		{"picking cannot be paid before its charge exists", func() error {
			_, err := picking(t).MarkPaid(settledAt)
			return err
		}},
		{"picking cannot be dispatched", func() error {
			_, err := picking(t).Dispatch(settledAt)
			return err
		}},
		{"ready cannot be dispatched before it is paid", func() error {
			_, err := ready.Dispatch(settledAt)
			return err
		}},
		{"paid cannot go back to ready", func() error {
			_, err := paid.MarkReady("chg-2", readyAt, deadline, false)
			return err
		}},
		{"paid cannot expire", func() error {
			_, err := paid.Expire(settledAt)
			return err
		}},
		{"dispatched is terminal", func() error {
			_, err := dispatched.Cancel("changed my mind", settledAt)
			return err
		}},
		{"expired cannot be paid", func() error {
			_, err := expired.MarkPaid(settledAt)
			return err
		}},
		{"expired cannot be dispatched", func() error {
			_, err := expired.Dispatch(settledAt)
			return err
		}},
		{"cancelled is terminal", func() error {
			_, err := cancelled.MarkReady("chg-2", readyAt, deadline, false)
			return err
		}},
		// The one that would let an expiry sweep "return" goods that physically
		// left the storeroom hours earlier.
		{"a counter sale can never expire", func() error {
			_, err := counter.Expire(settledAt)
			return err
		}},
		{"a counter sale cannot be cancelled", func() error {
			_, err := counter.Cancel("oops", settledAt)
			return err
		}},
		{"a counter sale cannot be dispatched again", func() error {
			_, err := counter.Dispatch(settledAt)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); glerrors.KindOf(err) != glerrors.KindConflict {
				t.Errorf("want KindConflict, got %v", err)
			}
		})
	}
}

// TestCounterSaleIsTerminalFromBirth is the shape B4 forced. The goods were
// handed over as they were sold, so there is nothing for any sweep to reclaim
// and no state after this one.
func TestCounterSaleIsTerminalFromBirth(t *testing.T) {
	f := mustCounterSale(t)
	if f.Status() != domain.StatusSoldAtCounter {
		t.Fatalf("status = %q, want sold_at_counter", f.Status())
	}
	// C2-015: the one thing that may follow is a return, and only a person
	// asks for it (ReturnAtCounter); no sweep has a road to it.
	if f.IsTerminal() {
		t.Error("a counter sale may still be returned")
	}
	if f.HoldsStock() {
		t.Error("a counter sale holds nothing: the goods already left")
	}
}

// TestOnlyLiveStatesHoldStock is what the expiry sweep and the panel's counters
// both read. Getting it wrong either strands inventory or double-sells it.
func TestOnlyLiveStatesHoldStock(t *testing.T) {
	cases := []struct {
		status domain.FulfillmentStatus
		holds  bool
	}{
		{domain.StatusPicking, true},
		{domain.StatusReadyForPickup, true},
		{domain.StatusPaid, false}, // the goods were deducted when it settled
		{domain.StatusDispatched, false},
		{domain.StatusExpired, false},
		{domain.StatusCancelled, false},
		{domain.StatusSoldAtCounter, false},
		{domain.StatusReturned, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.HoldsStock(); got != tc.holds {
				t.Errorf("HoldsStock() = %v, want %v", got, tc.holds)
			}
		})
	}
}

// TestExpireReleasesOnlyOnce guards the idempotency the sweep depends on: it runs
// daily, the lazy path runs on every read, and both may reach the same row.
func TestExpireReleasesOnlyOnce(t *testing.T) {
	expired := mustExpired(t, mustReady(t, picking(t)))
	if _, err := expired.Expire(settledAt); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("expiring twice must conflict, got %v", err)
	}
}

// TestMarkPaidAfterExpiryIsFlaggedNotSwallowed: money arrived for goods already
// released. No rule can settle that — the goods may be sold — so it is handed to
// a human rather than absorbed. Silence is the one outcome not allowed.
func TestMarkPaidAfterExpiryIsFlaggedNotSwallowed(t *testing.T) {
	expired := mustExpired(t, mustReady(t, picking(t)))

	flagged, err := expired.FlagForAttention("pago recibido de un apartado vencido")
	if err != nil {
		t.Fatalf("FlagForAttention: %v", err)
	}
	if !flagged.RequiresAttention() {
		t.Error("the delivery was not flagged")
	}
	if flagged.Status() != domain.StatusExpired {
		t.Errorf("flagging must not change the status, got %q", flagged.Status())
	}
	if flagged.AttentionReason() == "" {
		t.Error("a flag with no reason tells the staff nothing")
	}
}

func TestExtendDeadline(t *testing.T) {
	ready := mustReady(t, picking(t))
	longer := deadline.AddDate(0, 0, 7)

	got, err := ready.ExtendTo(longer, "la mamá está de viaje", 2)
	if err != nil {
		t.Fatalf("ExtendTo: %v", err)
	}
	if !got.PickupDeadline().Equal(longer) {
		t.Errorf("deadline = %v, want %v", got.PickupDeadline(), longer)
	}
	if got.ExtensionSeq() != 1 {
		t.Errorf("extension seq = %d, want 1 — the notification's idempotency key depends on it", got.ExtensionSeq())
	}
}

// TestExtendDeadlineNeverShortens: the date was already told to the family. A
// system that walks a promise back is worse than one that never made it.
func TestExtendDeadlineNeverShortens(t *testing.T) {
	ready := mustReady(t, picking(t))
	if _, err := ready.ExtendTo(deadline.AddDate(0, 0, -1), "reason", 2); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid shortening a deadline, got %v", err)
	}
}

func TestExtendDeadlineRespectsTheCeiling(t *testing.T) {
	f := mustReady(t, picking(t))
	var err error
	for i := 1; i <= 2; i++ {
		f, err = f.ExtendTo(deadline.AddDate(0, 0, 7*i), "reason", 2)
		if err != nil {
			t.Fatalf("extension %d: %v", i, err)
		}
	}
	// Without a ceiling a hold never really expires.
	if _, err := f.ExtendTo(deadline.AddDate(0, 0, 30), "reason", 2); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict past the ceiling, got %v", err)
	}
}

func TestExtendDeadlineNeedsAReason(t *testing.T) {
	ready := mustReady(t, picking(t))
	if _, err := ready.ExtendTo(deadline.AddDate(0, 0, 7), "  ", 2); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid with no reason, got %v", err)
	}
}

// --- helpers -----------------------------------------------------------------

func mustReady(t *testing.T, f domain.Fulfillment) domain.Fulfillment {
	t.Helper()
	out, err := f.MarkReady("chg-1", readyAt, deadline, false)
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	return out
}

func mustPaid(t *testing.T, f domain.Fulfillment) domain.Fulfillment {
	t.Helper()
	out, err := f.MarkPaid(settledAt)
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	return out
}

func mustDispatched(t *testing.T, f domain.Fulfillment) domain.Fulfillment {
	t.Helper()
	out, err := f.Dispatch(settledAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	return out
}

func mustExpired(t *testing.T, f domain.Fulfillment) domain.Fulfillment {
	t.Helper()
	out, err := f.Expire(settledAt)
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	return out
}

func mustCancelled(t *testing.T, f domain.Fulfillment) domain.Fulfillment {
	t.Helper()
	out, err := f.Cancel("sin motivo", settledAt)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	return out
}

func mustCounterSale(t *testing.T) domain.Fulfillment {
	t.Helper()
	out, err := domain.NewCounterSaleFulfillment("ful-c", "t-1", "ord-c", 1, 32000, settledAt)
	if err != nil {
		t.Fatalf("NewCounterSaleFulfillment: %v", err)
	}
	return out
}

// TestIsOverdue is the predicate the lazy path on reads and the daily sweep both
// ask. It answers only for deliveries that are actually waiting: a delivery
// already paid, dispatched or expired has no window left to miss, and treating
// one as overdue is how a sweep releases goods that were collected last week.
func TestIsOverdue(t *testing.T) {
	ready := mustReady(t, picking(t))
	dayAfter := deadline.AddDate(0, 0, 1)
	dayBefore := deadline.AddDate(0, 0, -1)

	cases := []struct {
		name string
		f    domain.Fulfillment
		now  time.Time
		want bool
	}{
		{"waiting, still inside the window", ready, dayBefore, false},
		{"waiting, on the deadline itself", ready, deadline, false},
		{"waiting, past the deadline", ready, dayAfter, true},
		{"already paid", mustPaid(t, ready), dayAfter, false},
		{"already dispatched", mustDispatched(t, mustPaid(t, ready)), dayAfter, false},
		{"already expired", mustExpired(t, ready), dayAfter, false},
		{"a counter sale has no window", mustCounterSale(t), dayAfter, false},
		{"still picking, so no clock yet", picking(t), dayAfter, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.IsOverdue(tc.now); got != tc.want {
				t.Errorf("IsOverdue(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestNewFulfillmentValidation(t *testing.T) {
	cases := []struct {
		name              string
		id, tenant, order string
		seq               int
		amount            int64
	}{
		{"blank id", "", "t-1", "ord-1", 1, 100},
		{"blank tenant", "f-1", "", "ord-1", 1, 100},
		{"blank order", "f-1", "t-1", "", 1, 100},
		{"sequence below one", "f-1", "t-1", "ord-1", 0, 100},
		{"negative amount", "f-1", "t-1", "ord-1", 1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewFulfillment(tc.id, tc.tenant, tc.order, tc.seq, tc.amount); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// TestTransitionsRequireTheirTimestamps: a delivery whose paid_at is zero cannot
// be reconciled later, and a ledger that cannot say WHEN is barely a ledger.
func TestTransitionsRequireTheirTimestamps(t *testing.T) {
	ready := mustReady(t, picking(t))
	var zero time.Time

	if _, err := picking(t).MarkReady("chg-1", zero, deadline, false); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("MarkReady with no start: want KindInvalid, got %v", err)
	}
	if _, err := picking(t).MarkReady("", readyAt, deadline, false); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("MarkReady with no charge: want KindInvalid, got %v", err)
	}
	if _, err := ready.MarkPaid(zero); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("MarkPaid with no time: want KindInvalid, got %v", err)
	}
	if _, err := mustPaid(t, ready).Dispatch(zero); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("Dispatch with no time: want KindInvalid, got %v", err)
	}
	if _, err := ready.Expire(zero); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("Expire with no time: want KindInvalid, got %v", err)
	}
	if _, err := ready.Cancel("motivo", zero); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("Cancel with no time: want KindInvalid, got %v", err)
	}
	if _, err := ready.FlagForAttention("   "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("FlagForAttention with no reason: want KindInvalid, got %v", err)
	}
	if _, err := domain.NewCounterSaleFulfillment("f-1", "t-1", "o-1", 1, 100, zero); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("counter sale with no time: want KindInvalid, got %v", err)
	}
}

// TestRestoreDoesNotReplayTheStateMachine is the rule rehydration lives by. A
// stored delivery is a fact about the past: asking the transition table to
// re-approve it would make LOADING a dispatched row fail for something that
// already happened. The table governs what happens NEXT — the only place it can
// prevent anything — so a restored delivery is accepted as it stands and then
// obeys the same guards from there on.
func TestRestoreDoesNotReplayTheStateMachine(t *testing.T) {
	for _, status := range []domain.FulfillmentStatus{
		domain.StatusPicking, domain.StatusReadyForPickup, domain.StatusPaid,
		domain.StatusDispatched, domain.StatusExpired, domain.StatusCancelled,
		domain.StatusSoldAtCounter, domain.StatusReturned,
	} {
		t.Run(string(status), func(t *testing.T) {
			got, err := domain.RestoreFulfillment(domain.FulfillmentSnapshot{
				ID: "ful-1", TenantID: "t-1", OrderID: "ord-1", Seq: 1,
				Status: status, AmountCents: 89000,
			})
			if err != nil {
				t.Fatalf("restoring a %s delivery failed: %v", status, err)
			}
			if got.Status() != status {
				t.Errorf("status = %q, want %q", got.Status(), status)
			}
		})
	}
}

// TestRestoredDeliveryStillObeysTheGuards: rehydration is permissive about the
// past and exactly as strict about the future.
func TestRestoredDeliveryStillObeysTheGuards(t *testing.T) {
	dispatched, err := domain.RestoreFulfillment(domain.FulfillmentSnapshot{
		ID: "ful-1", TenantID: "t-1", OrderID: "ord-1", Seq: 1,
		Status: domain.StatusDispatched, AmountCents: 89000,
	})
	if err != nil {
		t.Fatalf("RestoreFulfillment: %v", err)
	}
	if _, err := dispatched.Cancel("motivo", settledAt); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("a restored dispatched delivery must still be terminal, got %v", err)
	}
}

// TestRestoreRefusesAnUnknownStatus: a status the code does not know is data
// corruption or a half-finished migration. Loading it as if it were fine would
// hand the state machine a state with no transitions and no meaning.
func TestRestoreRefusesAnUnknownStatus(t *testing.T) {
	_, err := domain.RestoreFulfillment(domain.FulfillmentSnapshot{
		ID: "ful-1", TenantID: "t-1", OrderID: "ord-1", Seq: 1,
		Status: domain.FulfillmentStatus("teletransportado"), AmountCents: 1,
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid for an unknown status, got %v", err)
	}
}

// TestRestoreCarriesEverythingBack guards against a field being silently dropped
// in rehydration — the kind of bug that shows up as a deadline that reads as
// "never" three weeks later.
func TestRestoreCarriesEverythingBack(t *testing.T) {
	got, err := domain.RestoreFulfillment(domain.FulfillmentSnapshot{
		ID: "ful-1", TenantID: "t-1", OrderID: "ord-1", Seq: 3,
		Status: domain.StatusReadyForPickup, AmountCents: 89000,
		FinanceChargeID: "chg-1", ReadyAt: readyAt, PickupDeadline: deadline,
		DeadlineEstimated: true, ExtensionSeq: 2,
		RequiresAttention: true, AttentionReason: "pago tardío",
	})
	if err != nil {
		t.Fatalf("RestoreFulfillment: %v", err)
	}
	if got.Seq() != 3 || got.AmountCents() != 89000 || got.FinanceChargeID() != "chg-1" ||
		!got.ReadyAt().Equal(readyAt) || !got.PickupDeadline().Equal(deadline) ||
		!got.DeadlineEstimated() || got.ExtensionSeq() != 2 ||
		!got.RequiresAttention() || got.AttentionReason() != "pago tardío" {
		t.Errorf("a field was lost in rehydration: %+v", got)
	}
}

// C2-015: a counter sale returned whole ends returned, terminal; nothing else
// may be returned, and a return is not repeated.
func TestReturnAtCounter(t *testing.T) {
	returned, err := mustCounterSale(t).ReturnAtCounter()
	if err != nil {
		t.Fatalf("ReturnAtCounter: %v", err)
	}
	if returned.Status() != domain.StatusReturned || !returned.IsTerminal() {
		t.Errorf("status = %q terminal=%v, want returned and terminal", returned.Status(), returned.IsTerminal())
	}
	if _, err := returned.ReturnAtCounter(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("returning twice: want KindConflict, got %v", err)
	}
	if _, err := picking(t).ReturnAtCounter(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("returning an order that was never sold at the counter: want KindConflict, got %v", err)
	}
}
