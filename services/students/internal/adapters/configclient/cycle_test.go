package configclient_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

type stubCycleConfig struct {
	stubConfigService
	cycle *configv1.Cycle
}

func (s *stubCycleConfig) GetCycle(_ context.Context, _ *connect.Request[configv1.GetCycleRequest]) (*connect.Response[configv1.GetCycleResponse], error) {
	if s.cycle == nil {
		return nil, connect.NewError(connect.CodeNotFound, glerrors.New(glerrors.KindNotFound, "cycle not found"))
	}
	return connect.NewResponse(&configv1.GetCycleResponse{Cycle: s.cycle}), nil
}

// T9: the cycle's first day bounds a chosen entry day.
func TestCycleByID_MapsStartsOnAndNotFound(t *testing.T) {
	start := time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC)
	c := newClientFor(t, &stubCycleConfig{cycle: &configv1.Cycle{Id: "cyc", StartsOn: timestamppb.New(start)}})
	got, err := c.CycleByID(context.Background(), "cyc")
	if err != nil {
		t.Fatalf("CycleByID: %v", err)
	}
	if got.ID != "cyc" || !got.StartsOn.Equal(start) {
		t.Errorf("mapping mismatch: %+v", got)
	}

	missing := newClientFor(t, &stubCycleConfig{})
	if _, err := missing.CycleByID(context.Background(), "x"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}
