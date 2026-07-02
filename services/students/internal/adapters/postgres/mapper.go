package postgres

import (
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
	level, err := domain.ParseLevel(m.Level)
	if err != nil {
		return nil, err
	}
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
		Level:           s.Level().String(),
		GradeGroup:      s.GradeGroup(),
		GuardianName:    s.GuardianName(),
		AdmissionStatus: s.Status().String(),
	}
}
