package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// Health is the liveness/ping probe present on every service. It adapts the
// config HealthService to the generated Connect Health RPC.
func (h *ConfigHandler) Health(ctx context.Context, _ *connect.Request[configv1.HealthRequest]) (*connect.Response[configv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.HealthResponse{Status: status.Status}), nil
}
