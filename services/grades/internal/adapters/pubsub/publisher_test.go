package pubsubadapter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	pubsubadapter "github.com/garcia-lorca/services/grades/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

type fakeBus struct {
	msgs []pubsub.Message
	err  error
}

func (b *fakeBus) Publish(_ context.Context, msg pubsub.Message) error {
	if b.err != nil {
		return b.err
	}
	b.msgs = append(b.msgs, msg)
	return nil
}

func issued() ports.ReportCardIssuedEvent {
	return ports.ReportCardIssuedEvent{
		ReportCardID: "rc-1", TenantID: "ten-1", StudentID: "stu-1", EnrollmentID: "enr-1",
		CycleID: "cyc-1", TermID: "term-1", TermName: "1.er trimestre", Version: 2,
	}
}

// T7: an issued report card goes out as GradesPublished on grades-events,
// typed and tenant-routed, carrying what comms keys and renders on.
func TestPublishReportCardIssued(t *testing.T) {
	bus := &fakeBus{}
	require.NoError(t, pubsubadapter.NewEventPublisher(bus).PublishReportCardIssued(context.Background(), issued()))
	require.Len(t, bus.msgs, 1)
	msg := bus.msgs[0]
	assert.Equal(t, "grades-events", msg.Topic)
	assert.Equal(t, "GradesPublished", msg.Attributes[pubsub.AttrType])
	assert.Equal(t, "ten-1", msg.Attributes["tenant_id"])

	var ev gradesv1.GradesPublished
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	assert.Equal(t, "rc-1", ev.GetReportCardId())
	assert.Equal(t, "stu-1", ev.GetStudentId())
	assert.Equal(t, "enr-1", ev.GetEnrollmentId())
	assert.Equal(t, "ten-1", ev.GetTenantId())
	assert.Equal(t, "cyc-1", ev.GetCycleId())
	assert.Equal(t, "term-1", ev.GetTermId())
	assert.Equal(t, "1.er trimestre", ev.GetTermName())
	assert.Equal(t, int32(2), ev.GetVersion())
}

func TestPublishReportCardIssuedSurfacesTransportFailure(t *testing.T) {
	boom := errors.New("broker down")
	err := pubsubadapter.NewEventPublisher(&fakeBus{err: boom}).PublishReportCardIssued(context.Background(), issued())
	assert.ErrorIs(t, err, boom)
}
