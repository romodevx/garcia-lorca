package pubsubadapter_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	studentspubsub "github.com/garcia-lorca/services/students/internal/adapters/pubsub"
	"google.golang.org/protobuf/proto"
)

type spyBiller struct {
	calls int
	err   error
}

func (s *spyBiller) MarkInscriptionPaid(_ context.Context, _ string) error {
	s.calls++
	return s.err
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func settled(t *testing.T, ref string) pubsub.Message {
	t.Helper()
	data, err := proto.Marshal(&financev1.ChargeSettled{StudentId: "stu-1", ExternalRef: ref})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return pubsub.Message{Attributes: map[string]string{"type": "ChargeSettled"}, Data: data}
}

func TestPayingTheInscriptionEnrolsThePupil(t *testing.T) {
	biller := &spyBiller{}
	c := studentspubsub.NewConsumer(biller, quiet())

	if err := c.Handle(context.Background(), settled(t, "enrollment:cyc-1:stu-1")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if biller.calls != 1 {
		t.Errorf("biller called %d times, want 1", biller.calls)
	}
}

// Paying a colegiatura, a uniform or a late fee changes nothing about admission.
// Reacting to every settled charge would enrol a pupil for buying a jumper.
func TestPayingAnythingElseChangesNothing(t *testing.T) {
	for _, ref := range []string{
		"tuition:cyc-1:stu-1:2026-09",
		"latefee:chg-1:2026-10",
		"uniform-sale:abc",
		"",
	} {
		t.Run(ref, func(t *testing.T) {
			biller := &spyBiller{}
			c := studentspubsub.NewConsumer(biller, quiet())

			if err := c.Handle(context.Background(), settled(t, ref)); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if biller.calls != 0 {
				t.Errorf("a %q payment enrolled somebody", ref)
			}
		})
	}
}

// PaymentRegistered fires on PARTIAL collections too, and half an inscription
// does not enrol anybody.
func TestOnlyASettledChargeCounts(t *testing.T) {
	biller := &spyBiller{}
	c := studentspubsub.NewConsumer(biller, quiet())

	msg := settled(t, "enrollment:cyc-1:stu-1")
	msg.Attributes["type"] = "PaymentRegistered"

	if err := c.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if biller.calls != 0 {
		t.Error("a partial payment enrolled somebody")
	}
}

func TestAnUnreadableMessageIsAcked(t *testing.T) {
	c := studentspubsub.NewConsumer(&spyBiller{}, quiet())
	msg := pubsub.Message{Attributes: map[string]string{"type": "ChargeSettled"}, Data: []byte("not protobuf")}

	if err := c.Handle(context.Background(), msg); err != nil {
		t.Errorf("an unreadable message must be acked, not retried forever: %v", err)
	}
}

func TestARealFailureIsRedelivered(t *testing.T) {
	c := studentspubsub.NewConsumer(&spyBiller{err: errors.New("db down")}, quiet())

	if err := c.Handle(context.Background(), settled(t, "enrollment:cyc-1:stu-1")); err == nil {
		t.Error("a failure to enrol must not be acked away")
	}
}
