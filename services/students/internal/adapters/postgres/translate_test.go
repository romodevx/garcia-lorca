package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// An unnormalized email refused by the CHECK is bad input, not a server fault:
// the API client gets a 400, never a 500.
func TestTranslateGuardianWriteErrMapsTheEmailCheckToInvalid(t *testing.T) {
	err := translateGuardianWriteErr(&pgconn.PgError{Code: "23514", ConstraintName: "guardian_students_email_normalized_ck"})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", glerrors.KindOf(err))
	}
}

// Any OTHER CHECK can only fire when the server itself is inconsistent (the
// domain already enforces those rules): it stays an internal error that alerts.
func TestTranslateGuardianWriteErrKeepsOtherChecksInternal(t *testing.T) {
	err := translateGuardianWriteErr(&pgconn.PgError{Code: "23514", ConstraintName: "some_status_check"})
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("kind = %v, want KindInternal", glerrors.KindOf(err))
	}
}
