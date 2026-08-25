package handler

import (
	"errors"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// TestConnectErrLeaksNothing is a regression test for a leak that was live in
// every service at once.
//
// glerrors.Error.Error() deliberately concatenates the wrapped cause, which is
// right for a log line. connectErr used to hand the whole error to the client for
// every kind except internal, so a conflict wrapped around a Postgres unique
// violation shipped the constraint name and the SQLSTATE to whoever called — and
// on some constraints the conflicting VALUE with them.
//
// The kind decides the STATUS. It does not decide whether the cause is the
// caller's business, and it never is.
func TestConnectErrLeaksNothing(t *testing.T) {
	driver := errors.New(`ERROR: duplicate key value violates unique constraint "some_table_tenant_id_name_key" (SQLSTATE 23505)`)

	kinds := map[string]glerrors.Kind{
		"invalid":  glerrors.KindInvalid,
		"notFound": glerrors.KindNotFound,
		"conflict": glerrors.KindConflict,
		"internal": glerrors.KindInternal,
	}
	for name, kind := range kinds {
		t.Run(name, func(t *testing.T) {
			err := connectErr(glerrors.Wrap(kind, "the sentence we wrote", driver))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, secret := range []string{"SQLSTATE", "some_table_tenant_id_name_key", "duplicate key"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("%q reached the caller: %s", secret, err.Error())
				}
			}
		})
	}
}

// TestConnectErrKeepsTheUsefulPart: sanitising must not turn every refusal into
// "internal error". The sentence we wrote is the one a person reads.
func TestConnectErrKeepsTheUsefulPart(t *testing.T) {
	err := connectErr(glerrors.Wrap(glerrors.KindConflict, "the sentence we wrote",
		errors.New("ERROR: internal detail (SQLSTATE 23505)")))
	if !strings.Contains(err.Error(), "the sentence we wrote") {
		t.Errorf("the useful part was thrown away too: %s", err.Error())
	}
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", connect.CodeOf(err))
	}
}
