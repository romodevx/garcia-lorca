package client

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// A student's level travels as the school's own catalog entry — the config id
// and the name the school gave it — never folded to a fixed SEP key.
func TestStudentResultCarriesTheCatalogLevel(t *testing.T) {
	got := toStudentResult(&studentsv1.Student{Id: "stu-1", LevelId: "lvl-9", LevelName: "Preescolar"})
	if got.LevelID != "lvl-9" || got.Level != "Preescolar" {
		t.Errorf("level = (%q, %q)", got.LevelID, got.Level)
	}
}

func TestStudentResultCarriesTheProfile(t *testing.T) {
	got := toStudentResult(&studentsv1.Student{Id: "stu-1", Profile: &studentsv1.StudentProfile{
		BirthDate: "2019-03-12", Sex: "F", Email: "ana@correo.mx",
		Phones:  []*studentsv1.StudentPhone{{Kind: "casa", Number: "3312345678"}},
		Address: &studentsv1.StudentAddress{PostalCode: "44100"},
	}})
	p := got.Profile
	if p.BirthDate != "2019-03-12" || p.Sex != "F" || len(p.Phones) != 1 || p.Phones[0].Kind != "casa" || p.Address.PostalCode != "44100" {
		t.Errorf("profile = %+v", p)
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

func TestToGuardianLinkCopiesEveryField(t *testing.T) {
	got := toGuardianLink(&studentsv1.GuardianLink{
		Id:             "gl-1",
		TenantId:       "t1",
		GuardianUserId: "usr-9",
		StudentId:      "stu-1",
		Relationship:   "mother",
	})
	want := GuardianLink{
		ID: "gl-1", TenantID: "t1", GuardianUserID: "usr-9", StudentID: "stu-1", Relationship: "mother",
	}
	if got != want {
		t.Errorf("toGuardianLink() = %+v, want %+v", got, want)
	}
}

// A nil link (an empty response field) must map to the zero value, not panic.
func TestToGuardianLinkNilIsZero(t *testing.T) {
	if got := toGuardianLink(nil); got != (GuardianLink{}) {
		t.Errorf("toGuardianLink(nil) = %+v, want zero", got)
	}
}
