package postgres

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain.

// toDomain rebuilds a domain Student from its row.
func toDomain(m studentModel) (*domain.Student, error) {
	curp, err := domain.NewCURP(m.CURP)
	if err != nil {
		return nil, err
	}
	// Restored, not re-judged: a legacy pupil whose old level matched no config
	// level has a name and no id, and must stay readable (migration 0007).
	levelID := ""
	if m.LevelID != nil {
		levelID = *m.LevelID
	}
	level := domain.RestoreLevel(levelID, levelName(m))
	status, err := domain.ParseAdmissionStatus(m.AdmissionStatus)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "unknown admission status in db", err)
	}
	return domain.Rehydrate(domain.NewApplicantParams{
		ID:           m.ID,
		TenantID:     m.TenantID,
		CURP:         curp,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		Level:        level,
		GradeGroup:   m.GradeGroup,
		GuardianName: m.GuardianName,
	}, status), nil
}

// fromDomain projects a domain Student into its GORM row (for insert/update).
func fromDomain(s *domain.Student) studentModel {
	return studentModel{
		ID:              s.ID(),
		TenantID:        s.TenantID(),
		CURP:            s.CURP().String(),
		FirstName:       s.FirstName(),
		LastName:        s.LastName(),
		LevelID:         nullableID(s.Level().ID()),
		LevelName:       s.Level().Name(),
		GradeGroup:      s.GradeGroup(),
		GuardianName:    s.GuardianName(),
		AdmissionStatus: s.Status().String(),
	}
}

// enrollmentToDomain rebuilds a domain Enrollment from its row. A NULL term_id
// maps to the empty string (the domain's "no term" value); an unrecognized
// status is an internal error (the DB CHECK should make this impossible).
func enrollmentToDomain(m enrollmentModel) (domain.Enrollment, error) {
	status, err := domain.ParseEnrollmentStatus(m.Status)
	if err != nil {
		return domain.Enrollment{}, glerrors.Wrap(glerrors.KindInternal, "unknown enrollment status in db", err)
	}
	term := ""
	if m.TermID != nil {
		term = *m.TermID
	}
	return domain.NewEnrollment(m.ID, m.TenantID, m.StudentID, m.CycleID, m.GroupID, term, status, m.EnrolledAt)
}

// enrollmentFromDomain projects a domain Enrollment into its GORM row. An empty
// term maps to SQL NULL so an absent term is not stored as the empty string.
func enrollmentFromDomain(e domain.Enrollment) enrollmentModel {
	var term *string
	if e.TermID() != "" {
		t := e.TermID()
		term = &t
	}
	return enrollmentModel{
		ID:         e.ID(),
		TenantID:   e.TenantID(),
		StudentID:  e.StudentID(),
		CycleID:    e.CycleID(),
		GroupID:    e.GroupID(),
		TermID:     term,
		Status:     e.Status().String(),
		EnrolledAt: e.EnrolledAt(),
	}
}

// guardianLinkToDomain rebuilds a domain GuardianLink from its row. An
// unrecognized relationship is an internal error (the DB CHECK should make this
// impossible).
func guardianLinkToDomain(m guardianStudentModel) (domain.GuardianLink, error) {
	rel, err := domain.ParseRelationship(m.Relationship)
	if err != nil {
		return domain.GuardianLink{}, glerrors.Wrap(glerrors.KindInternal, "unknown relationship in db", err)
	}
	person := domain.ContactPerson{FullName: m.FullName, Phone: m.Phone, Email: m.Email}
	if m.GuardianUserID == nil || *m.GuardianUserID == "" {
		return domain.RestoreContact(m.ID, m.TenantID, m.StudentID, rel, person)
	}
	link, err := domain.NewGuardianLink(m.ID, m.TenantID, *m.GuardianUserID, m.StudentID, rel)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	return link.WithPerson(person), nil
}

// guardianLinkFromDomain projects a domain GuardianLink into its GORM row.
func guardianLinkFromDomain(l domain.GuardianLink) guardianStudentModel {
	m := guardianStudentModel{
		ID:           l.ID(),
		TenantID:     l.TenantID(),
		StudentID:    l.StudentID(),
		Relationship: l.Relationship().String(),
		FullName:     l.FullName(),
		Phone:        l.Phone(),
		Email:        l.Email(),
	}
	if l.HasAccount() {
		userID := l.GuardianUserID()
		m.GuardianUserID = &userID
	}
	return m
}

// transferFromDomain projects a domain EnrollmentTransfer into its GORM row.
func transferFromDomain(t domain.EnrollmentTransfer) enrollmentTransferModel {
	return enrollmentTransferModel{
		ID:           t.ID(),
		TenantID:     t.TenantID(),
		EnrollmentID: t.EnrollmentID(),
		FromGroupID:  t.FromGroupID(),
		ToGroupID:    t.ToGroupID(),
		Reason:       t.Reason(),
		MovedAt:      t.MovedAt(),
	}
}

// nullableID maps the domain's "" (no config level) to SQL NULL.
func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// levelName is the stored name, or — for a row the previous revision wrote
// after migration 0007 ran, which carries only the old SEP key — that key,
// capitalized ("primaria" → "Primaria"), so the pupil never reads as levelless.
func levelName(m studentModel) string {
	if m.LevelName != "" || m.LegacyLevel == nil || *m.LegacyLevel == "" {
		return m.LevelName
	}
	key := *m.LegacyLevel
	return strings.ToUpper(key[:1]) + key[1:]
}
