package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// A CHECK violation is bad input, not a server fault: an API client that sends
// a value the schema forbids gets a 400, never a 500.
func TestTranslateCreateErrMapsCheckViolationToInvalid(t *testing.T) {
	err := translateCreateErr(&pgconn.PgError{Code: "23514"}, "creating teacher", "duplicate")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", glerrors.KindOf(err))
	}
}
