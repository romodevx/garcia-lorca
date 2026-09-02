package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// THE BUG THIS EXISTS TO CLOSE.
//
// A payment committed and then published, best-effort. With the broker down the
// event was gone: the family had paid, the money was in the ledger, and students
// never heard — so the pupil stayed "en trámite" forever, with nothing in the
// system able to notice or fix it.
//
// Now the event is queued in the same transaction as the money, so a broker that
// is down costs a delay and not a fact.
func TestAPaymentSurvivesABrokenBroker(t *testing.T) {
	f := depositFixture(t)
	f.publisher.settledErr = errors.New("pub/sub is down")

	got := deposit(t, f, 670000, "2026-10-10")
	if len(got.Allocations) == 0 {
		t.Fatal("the deposit collected nothing")
	}

	// Nothing reached the broker, and the event is still waiting.
	if f.publisher.settledCalls == 0 {
		t.Error("the drain never tried to publish")
	}
	if pending := f.repo.pendingOutbox(); pending == 0 {
		t.Fatal("the event was lost: nothing is waiting to be published")
	}

	// The broker comes back and the next drain gets it out.
	f.publisher.settledErr = nil
	published, err := f.svc.DrainOutbox(context.Background(), 50)
	if err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
	if published == 0 {
		t.Error("the drain published nothing once the broker was back")
	}
	if pending := f.repo.pendingOutbox(); pending != 0 {
		t.Errorf("%d events still waiting after a successful drain", pending)
	}
}

// The ordinary case: the broker is fine and the event goes out immediately, so
// nobody waits on a scheduler for a pupil to be marked inscribed.
func TestAPaymentPublishesStraightAwayWhenTheBrokerIsUp(t *testing.T) {
	f := depositFixture(t)

	deposit(t, f, 670000, "2026-10-10")

	if f.repo.pendingOutbox() != 0 {
		t.Errorf("%d events left waiting though the broker was up", f.repo.pendingOutbox())
	}
	if f.publisher.settledCalls == 0 {
		t.Error("ChargeSettled never went out")
	}
}

// The event is queued with the money. A payment that fails must leave no event
// behind, or a consumer would act on a payment that never happened.
func TestAFailedPaymentQueuesNoEvent(t *testing.T) {
	f := depositFixture(t)

	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash,
		// Future-dated: refused before anything is written.
		PaidAt: f.clock.AddDate(1, 0, 0),
	})
	if err == nil {
		t.Fatal("the deposit was accepted")
	}
	if pending := f.repo.pendingOutbox(); pending != 0 {
		t.Errorf("%d events queued for a payment that never happened", pending)
	}
}

// A drain with nothing to do is not an error: the scheduler calls this on a
// timer and an empty queue is the normal answer.
func TestDrainingAnEmptyOutboxIsFine(t *testing.T) {
	f := newFixture(t)

	published, err := f.svc.DrainOutbox(context.Background(), 50)
	if err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
	if published != 0 {
		t.Errorf("published = %d, want 0", published)
	}
}

var _ = ports.OutboxChargeSettled
var _ = time.Now
