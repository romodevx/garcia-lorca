package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestAdmissionValidTransitions(t *testing.T) {
	valid := []struct {
		from, to domain.AdmissionStatus
	}{
		{domain.AdmissionPending, domain.AdmissionAccepted},
		{domain.AdmissionPending, domain.AdmissionRejected},
		{domain.AdmissionPending, domain.AdmissionWithdrawn},
		{domain.AdmissionAccepted, domain.AdmissionEnrolled},
		{domain.AdmissionAccepted, domain.AdmissionWithdrawn},
		{domain.AdmissionEnrolled, domain.AdmissionWithdrawn},
	}
	for _, tc := range valid {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%v → %v should be allowed", tc.from, tc.to)
		}
	}
}

func TestAdmissionInvalidTransitions(t *testing.T) {
	invalid := []struct {
		from, to domain.AdmissionStatus
	}{
		{domain.AdmissionPending, domain.AdmissionEnrolled},   // must go via accepted
		{domain.AdmissionRejected, domain.AdmissionAccepted},  // terminal
		{domain.AdmissionWithdrawn, domain.AdmissionEnrolled}, // terminal
		{domain.AdmissionEnrolled, domain.AdmissionAccepted},  // no going back
		{domain.AdmissionAccepted, domain.AdmissionRejected},  // reject is from pending only
		{domain.AdmissionAccepted, domain.AdmissionAccepted},  // no-op is not a transition
	}
	for _, tc := range invalid {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%v → %v should NOT be allowed", tc.from, tc.to)
		}
	}
}

func TestAdmissionTerminalAndString(t *testing.T) {
	if !domain.AdmissionRejected.IsTerminal() || !domain.AdmissionWithdrawn.IsTerminal() {
		t.Error("rejected and withdrawn are terminal")
	}
	if domain.AdmissionPending.IsTerminal() || domain.AdmissionEnrolled.IsTerminal() {
		t.Error("pending and enrolled are not terminal")
	}
	if domain.AdmissionEnrolled.String() != "enrolled" {
		t.Errorf("String() = %q", domain.AdmissionEnrolled.String())
	}
}

func TestParseAdmissionStatus(t *testing.T) {
	got, err := domain.ParseAdmissionStatus("accepted")
	if err != nil || got != domain.AdmissionAccepted {
		t.Errorf("ParseAdmissionStatus(accepted) = %v, %v", got, err)
	}
	if _, err := domain.ParseAdmissionStatus("bogus"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ParseAdmissionStatus(bogus): want KindInvalid, got %v", err)
	}
}
