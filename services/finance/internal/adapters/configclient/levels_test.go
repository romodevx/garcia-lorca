package configclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/finance/internal/adapters/configclient"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// levelsStub answers ListLevels with fixed levels.
type levelsStub struct {
	configv1connect.UnimplementedConfigServiceHandler
	levels []*configv1.Level
}

func (s *levelsStub) ListLevels(context.Context, *connect.Request[configv1.ListLevelsRequest]) (*connect.Response[configv1.ListLevelsResponse], error) {
	return connect.NewResponse(&configv1.ListLevelsResponse{Levels: s.levels}), nil
}

// Confirmation review (gap 3): config's level status travels with the level
// list — the lookup a tuition plan write makes.
func TestListLevelsCarriesTheDeletingStatus(t *testing.T) {
	stub := &levelsStub{levels: []*configv1.Level{
		{Id: "lvl-1", Name: "Primaria", Status: configv1.LevelStatus_LEVEL_STATUS_DELETING},
		{Id: "lvl-2", Name: "Secundaria", Status: configv1.LevelStatus_LEVEL_STATUS_ACTIVE},
		{Id: "lvl-3", Name: "Maternal"},
	}}
	mux := http.NewServeMux()
	mux.Handle(configv1connect.NewConfigServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := configclient.New(srv.URL, srv.Client()).ListLevels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []ports.LevelRef{
		{ID: "lvl-1", Name: "Primaria", Deleting: true},
		{ID: "lvl-2", Name: "Secundaria"},
		{ID: "lvl-3", Name: "Maternal"},
	}, got)
}
