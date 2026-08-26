package errors_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want glerrors.Kind
	}{
		{"nil is unknown", nil, glerrors.KindUnknown},
		{"sentinel not found", glerrors.ErrNotFound, glerrors.KindNotFound},
		{"sentinel forbidden", glerrors.ErrForbidden, glerrors.KindForbidden},
		{"plain error is internal", fmt.Errorf("boom"), glerrors.KindInternal},
		{
			"wrapped typed error keeps kind",
			fmt.Errorf("loading: %w", glerrors.Wrap(glerrors.KindInvalid, "bad input", nil)),
			glerrors.KindInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := glerrors.KindOf(tt.err); got != tt.want {
				t.Fatalf("KindOf() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWrapUnwrapChain(t *testing.T) {
	cause := glerrors.ErrNotFound
	wrapped := glerrors.Wrap(glerrors.KindInternal, "repo lookup", cause)
	outer := fmt.Errorf("usecase: %w", wrapped)

	if !errorsIs(outer, glerrors.ErrNotFound) {
		t.Fatalf("expected chain to unwrap to ErrNotFound")
	}
}

// errorsIs is a tiny local shim to keep the import list explicit in the test.
func errorsIs(err, target error) bool {
	for {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
		if err == nil {
			return false
		}
	}
}

// TestSafeMessageStopsAtTheTypedMessage exists because Error() deliberately
// includes the wrapped cause — which is right for a log line and wrong for a
// response.
//
// The leak is easy to miss: a handler that maps a Kind to a status and then
// hands the error straight to the client ships whatever the driver said with it.
// A unique-violation message carries the constraint name and the SQLSTATE, and
// on some constraints it carries the conflicting VALUE too.
func TestSafeMessageStopsAtTheTypedMessage(t *testing.T) {
	driver := errors.New(`ERROR: duplicate key value violates unique constraint "uniform_items_tenant_id_code_key" (SQLSTATE 23505)`)
	wrapped := glerrors.Wrap(glerrors.KindConflict, "a garment with that code already exists", driver)

	if got := glerrors.SafeMessage(wrapped); got != "a garment with that code already exists" {
		t.Errorf("SafeMessage = %q; it must stop at the message we wrote", got)
	}
	// Error() still carries everything, because that is what a log wants.
	if !strings.Contains(wrapped.Error(), "SQLSTATE") {
		t.Error("Error() should still carry the cause for logs")
	}
}

func TestSafeMessageOnAPlainError(t *testing.T) {
	// Something that is not one of ours could be carrying anything, so it gets
	// nothing: a caller learning that a host is unreachable learns the host.
	if got := glerrors.SafeMessage(errors.New("dial tcp 10.0.0.7:5432: connect: refused")); got != "internal error" {
		t.Errorf("SafeMessage = %q, want a bare internal error", got)
	}
}

func TestSafeMessageOnNil(t *testing.T) {
	if got := glerrors.SafeMessage(nil); got != "" {
		t.Errorf("SafeMessage(nil) = %q, want empty", got)
	}
}

// TestSafeMessageUnwrapsToTheOutermostTyped: when our own errors nest, the
// outermost one is the one written for this caller.
func TestSafeMessageNested(t *testing.T) {
	inner := glerrors.Wrap(glerrors.KindConflict, "row lock lost", errors.New("pq: deadlock detected"))
	outer := glerrors.Wrap(glerrors.KindConflict, "esa talla ya no está disponible", inner)
	if got := glerrors.SafeMessage(outer); got != "esa talla ya no está disponible" {
		t.Errorf("SafeMessage = %q", got)
	}
}
