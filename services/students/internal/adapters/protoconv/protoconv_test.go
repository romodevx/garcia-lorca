package protoconv_test

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestLevelRoundTrip(t *testing.T) {
	cases := []struct {
		dom  domain.Level
		wire studentsv1.Level
	}{
		{domain.LevelKinder, studentsv1.Level_LEVEL_KINDER},
		{domain.LevelPrimaria, studentsv1.Level_LEVEL_PRIMARIA},
		{domain.LevelSecundaria, studentsv1.Level_LEVEL_SECUNDARIA},
		{domain.LevelPreparatoria, studentsv1.Level_LEVEL_PREPARATORIA},
		{domain.LevelUnspecified, studentsv1.Level_LEVEL_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := protoconv.ToProtoLevel(c.dom); got != c.wire {
			t.Errorf("ToProtoLevel(%v) = %v, want %v", c.dom, got, c.wire)
		}
		if got := protoconv.ToDomainLevel(c.wire); got != c.dom {
			t.Errorf("ToDomainLevel(%v) = %v, want %v", c.wire, got, c.dom)
		}
	}
}

func TestUnknownWireLevelIsUnspecified(t *testing.T) {
	if got := protoconv.ToDomainLevel(studentsv1.Level(99)); got != domain.LevelUnspecified {
		t.Errorf("ToDomainLevel(99) = %v, want Unspecified", got)
	}
}

func TestAdmissionStatusRoundTrip(t *testing.T) {
	cases := []struct {
		dom  domain.AdmissionStatus
		wire studentsv1.AdmissionStatus
	}{
		{domain.AdmissionPending, studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING},
		{domain.AdmissionAccepted, studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED},
		{domain.AdmissionEnrolled, studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED},
		{domain.AdmissionRejected, studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED},
		{domain.AdmissionWithdrawn, studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN},
		{domain.AdmissionUnspecified, studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := protoconv.ToProtoStatus(c.dom); got != c.wire {
			t.Errorf("ToProtoStatus(%v) = %v, want %v", c.dom, got, c.wire)
		}
		if got := protoconv.ToDomainStatus(c.wire); got != c.dom {
			t.Errorf("ToDomainStatus(%v) = %v, want %v", c.wire, got, c.dom)
		}
	}
}

func TestUnknownWireStatusIsUnspecified(t *testing.T) {
	if got := protoconv.ToDomainStatus(studentsv1.AdmissionStatus(99)); got != domain.AdmissionUnspecified {
		t.Errorf("ToDomainStatus(99) = %v, want Unspecified", got)
	}
}
