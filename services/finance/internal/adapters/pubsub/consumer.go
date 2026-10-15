package pubsubadapter

import (
	"context"
	"log/slog"
	"time"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"google.golang.org/protobuf/proto"
)

const (
	typeStudentEnrolled  = "StudentEnrolled"
	typeStudentWithdrawn = "StudentWithdrawn"
)

// EnrollmentBiller issues an inscription for an enrolled pupil.
type EnrollmentBiller interface {
	EnsureEnrollmentCharge(ctx context.Context, cycleID, studentID string) (bool, error)
}

// WithdrawalBiller applies a dated student withdrawal to the finance ledger.
type WithdrawalBiller interface {
	ApplyWithdrawal(ctx context.Context, studentID, cycleID, withdrawnOn string) error
}

// StudentEventBiller is the set of student lifecycle actions finance consumes.
type StudentEventBiller interface {
	EnrollmentBiller
	WithdrawalBiller
}

// Consumer turns student lifecycle events into finance ledger actions.
// StudentEnrolled is idempotent through the charge external ref; withdrawal is
// idempotent through guarded cancellation and the activity end date.
type Consumer struct {
	biller StudentEventBiller
	log    *slog.Logger
}

// NewConsumer wires the consumer to both lifecycle use cases.
func NewConsumer(biller StudentEventBiller, log *slog.Logger) *Consumer {
	return &Consumer{biller: biller, log: log}
}

// Handle processes one message. Unknown and malformed events are ACKED, while
// transient failures are returned so Pub/Sub redelivers them.
func (c *Consumer) Handle(ctx context.Context, msg pubsub.Message) error {
	switch msg.Attributes[attrType] {
	case typeStudentEnrolled:
		return c.handleEnrolled(ctx, msg.Data)
	case typeStudentWithdrawn:
		return c.handleWithdrawn(ctx, msg.Data)
	default:
		return nil
	}
}

func (c *Consumer) handleEnrolled(ctx context.Context, data []byte) error {
	var ev studentsv1.StudentEnrolled
	if err := proto.Unmarshal(data, &ev); err != nil {
		c.log.Warn("finance dropped an unreadable StudentEnrolled", slog.Any("error", err))
		return nil
	}
	created, err := c.biller.EnsureEnrollmentCharge(ctx, ev.GetCycleId(), ev.GetStudentId())
	if err != nil {
		return err
	}
	if created {
		c.log.Info("finance charged an inscription on enrolment",
			slog.String("student_id", ev.GetStudentId()), slog.String("cycle_id", ev.GetCycleId()))
	}
	return nil
}

func (c *Consumer) handleWithdrawn(ctx context.Context, data []byte) error {
	var ev studentsv1.StudentWithdrawn
	if err := proto.Unmarshal(data, &ev); err != nil {
		c.log.Warn("finance dropped an unreadable StudentWithdrawn", slog.Any("error", err))
		return nil
	}
	if ev.GetStudentId() == "" || ev.GetCycleId() == "" {
		c.log.Warn("finance dropped an incomplete StudentWithdrawn")
		return nil
	}
	if _, err := time.Parse("2006-01-02", ev.GetWithdrawnOn()); err != nil {
		c.log.Warn("finance dropped a StudentWithdrawn with an invalid date", slog.Any("error", err))
		return nil
	}
	return c.biller.ApplyWithdrawal(ctx, ev.GetStudentId(), ev.GetCycleId(), ev.GetWithdrawnOn())
}
