package client

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

func TestLevelNameRoundTrip(t *testing.T) {
	cases := []struct {
		wire studentsv1.Level
		name string
	}{
		{studentsv1.Level_LEVEL_KINDER, "kinder"},
		{studentsv1.Level_LEVEL_PRIMARIA, "primaria"},
		{studentsv1.Level_LEVEL_SECUNDARIA, "secundaria"},
		{studentsv1.Level_LEVEL_PREPARATORIA, "preparatoria"},
	}
	for _, c := range cases {
		if got := levelName(c.wire); got != c.name {
			t.Errorf("levelName(%v) = %q, want %q", c.wire, got, c.name)
		}
		if got := levelFromName(c.name); got != c.wire {
			t.Errorf("levelFromName(%q) = %v, want %v", c.name, got, c.wire)
		}
	}
}

func TestUnknownLevelNameIsUnspecified(t *testing.T) {
	if got := levelFromName("licenciatura"); got != studentsv1.Level_LEVEL_UNSPECIFIED {
		t.Errorf("levelFromName(unknown) = %v, want UNSPECIFIED", got)
	}
	if got := levelName(studentsv1.Level_LEVEL_UNSPECIFIED); got != "" {
		t.Errorf("levelName(UNSPECIFIED) = %q, want empty", got)
	}
}

func TestEnrollmentStatusNames(t *testing.T) {
	cases := []struct {
		wire studentsv1.EnrollmentStatus
		name string
	}{
		{studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE, "active"},
		{studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED, "transferred"},
		{studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN, "withdrawn"},
	}
	for _, c := range cases {
		if got := enrollmentStatusName(c.wire); got != c.name {
			t.Errorf("enrollmentStatusName(%v) = %q, want %q", c.wire, got, c.name)
		}
	}
	if got := enrollmentStatusName(studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED); got != "" {
		t.Errorf("enrollmentStatusName(UNSPECIFIED) = %q, want empty", got)
	}
}

func TestAdmissionNames(t *testing.T) {
	cases := []struct {
		wire studentsv1.AdmissionStatus
		name string
	}{
		{studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING, "pending"},
		{studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED, "accepted"},
		{studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED, "enrolled"},
		{studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED, "rejected"},
		{studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN, "withdrawn"},
	}
	for _, c := range cases {
		if got := admissionName(c.wire); got != c.name {
			t.Errorf("admissionName(%v) = %q, want %q", c.wire, got, c.name)
		}
	}
	if got := admissionName(studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED); got != "" {
		t.Errorf("admissionName(UNSPECIFIED) = %q, want empty", got)
	}
}

func TestAdmissionFromName(t *testing.T) {
	cases := []struct {
		name string
		wire studentsv1.AdmissionStatus
	}{
		{"pending", studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING},
		{"accepted", studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED},
		{"enrolled", studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED},
		{"rejected", studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED},
		{"withdrawn", studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN},
	}
	for _, c := range cases {
		if got := admissionFromName(c.name); got != c.wire {
			t.Errorf("admissionFromName(%q) = %v, want %v", c.name, got, c.wire)
		}
	}
	// "" (no filter) and any unknown name map to UNSPECIFIED (= all).
	if got := admissionFromName(""); got != studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED {
		t.Errorf("admissionFromName(empty) = %v, want UNSPECIFIED", got)
	}
	if got := admissionFromName("graduado"); got != studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED {
		t.Errorf("admissionFromName(unknown) = %v, want UNSPECIFIED", got)
	}
}
