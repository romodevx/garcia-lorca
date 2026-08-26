package pubsubadapter

import (
	"context"
	"log/slog"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"google.golang.org/protobuf/proto"
)

// typeStudentEnrolled is the only event finance reacts to. The student-events
// topic also carries AdmissionStatusChanged, which the ledger has no opinion on.
const typeStudentEnrolled = "StudentEnrolled"

// EnrollmentBiller is the seam the consumer drives: issue a pupil's inscription
// for a cycle, if their level is priced and they do not already owe it.
type EnrollmentBiller interface {
	EnsureEnrollmentCharge(ctx context.Context, cycleID, studentID string) (bool, error)
}

// Consumer turns StudentEnrolled into the pupil's inscription charge.
//
// It keeps NO idempotency ledger, unlike the comms consumer reading the same
// topic. Pub/Sub delivers at least once, and the charge's own external_ref
// (`enrollment:<cycle>:<student>`, unique per school since migration 0003) makes
// a redelivery insert nothing. Adding a ledger would be a second mechanism
// guarding a property the first one already guarantees.
type Consumer struct {
	biller EnrollmentBiller
	log    *slog.Logger
}

// NewConsumer wires the consumer to the use case.
func NewConsumer(biller EnrollmentBiller, log *slog.Logger) *Consumer {
	return &Consumer{biller: biller, log: log}
}

// Handle processes one message. It returns nil for anything it cannot act on, so
// the message is ACKED rather than redelivered forever: an event finance has no
// opinion about is not a failure, and neither is a malformed one that no number
// of retries will improve.
func (c *Consumer) Handle(ctx context.Context, msg pubsub.Message) error {
	if msg.Attributes[attrType] != typeStudentEnrolled {
		return nil
	}
	var ev studentsv1.StudentEnrolled
	if err := proto.Unmarshal(msg.Data, &ev); err != nil {
		c.log.Warn("finance dropped an unreadable StudentEnrolled", slog.Any("error", err))
		return nil
	}

	created, err := c.biller.EnsureEnrollmentCharge(ctx, ev.GetCycleId(), ev.GetStudentId())
	if err != nil {
		// A real failure: NOT acked, so Pub/Sub redelivers. Safe to retry
		// precisely because the charge is keyed by its external ref.
		return err
	}
	if created {
		c.log.Info("finance charged an inscription on enrolment",
			slog.String("student_id", ev.GetStudentId()),
			slog.String("cycle_id", ev.GetCycleId()))
	}
	return nil
}
