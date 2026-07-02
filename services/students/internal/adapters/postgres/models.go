// Package postgres implements the students repository port with GORM. It is the
// ONLY place GORM appears in students — the domain and use cases never import it.
// A double model (GORM struct here ↔ domain entity via mapper.go) keeps the
// domain framework-free. All access is tenant-scoped for RLS (mirrors iam).
package postgres

import "time"

// studentModel is the GORM row for `students`.
type studentModel struct {
	ID              string `gorm:"column:id;primaryKey"`
	TenantID        string `gorm:"column:tenant_id"`
	CURP            string `gorm:"column:curp"`
	FirstName       string `gorm:"column:first_name"`
	LastName        string `gorm:"column:last_name"`
	Level           string `gorm:"column:level"`
	GradeGroup      string `gorm:"column:grade_group"`
	GuardianName    string `gorm:"column:guardian_name"`
	AdmissionStatus string `gorm:"column:admission_status"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (studentModel) TableName() string { return "students" }
