package configclient

import (
	"errors"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The adapter's only decision is how a transport failure becomes the seam's error
// vocabulary (the group projection is nothing but existence), so that is what is
// tested here — directly on the helper, no server involved.
func TestMapConfigErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		notFound bool
		wantKind glerrors.Kind
	}{
		{
			name:     "not found becomes the shared sentinel",
			err:      connect.NewError(connect.CodeNotFound, errors.New("group not found")),
			notFound: true,
			wantKind: glerrors.KindNotFound,
		},
		{
			name:     "internal is wrapped as internal",
			err:      connect.NewError(connect.CodeInternal, errors.New("boom")),
			wantKind: glerrors.KindInternal,
		},
		{
			name:     "unavailable is wrapped as internal",
			err:      connect.NewError(connect.CodeUnavailable, errors.New("config is down")),
			wantKind: glerrors.KindInternal,
		},
		{
			name:     "plain transport error is wrapped as internal",
			err:      fmt.Errorf("dial tcp: connection refused"),
			wantKind: glerrors.KindInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapConfigErr(tt.err, "group")
			if got == nil {
				t.Fatalf("mapConfigErr returned nil")
			}
			if errors.Is(got, glerrors.ErrNotFound) != tt.notFound {
				t.Errorf("errors.Is(err, ErrNotFound) = %v, want %v", !tt.notFound, tt.notFound)
			}
			if kind := glerrors.KindOf(got); kind != tt.wantKind {
				t.Errorf("KindOf(err) = %v, want %v", kind, tt.wantKind)
			}
		})
	}
}
