// Package protoconv translates the students domain enums to and from the
// generated garcialorca.students.v1 wire enums. It is the single place that
// mapping lives, shared by the gRPC and Pub/Sub adapters so the two never drift.
// The domain imports none of this — mapping is an adapter concern.
package protoconv

import (
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// ToDomainStatus maps a wire AdmissionStatus to the domain status.
func ToDomainStatus(s studentsv1.AdmissionStatus) domain.AdmissionStatus {
	switch s {
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING:
		return domain.AdmissionPending
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED:
		return domain.AdmissionAccepted
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED:
		return domain.AdmissionEnrolled
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED:
		return domain.AdmissionRejected
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN:
		return domain.AdmissionWithdrawn
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT:
		return domain.AdmissionDraft
	default:
		return domain.AdmissionUnspecified
	}
}

// ToProtoStatus maps a domain AdmissionStatus to the wire status.
func ToProtoStatus(s domain.AdmissionStatus) studentsv1.AdmissionStatus {
	switch s {
	case domain.AdmissionPending:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING
	case domain.AdmissionAccepted:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED
	case domain.AdmissionEnrolled:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED
	case domain.AdmissionRejected:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED
	case domain.AdmissionWithdrawn:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN
	case domain.AdmissionDraft:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT
	default:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED
	}
}

// ToDomainEnrollmentStatus maps a wire EnrollmentStatus to the domain status
// (unknown → Unspecified).
func ToDomainEnrollmentStatus(s studentsv1.EnrollmentStatus) domain.EnrollmentStatus {
	switch s {
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE:
		return domain.EnrollmentStatusActive
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED:
		return domain.EnrollmentStatusTransferred
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN:
		return domain.EnrollmentStatusWithdrawn
	default:
		return domain.EnrollmentStatusUnspecified
	}
}

// ToProtoEnrollmentStatus maps a domain EnrollmentStatus to the wire status
// (unknown → UNSPECIFIED).
func ToProtoEnrollmentStatus(s domain.EnrollmentStatus) studentsv1.EnrollmentStatus {
	switch s {
	case domain.EnrollmentStatusActive:
		return studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE
	case domain.EnrollmentStatusTransferred:
		return studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED
	case domain.EnrollmentStatusWithdrawn:
		return studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN
	default:
		return studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED
	}
}
