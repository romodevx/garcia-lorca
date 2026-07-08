// Package protoconv translates the students domain enums to and from the
// generated garcialorca.students.v1 wire enums. It is the single place that
// mapping lives, shared by the gRPC and Pub/Sub adapters so the two never drift.
// The domain imports none of this — mapping is an adapter concern.
package protoconv

import (
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// ToDomainLevel maps a wire Level to the domain Level (unknown → Unspecified).
func ToDomainLevel(l studentsv1.Level) domain.Level {
	switch l {
	case studentsv1.Level_LEVEL_KINDER:
		return domain.LevelKinder
	case studentsv1.Level_LEVEL_PRIMARIA:
		return domain.LevelPrimaria
	case studentsv1.Level_LEVEL_SECUNDARIA:
		return domain.LevelSecundaria
	case studentsv1.Level_LEVEL_PREPARATORIA:
		return domain.LevelPreparatoria
	default:
		return domain.LevelUnspecified
	}
}

// ToProtoLevel maps a domain Level to the wire Level (unknown → UNSPECIFIED).
func ToProtoLevel(l domain.Level) studentsv1.Level {
	switch l {
	case domain.LevelKinder:
		return studentsv1.Level_LEVEL_KINDER
	case domain.LevelPrimaria:
		return studentsv1.Level_LEVEL_PRIMARIA
	case domain.LevelSecundaria:
		return studentsv1.Level_LEVEL_SECUNDARIA
	case domain.LevelPreparatoria:
		return studentsv1.Level_LEVEL_PREPARATORIA
	default:
		return studentsv1.Level_LEVEL_UNSPECIFIED
	}
}

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
	default:
		return studentsv1.AdmissionStatus_ADMISSION_STATUS_UNSPECIFIED
	}
}
