package configclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/adapters/configclient"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// stubCalendar counts reads so the cache's whole claim — that it stops making
// them — is observable.
type stubCalendar struct {
	calls int
	days  []time.Time
	err   error
	block time.Duration
}

func (s *stubCalendar) NonWorkingDays(ctx context.Context, _ string) (ports.CalendarSnapshot, error) {
	s.calls++
	if s.block > 0 {
		select {
		case <-time.After(s.block):
		case <-ctx.Done():
			return ports.CalendarSnapshot{}, ctx.Err()
		}
	}
	if s.err != nil {
		return ports.CalendarSnapshot{}, s.err
	}
	return ports.CalendarSnapshot{Days: s.days}, nil
}

var sep16 = time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

// Every payment would otherwise cost a round trip to config to ask what days the
// school is shut — an answer that changes a few times a year.
func TestCalendarIsReadOncePerTTL(t *testing.T) {
	stub := &stubCalendar{days: []time.Time{sep16}}
	cal := configclient.NewCachedCalendar(stub, time.Minute, time.Second)

	for i := 0; i < 5; i++ {
		if _, err := cal.NonWorkingDays(context.Background(), "c1"); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if stub.calls != 1 {
		t.Errorf("config was asked %d times, want 1 — the cache is not holding", stub.calls)
	}
}

// The version is what gets stamped on a payment, so a recomputation months later
// can tell whether it is looking at the same calendar the cashier saw.
func TestSnapshotCarriesAVersion(t *testing.T) {
	stub := &stubCalendar{days: []time.Time{sep16}}
	cal := configclient.NewCachedCalendar(stub, time.Minute, time.Second)

	snap, err := cal.NonWorkingDays(context.Background(), "c1")
	if err != nil {
		t.Fatalf("NonWorkingDays: %v", err)
	}
	if snap.Version == "" {
		t.Error("snapshot has no version — a payment could not record which calendar it used")
	}

	other := configclient.NewCachedCalendar(&stubCalendar{days: []time.Time{sep16.AddDate(0, 0, 1)}}, time.Minute, time.Second)
	otherSnap, err := other.NonWorkingDays(context.Background(), "c1")
	if err != nil {
		t.Fatalf("NonWorkingDays: %v", err)
	}
	if otherSnap.Version == snap.Version {
		t.Error("two different calendars share a version — the stamp says nothing")
	}
}

// A cashier is standing in front of a parent. If config is slow, the payment
// does not wait on it: the read is cut off and the last calendar we hold is used.
func TestASlowConfigDoesNotHoldUpAPayment(t *testing.T) {
	stub := &stubCalendar{days: []time.Time{sep16}}
	cal := configclient.NewCachedCalendar(stub, time.Millisecond, 20*time.Millisecond)
	if _, err := cal.NonWorkingDays(context.Background(), "c1"); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}

	stub.block = 2 * time.Second
	time.Sleep(2 * time.Millisecond) // let the entry go stale

	start := time.Now()
	snap, err := cal.NonWorkingDays(context.Background(), "c1")
	if err != nil {
		t.Fatalf("NonWorkingDays under a slow config: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v on config — the timeout did not fire", elapsed)
	}
	if len(snap.Days) != 1 || !snap.Days[0].Equal(sep16) {
		t.Errorf("did not fall back to the calendar we already had: %+v", snap.Days)
	}
	if !snap.Stale {
		t.Error("a served-from-stale snapshot must say so, or the payment records a calendar it did not read")
	}
}

// With nothing cached and config down there is no honest answer but "weekends
// only", and the snapshot has to admit it rather than pass for a real calendar.
func TestNoCacheAndNoConfigDegradesHonestly(t *testing.T) {
	stub := &stubCalendar{err: errors.New("config is down")}
	cal := configclient.NewCachedCalendar(stub, time.Minute, 20*time.Millisecond)

	snap, err := cal.NonWorkingDays(context.Background(), "c1")
	if err != nil {
		t.Fatalf("NonWorkingDays: %v", err)
	}
	if len(snap.Days) != 0 {
		t.Errorf("invented %d closed days out of nothing", len(snap.Days))
	}
	if !snap.Stale || snap.Version != configclient.VersionUnavailable {
		t.Errorf("degraded snapshot does not admit it: %+v", snap)
	}
}
