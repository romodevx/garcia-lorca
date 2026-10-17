package grpcadapter_test

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func (f *fakeCatalog) RenameCycle(_ context.Context, c domain.Cycle) error {
	if _, ok := f.cycles[c.ID()]; !ok {
		return glerrors.ErrNotFound
	}
	f.cycles[c.ID()] = c
	return nil
}

func TestRenameCycle(t *testing.T) {
	h, _ := newHandler(t)
	id := seedCycle(t, h)

	res, err := h.RenameCycle(ctx(), connect.NewRequest(&configv1.RenameCycleRequest{Id: id, Name: " 2026-2027 B "}))

	require.NoError(t, err)
	assert.Equal(t, id, res.Msg.GetCycle().GetId())
	assert.Equal(t, "2026-2027 B", res.Msg.GetCycle().GetName())
}

func TestRenameCycleRefusals(t *testing.T) {
	cases := map[string]struct {
		id, name string
		code     connect.Code
		meta     string
	}{
		"blank name":    {name: "  ", code: connect.CodeInvalidArgument, meta: string(domain.CodeCycleNameRequired)},
		"unknown cycle": {id: "nope", name: "x", code: connect.CodeNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := newHandler(t)
			id := seedCycle(t, h)
			if tc.id != "" {
				id = tc.id
			}
			_, err := h.RenameCycle(ctx(), connect.NewRequest(&configv1.RenameCycleRequest{Id: id, Name: tc.name}))
			var ce *connect.Error
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, tc.code, ce.Code())
			assert.Equal(t, tc.meta, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
		})
	}
}
