package studentsclient

import (
	"errors"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The adapter makes exactly two decisions — how a transport failure becomes the
// seam's error vocabulary, and how an enrollment projects onto the attendance
// view — so those are what is tested here, directly on the helpers, no server
// involved.

func TestMapStudentsErr(t *testing.T) {
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

			got := mapStudentsErr(tt.err, "enrollments")
			if got == nil {
				t.Fatalf("mapStudentsErr returned nil")
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

// TestStatusString pins the enum → lowercase string mapping the service compares
// against: only "active" puts a student on a roster, and an unknown enum must
// NEVER read as active.
func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   studentsv1.EnrollmentStatus
		want string
	}{
		{name: "active", in: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE, want: "active"},
		{name: "transferred", in: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED, want: "transferred"},
		{name: "withdrawn", in: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN, want: "withdrawn"},
		{name: "unspecified is not active", in: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED, want: ""},
		{name: "unknown enum is not active", in: studentsv1.EnrollmentStatus(99), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := statusString(tt.in); got != tt.want {
				t.Errorf("statusString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestToRef proves the projection carries exactly what a roster needs — and that
// a nil enrollment (a malformed response) degrades to a zero ref instead of
// panicking.
func TestToRef(t *testing.T) {
	t.Parallel()

	got := toRef(&studentsv1.Enrollment{
		Id:        "enr-1",
		StudentId: "stu-1",
		GroupId:   "grp-1",
		CycleId:   "cyc-1",
		Status:    studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE,
	})
	if got.ID != "enr-1" || got.StudentID != "stu-1" || got.GroupID != "grp-1" || got.Status != "active" {
		t.Errorf("toRef = %+v, want the enrollment projected with a lowercase status", got)
	}

	if zero := toRef(nil); zero.ID != "" || zero.Status != "" {
		t.Errorf("toRef(nil) = %+v, want the zero ref", zero)
	}
}
