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

// enrollmentModel is the GORM row for `enrollments`. term_id is a nullable
// cross-service reference, so it maps to *string ("" domain value ↔ SQL NULL via
// mapper.go). cycle_id/group_id/term_id are plain UUIDs validated by RPC, never
// FKs (config owns those tables).
type enrollmentModel struct {
	ID         string    `gorm:"column:id;primaryKey"`
	TenantID   string    `gorm:"column:tenant_id"`
	StudentID  string    `gorm:"column:student_id"`
	CycleID    string    `gorm:"column:cycle_id"`
	GroupID    string    `gorm:"column:group_id"`
	TermID     *string   `gorm:"column:term_id"`
	Status     string    `gorm:"column:status"`
	EnrolledAt time.Time `gorm:"column:enrolled_at"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (enrollmentModel) TableName() string { return "enrollments" }

// enrollmentTransferModel is the GORM row for `enrollment_transfers`, the dated,
// reasoned history of moving an enrollment between groups.
type enrollmentTransferModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	EnrollmentID string    `gorm:"column:enrollment_id"`
	FromGroupID  string    `gorm:"column:from_group_id"`
	ToGroupID    string    `gorm:"column:to_group_id"`
	Reason       string    `gorm:"column:reason"`
	MovedAt      time.Time `gorm:"column:moved_at"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (enrollmentTransferModel) TableName() string { return "enrollment_transfers" }
