package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// EnsurePlacements finds or creates the catalog of a legacy import (ADR 0033).
// Wired in the next step of the module; until then it answers Unimplemented.
func (h *ConfigHandler) EnsurePlacements(
	_ context.Context, _ *connect.Request[configv1.EnsurePlacementsRequest],
) (*connect.Response[configv1.EnsurePlacementsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("placements not implemented yet"))
}
