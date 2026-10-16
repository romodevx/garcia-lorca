package pubsubadapter

import (
	"context"
	"log/slog"
	"strings"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"google.golang.org/protobuf/proto"
)

// typeChargeSettled is the only event students reacts to. The payment topic also
// carries PaymentRegistered, which fires on partial collections too — and half an
// inscription does not enrol anybody.
const typeChargeSettled = "ChargeSettled"

// inscriptionRefPrefix is how an inscription charge is recognised without asking
// finance anything. The reference is minted as `enrollment:<cycle>:<student>`
// (finance's billing run), and ChargeSettled echoes it precisely so a consumer
// can resolve its own row without a lookup back.
const inscriptionRefPrefix = "enrollment:"

// InscriptionBiller is the seam the consumer drives: make a pupil "inscrito"
// because their inscription was paid.
type InscriptionBiller interface {
	MarkInscriptionPaid(ctx context.Context, studentID string) error
}

// Consumer turns a settled inscription into an enrolled pupil.
//
// It is the school's rule made real: a new pupil is en trámite, and inscrito only
// once the inscription is paid. The two facts live in different services — who
// the pupil is here, whether the money arrived in finance — so the event is what
// joins them.
//
// It keeps NO idempotency ledger. A redelivery finds the pupil already enrolled
// and does nothing, as does a terminal admission, which is cheaper and simpler
// than a second mechanism guarding properties the state machine already has.
type Consumer struct {
	biller InscriptionBiller
	log    *slog.Logger
}

// NewConsumer wires the consumer to the use case.
func NewConsumer(biller InscriptionBiller, log *slog.Logger) *Consumer {
	return &Consumer{biller: biller, log: log}
}

// Handle processes one message. Anything it cannot act on is ACKED rather than
// redelivered forever: an event students has no opinion about is not a failure,
// and neither is a malformed one that no number of retries will improve.
func (c *Consumer) Handle(ctx context.Context, msg pubsub.Message) error {
	if msg.Attributes[attrType] != typeChargeSettled {
		return nil
	}
	var ev financev1.ChargeSettled
	if err := proto.Unmarshal(msg.Data, &ev); err != nil {
		c.log.Warn("students dropped an unreadable ChargeSettled", slog.Any("error", err))
		return nil
	}
	if !strings.HasPrefix(ev.GetExternalRef(), inscriptionRefPrefix) {
		// A colegiatura, a uniform, a late fee. Paying any of those does not
		// change a pupil's admission.
		return nil
	}

	// A real failure is NOT acked, so Pub/Sub redelivers. Safe to retry: marking
	// an already-enrolled pupil is a no-op.
	if err := c.biller.MarkInscriptionPaid(ctx, ev.GetStudentId()); err != nil {
		return err
	}
	c.log.Info("students enrolled a pupil whose inscription was paid",
		slog.String("student_id", ev.GetStudentId()))
	return nil
}
