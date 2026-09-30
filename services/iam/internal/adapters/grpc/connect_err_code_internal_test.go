package grpcadapter

import (
	"errors"
	"testing"

	connect "connectrpc.com/connect"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// A deletion whose provider half failed travels with its own machine code, so
// the screen can say "ya no puede entrar; falta el proveedor" and offer the
// retry instead of claiming nothing happened.
func TestProviderPendingCarriesItsCode(t *testing.T) {
	err := connectErr(glerrors.Wrap(glerrors.KindInternal, "x", errors.Join(domain.ErrDeletionPending, errors.New("503"))))

	if got := err.Meta().Get(errorCodeMeta); got != "DELETION_PENDING" {
		t.Errorf("X-Error-Code = %q, want DELETION_PENDING", got)
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
	if plain := connectErr(glerrors.New(glerrors.KindInternal, "x")); plain.Meta().Get(errorCodeMeta) != "" {
		t.Error("an ordinary failure must carry no code")
	}
}
