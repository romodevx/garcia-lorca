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
