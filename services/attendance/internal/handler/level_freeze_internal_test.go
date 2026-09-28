package handler

import (
	"errors"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// Review finding 4: a list refused because its level is being deleted reaches
// the gateway as a 409 carrying LEVEL_BEING_DELETED in X-Error-Code.
func TestConnectErrCarriesTheLevelBeingDeletedCode(t *testing.T) {
	ce := connectErr(fmt.Errorf("save: %w", levelarchive.LevelBeingDeleted()))
	if ce.Code() != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists (409 at the edge)", ce.Code())
	}
	if got := ce.Meta().Get(ErrorCodeMeta); got != levelarchive.CodeLevelBeingDeleted {
		t.Fatalf("X-Error-Code = %q", got)
	}
	if ce.Message() != "Este nivel se está eliminando" {
		t.Fatalf("message = %q", ce.Message())
	}
	if connectErr(errors.New("boom")).Meta().Get(ErrorCodeMeta) != "" {
		t.Fatal("an uncoded error carries no code")
	}
}
