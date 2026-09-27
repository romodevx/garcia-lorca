package postgres

import (
	"encoding/json"
	"strings"
	"time"

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
	profile, err := profileFromRow(m)
	if err != nil {
		return nil, err
	}
	s := domain.Rehydrate(domain.NewApplicantParams{
		ID:           m.ID,
		TenantID:     m.TenantID,
		CURP:         curp,
		FirstName:    m.FirstName,
		LastName:     m.LastName,
		Level:        level,
		GradeGroup:   m.GradeGroup,
		GuardianName: m.GuardianName,
		Profile:      profile,
	}, status)
	s.AssignEnrollmentNumber(m.EnrollmentNumber)
	return s, nil
}

// phoneRow is one element of the `phones` JSON array.
type phoneRow struct {
	Kind   string `json:"kind"`
	Number string `json:"number"`
}

// profileFromRow restores the file's editable half without re-judging it.
func profileFromRow(m studentModel) (domain.Profile, error) {
	var rows []phoneRow
	if m.Phones != "" {
		if err := json.Unmarshal([]byte(m.Phones), &rows); err != nil {
			return domain.Profile{}, glerrors.Wrap(glerrors.KindInternal, "unreadable phones in db", err)
		}
	}
	phones := make([]domain.Phone, 0, len(rows))
	for _, r := range rows {
		phones = append(phones, domain.Phone{Kind: domain.PhoneKind(r.Kind), Number: r.Number})
	}
	var born time.Time
	if m.BirthDate != nil {
		born = *m.BirthDate
	}
	return domain.RestoreProfile(born, domain.Sex(m.Sex), m.Email, phones, domain.Address{
		Street: m.AddressStreet, ExtNumber: m.AddressExtNumber, IntNumber: m.AddressIntNumber,
		Neighborhood: m.AddressNeighborhood, PostalCode: m.AddressPostalCode,
		Municipality: m.AddressMunicipality, State: m.AddressState,
	}), nil
}

// profileColumns projects a profile onto its columns, for insert and update.
func profileColumns(p domain.Profile) (map[string]any, error) {
	rows := make([]phoneRow, 0, len(p.Phones()))
	for _, ph := range p.Phones() {
		rows = append(rows, phoneRow{Kind: string(ph.Kind), Number: ph.Number})
	}
	phones, err := json.Marshal(rows)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "encoding phones", err)
	}
	var born *time.Time
	if !p.BirthDate().IsZero() {
		b := p.BirthDate()
		born = &b
	}
	a := p.Address()
	return map[string]any{
		"birth_date": born, "sex": string(p.Sex()), "email": p.Email(), "phones": string(phones),
		"address_street": a.Street, "address_ext_number": a.ExtNumber, "address_int_number": a.IntNumber,
		"address_neighborhood": a.Neighborhood, "address_postal_code": a.PostalCode,
		"address_municipality": a.Municipality, "address_state": a.State,
	}, nil
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
		Phones:          "[]",
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
	e, err := domain.NewEnrollment(m.ID, m.TenantID, m.StudentID, m.CycleID, m.GroupID, term, status, m.EnrolledAt)
	if err != nil {
		return domain.Enrollment{}, err
	}
	// A non-active enrollment is never written again after its status change
	// (only an ACTIVE one can be transferred or withdrawn), so updated_at is the
	// instant it ended. WithEndedAt ignores it for an active one.
	return e.WithEndedAt(m.UpdatedAt), nil
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
	person := domain.ContactPerson{FullName: m.FullName, Phone: m.Phone, Email: m.Email, Emergency: m.IsEmergency}
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
		IsEmergency:  l.IsEmergency(),
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

// transferToDomain rebuilds a domain EnrollmentTransfer from its row.
func transferToDomain(m enrollmentTransferModel) (domain.EnrollmentTransfer, error) {
	return domain.NewEnrollmentTransfer(m.ID, m.TenantID, m.EnrollmentID, m.FromGroupID, m.ToGroupID, m.Reason, m.MovedAt)
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
