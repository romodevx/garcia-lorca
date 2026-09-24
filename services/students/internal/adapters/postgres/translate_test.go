package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// A CHECK violation is bad input, not a server fault: an API client that sends
// a value the schema forbids gets a 400, never a 500.
func TestTranslateGuardianWriteErrMapsCheckViolationToInvalid(t *testing.T) {
	err := translateGuardianWriteErr(&pgconn.PgError{Code: "23514"})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", glerrors.KindOf(err))
	}
}
