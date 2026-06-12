package errors_test

import (
	"fmt"
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
