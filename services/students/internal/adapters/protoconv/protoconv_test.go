package protoconv_test

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
)

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
		{domain.AdmissionDraft, studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT},
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

func TestSuggestionEnumsToProto(t *testing.T) {
	reasons := map[domain.SuggestionReason]studentsv1.FamilySuggestionReason{
		domain.ReasonUnspecified:        studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_UNSPECIFIED,
		domain.ReasonSameSurnames:       studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SAME_SURNAMES,
		domain.ReasonSharedContactEmail: studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_EMAIL,
		domain.ReasonSharedContactPhone: studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_CONTACT_PHONE,
		domain.ReasonSharedAccount:      studentsv1.FamilySuggestionReason_FAMILY_SUGGESTION_REASON_SHARED_ACCOUNT,
	}
	for dom, wire := range reasons {
		if got := protoconv.ToProtoSuggestionReason(dom); got != wire {
			t.Errorf("reason %d → %v, want %v", dom, got, wire)
		}
	}
	confidences := map[domain.SuggestionConfidence]studentsv1.FamilySuggestionConfidence{
		domain.ConfidenceUnspecified: studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_UNSPECIFIED,
		domain.ConfidenceMedium:      studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_MEDIUM,
		domain.ConfidenceHigh:        studentsv1.FamilySuggestionConfidence_FAMILY_SUGGESTION_CONFIDENCE_HIGH,
	}
	for dom, wire := range confidences {
		if got := protoconv.ToProtoSuggestionConfidence(dom); got != wire {
			t.Errorf("confidence %d → %v, want %v", dom, got, wire)
		}
	}
}
