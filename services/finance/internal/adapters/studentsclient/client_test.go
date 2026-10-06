package studentsclient

import (
	"errors"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The adapter's only decision is how a transport failure becomes the port's
// error vocabulary, so that is what is tested here — directly on the helper, no
// server involved. Everything else in the client is projection.
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
			err:      connect.NewError(connect.CodeNotFound, errors.New("student not found")),
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

			got := mapStudentsErr(tt.err, "student")
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

// statusString is what the bulk generation filters on, so an unknown enum must
// degrade to the empty string (non-active) rather than to something truthy.
func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   studentsv1.EnrollmentStatus
		want string
	}{
		{"active", studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE, "active"},
		{"transferred", studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED, "transferred"},
		{"withdrawn", studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN, "withdrawn"},
		{"unspecified", studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED, ""},
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

// The admission status rides along best-effort on StudentRef; an unmapped enum
// must degrade to the empty string rather than to something that reads live.
func TestAdmissionStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   studentsv1.AdmissionStatus
		want string
	}{
		{"pending", studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING, "pending"},
		{"accepted", studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED, "accepted"},
		{"enrolled", studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED, "enrolled"},
		{"rejected", studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED, "rejected"},
		{"withdrawn", studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN, "withdrawn"},
		{"unspecified", studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := admissionStatusString(tt.in); got != tt.want {
				t.Errorf("admissionStatusString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The import's mark must survive the trip: losing it bills every imported
// pupil an inscription the previous system already collected (ADR 0033).
func TestToRefCarriesTheEnrollmentOrigin(t *testing.T) {
	for _, origin := range []string{"", "manual", "import"} {
		got := toRef(&studentsv1.Enrollment{Id: "e1", Origin: origin})
		if got.Origin != origin {
			t.Errorf("origin %q came back as %q", origin, got.Origin)
		}
	}
	if !toRef(&studentsv1.Enrollment{Origin: "import"}).Imported() {
		t.Error("an import enrolment must read as imported")
	}
}
