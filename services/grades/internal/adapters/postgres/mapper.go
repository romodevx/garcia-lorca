package postgres

import (
	"github.com/garcia-lorca/services/grades/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain.

// toDomain rebuilds a domain GradeEntry from its row. The DB-managed updated_at
// is the entry's capture instant (the reserved published_at column is NOT the
// capture time and is left out of the domain — publication is a later phase).
func toDomain(m gradeEntryModel) (domain.GradeEntry, error) {
	return domain.NewGradeEntry(m.ID, m.TenantID, m.EnrollmentID, m.SubjectID, m.TermID, m.TeacherID, m.Value, m.UpdatedAt)
}

// fromDomain projects a domain GradeEntry into its GORM row for insert/upsert.
// PublishedAt is deliberately left nil (the reserved boleta column stays NULL on
// capture) and CreatedAt/UpdatedAt are left zero so the DB assigns them via
// DEFAULT now() (the upsert bumps updated_at explicitly).
func fromDomain(e domain.GradeEntry) gradeEntryModel {
	return gradeEntryModel{
		ID:           e.ID(),
		TenantID:     e.TenantID(),
		EnrollmentID: e.EnrollmentID(),
		SubjectID:    e.SubjectID(),
		TermID:       e.TermID(),
		TeacherID:    e.TeacherID(),
		Value:        e.Value(),
	}
}
