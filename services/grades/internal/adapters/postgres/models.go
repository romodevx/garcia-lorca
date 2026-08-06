// Package postgres implements the grades GradeEntryRepository port with GORM. It
// is the ONLY place GORM appears in grades — the domain and use cases never
// import it. A double model (GORM struct here ↔ domain entity via mapper.go)
// keeps the domain framework-free. All access is tenant-scoped for RLS (mirrors
// iam/students).
package postgres

import "time"

// gradeEntryModel is the GORM row for `grade_entries`. All references
// (enrollment/subject/term/teacher) are cross-service UUIDs validated by RPC —
// never FKs. PublishedAt is a nullable column RESERVED for the future
// boleta/publish flow; capture never writes it, so it maps to *time.Time and
// stays NULL. CreatedAt/UpdatedAt are DB-managed (DEFAULT now(); the upsert
// bumps updated_at).
type gradeEntryModel struct {
	ID           string     `gorm:"column:id;primaryKey"`
	TenantID     string     `gorm:"column:tenant_id"`
	EnrollmentID string     `gorm:"column:enrollment_id"`
	SubjectID    string     `gorm:"column:subject_id"`
	TermID       string     `gorm:"column:term_id"`
	TeacherID    string     `gorm:"column:teacher_id"`
	Value        float64    `gorm:"column:value"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (gradeEntryModel) TableName() string { return "grade_entries" }
