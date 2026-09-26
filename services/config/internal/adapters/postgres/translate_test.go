package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// An unnormalized email refused by the CHECK is bad input, not a server fault:
// the API client gets a 400, never a 500.
func TestTranslateCreateErrMapsTheEmailCheckToInvalid(t *testing.T) {
	err := translateCreateErr(&pgconn.PgError{Code: "23514", ConstraintName: "teachers_email_normalized_ck"}, "creating teacher", "duplicate")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", glerrors.KindOf(err))
	}
}

// Any OTHER CHECK can only fire when the server itself is inconsistent (the
// domain already enforces those rules): it stays an internal error that alerts.
func TestTranslateCreateErrKeepsOtherChecksInternal(t *testing.T) {
	err := translateCreateErr(&pgconn.PgError{Code: "23514", ConstraintName: "some_status_check"}, "creating teacher", "duplicate")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("kind = %v, want KindInternal", glerrors.KindOf(err))
	}
}

// A deadlock or a serialization failure is two writers racing, not a server
// fault: the transaction rolled back whole and the caller should retry, so both
// surface as KindConflict (409), never as a 500 that pages someone.
func TestTranslateCreateErrMapsConcurrencyAbortsToConflict(t *testing.T) {
	for _, code := range []string{"40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			err := translateCreateErr(&pgconn.PgError{Code: code}, "creating teaching load", "duplicate")
			if glerrors.KindOf(err) != glerrors.KindConflict {
				t.Fatalf("kind = %v, want KindConflict", glerrors.KindOf(err))
			}
		})
	}
}
