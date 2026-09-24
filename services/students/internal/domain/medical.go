package domain

import (
	"strings"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// BloodType is one of the eight ABO/Rh types, or empty when not captured.
type BloodType string

var bloodTypes = map[BloodType]bool{
	"": true, "A+": true, "A-": true, "B+": true, "B-": true,
	"AB+": true, "AB-": true, "O+": true, "O-": true,
}

const maxMedicalNote = 2000

// MedicalRecord is the part of a pupil's file that is health data: blood type,
// allergies and relevant conditions. It lives apart from the rest of the file
// and is read and written under its own permissions (medical:read/write),
// because who may see it is a narrower question than who may see the pupil.
type MedicalRecord struct {
	bloodType  BloodType
	allergies  string
	conditions string
}

// NewMedicalRecord validates a captured record: a blood type from the closed
// list (or none), and free-text notes of at most 2000 characters each.
func NewMedicalRecord(bloodType, allergies, conditions string) (MedicalRecord, error) {
	bt := BloodType(strings.TrimSpace(bloodType))
	if !bloodTypes[bt] {
		return MedicalRecord{}, glerrors.New(glerrors.KindInvalid, "blood type must be one of A+, A-, B+, B-, AB+, AB-, O+, O-")
	}
	a, c := strings.TrimSpace(allergies), strings.TrimSpace(conditions)
	if utf8.RuneCountInString(a) > maxMedicalNote || utf8.RuneCountInString(c) > maxMedicalNote {
		return MedicalRecord{}, glerrors.New(glerrors.KindInvalid, "a medical note is at most 2000 characters")
	}
	return MedicalRecord{bloodType: bt, allergies: a, conditions: c}, nil
}

// RestoreMedicalRecord rebuilds a stored record without re-judging it.
func RestoreMedicalRecord(bloodType BloodType, allergies, conditions string) MedicalRecord {
	return MedicalRecord{bloodType: bloodType, allergies: allergies, conditions: conditions}
}

// BloodType is the recorded blood type; empty if not captured.
func (m MedicalRecord) BloodType() BloodType { return m.bloodType }

// Allergies are the recorded allergies, free text.
func (m MedicalRecord) Allergies() string { return m.allergies }

// Conditions are the recorded relevant conditions, free text.
func (m MedicalRecord) Conditions() string { return m.conditions }
