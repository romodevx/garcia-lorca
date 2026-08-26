package pubsubadapter_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	financepubsub "github.com/garcia-lorca/services/finance/internal/adapters/pubsub"
	"google.golang.org/protobuf/proto"
)

type spyBiller struct {
	calls int
	err   error
}

func (s *spyBiller) EnsureEnrollmentCharge(_ context.Context, _, _ string) (bool, error) {
	s.calls++
	return s.err == nil, s.err
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func enrolledMessage(t *testing.T) pubsub.Message {
	t.Helper()
	data, err := proto.Marshal(&studentsv1.StudentEnrolled{StudentId: "stu-1", CycleId: "cyc-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return pubsub.Message{Attributes: map[string]string{"type": "StudentEnrolled"}, Data: data}
}

func TestConsumerBillsTheInscription(t *testing.T) {
	biller := &spyBiller{}
	c := financepubsub.NewConsumer(biller, quietLog())

	if err := c.Handle(context.Background(), enrolledMessage(t)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if biller.calls != 1 {
		t.Errorf("biller called %d times, want 1", biller.calls)
	}
}

// The topic carries more than one event. Reacting to the wrong one would charge
// a family for a change of admission status.
func TestConsumerIgnoresEventsItHasNoOpinionAbout(t *testing.T) {
	biller := &spyBiller{}
	c := financepubsub.NewConsumer(biller, quietLog())

	msg := enrolledMessage(t)
	msg.Attributes["type"] = "AdmissionStatusChanged"

	if err := c.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if biller.calls != 0 {
		t.Error("finance acted on an event that was not an enrolment")
	}
}

// Unreadable is ACKED, not retried: no number of redeliveries improves bad bytes,
// and a message that can never succeed would block the subscription forever.
func TestConsumerAcksAMessageItCannotRead(t *testing.T) {
	biller := &spyBiller{}
	c := financepubsub.NewConsumer(biller, quietLog())

	msg := pubsub.Message{Attributes: map[string]string{"type": "StudentEnrolled"}, Data: []byte("not protobuf at all")}

	if err := c.Handle(context.Background(), msg); err != nil {
		t.Errorf("an unreadable message must be acked, not retried forever: %v", err)
	}
}

// A real failure is NOT acked. Retrying is safe because the charge is keyed by
// its external ref, so a redelivery cannot double-charge.
func TestConsumerLetsARealFailureRedeliver(t *testing.T) {
	biller := &spyBiller{err: errors.New("the ledger is down")}
	c := financepubsub.NewConsumer(biller, quietLog())

	if err := c.Handle(context.Background(), enrolledMessage(t)); err == nil {
		t.Error("a failure to bill must not be acked away")
	}
}
