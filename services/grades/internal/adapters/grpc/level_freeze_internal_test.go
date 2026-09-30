package grpcadapter

import (
	"errors"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// Review finding 4: a write refused because its level is being deleted
// reaches the gateway as a 409 carrying LEVEL_BEING_DELETED, so the screen
// can say "Este nivel se está eliminando" instead of a generic failure.
func TestConnectErrCarriesTheLevelBeingDeletedCode(t *testing.T) {
	ce := connectErr(fmt.Errorf("capture: %w", levelarchive.LevelBeingDeleted()))
	assert.Equal(t, connect.CodeAlreadyExists, ce.Code())
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, ce.Meta().Get(ErrorCodeMeta))
	assert.Equal(t, "Este nivel se está eliminando", ce.Message())

	plain := connectErr(errors.New("boom"))
	assert.Empty(t, plain.Meta().Get(ErrorCodeMeta))
}
