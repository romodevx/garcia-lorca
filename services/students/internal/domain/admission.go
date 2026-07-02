package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// AdmissionStatus is the applicant/student lifecycle state. The transitions form
// a state machine:
//
//	pending ──accept──> accepted ──enroll──> enrolled
//	   │                   │                    │
//	 reject             withdraw             withdraw
//	   ▼                   ▼                    ▼
//	rejected           withdrawn            withdrawn   (rejected/withdrawn = terminal)
type AdmissionStatus uint8

const (
	// AdmissionUnspecified is the zero value; not a valid status.
	AdmissionUnspecified AdmissionStatus = iota
	AdmissionPending
	AdmissionAccepted
	AdmissionEnrolled
	AdmissionRejected
	AdmissionWithdrawn
)

var admissionNames = map[AdmissionStatus]string{
	AdmissionPending:   "pending",
	AdmissionAccepted:  "accepted",
	AdmissionEnrolled:  "enrolled",
	AdmissionRejected:  "rejected",
	AdmissionWithdrawn: "withdrawn",
}

// allowedTransitions is the single source of truth for the state machine.
var allowedTransitions = map[AdmissionStatus]map[AdmissionStatus]bool{
	AdmissionPending:  {AdmissionAccepted: true, AdmissionRejected: true},
	AdmissionAccepted: {AdmissionEnrolled: true, AdmissionWithdrawn: true},
	AdmissionEnrolled: {AdmissionWithdrawn: true},
	// rejected and withdrawn are terminal (no outgoing transitions).
}

// CanTransitionTo reports whether moving from s to target is a legal transition.
func (s AdmissionStatus) CanTransitionTo(target AdmissionStatus) bool {
	return allowedTransitions[s][target]
}

// IsTerminal reports whether the status has no outgoing transitions.
func (s AdmissionStatus) IsTerminal() bool {
	return len(allowedTransitions[s]) == 0 && s != AdmissionUnspecified
}

// String returns the canonical status name ("" for unspecified).
func (s AdmissionStatus) String() string { return admissionNames[s] }

// ParseAdmissionStatus resolves a status name to an AdmissionStatus.
func ParseAdmissionStatus(raw string) (AdmissionStatus, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for status, name := range admissionNames {
		if name == key {
			return status, nil
		}
	}
	return AdmissionUnspecified, glerrors.New(glerrors.KindInvalid, "unknown admission status: "+raw)
}
