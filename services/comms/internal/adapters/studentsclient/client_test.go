package studentsclient

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The adapter makes exactly two decisions — how a transport failure becomes the
// seam's error vocabulary, and which field of a guardian link a push is targeted
// by — so those are what is tested here, directly on the helpers, no server
// involved.

func TestMapStudentsErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantKind glerrors.Kind
	}{
		{
			name:     "internal is wrapped as internal",
			err:      connect.NewError(connect.CodeInternal, errors.New("boom")),
			wantKind: glerrors.KindInternal,
		},
		{
			name:     "unavailable is wrapped as internal",
			err:      connect.NewError(connect.CodeUnavailable, errors.New("students is down")),
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

			got := mapStudentsErr(tt.err)
			if got == nil {
				t.Fatalf("mapStudentsErr returned nil")
			}
			if kind := glerrors.KindOf(got); kind != tt.wantKind {
				t.Errorf("KindOf(err) = %v, want %v", kind, tt.wantKind)
			}
			if errors.Is(got, glerrors.ErrNotFound) {
				t.Error("a transport failure must not read as NotFound: the caller would take it for a student with no guardians")
			}
		})
	}
}

// TestGuardianUserIDsProjectsTheUserIDs pins the projection: a push is targeted by
// the GUARDIAN USER id (the id devices are registered under), never by the link's
// own id, and a link with no guardian user is dropped rather than pushed to an
// empty audience.
func TestGuardianUserIDsProjectsTheUserIDs(t *testing.T) {
	t.Parallel()

	got := guardianUserIDs([]*studentsv1.GuardianLink{
		{Id: "link-1", GuardianUserId: "user-1", StudentId: "stu-1", Relationship: "mother"},
		{Id: "link-2", GuardianUserId: "user-2", StudentId: "stu-1", Relationship: "father"},
		{Id: "link-3", StudentId: "stu-1"},
		nil,
	})
	if want := []string{"user-1", "user-2"}; !slices.Equal(got, want) {
		t.Errorf("guardianUserIDs = %v, want %v", got, want)
	}

	if empty := guardianUserIDs(nil); len(empty) != 0 {
		t.Errorf("guardianUserIDs(nil) = %v, want no targets", empty)
	}
}
