package grpcadapter

import (
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// Confirmation review (gap 3): a plan write refused because its level is being
// deleted reaches the gateway as a conflict carrying the machine code, which
// the gateway forwards as X-Error-Code.
func TestConnectErrCarriesTheLevelBeingDeletedCode(t *testing.T) {
	ce := connectErr(fmt.Errorf("tuition plan: %w", levelarchive.LevelBeingDeleted()))
	if connect.CodeOf(ce) != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want AlreadyExists (409)", connect.CodeOf(ce))
	}
	if got := ce.Meta().Get(errorCodeMeta); got != levelarchive.CodeLevelBeingDeleted {
		t.Errorf("X-Error-Code = %q, want %q", got, levelarchive.CodeLevelBeingDeleted)
	}
}
