package repo

import "time"

// attendanceModel is the GORM row for `attendance`: one stored attendance
// exception. EnrollmentID is a cross-service UUID (students.enrollments)
// validated by RPC — a plain column, never a FK. Date maps the `date` column, so
// only its date part is meaningful. Status is constrained by the DB to
// present|absent|late|excused, but the service never stores "present" (a student
// with no row IS present). CreatedAt/UpdatedAt are managed on write.
type attendanceModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	EnrollmentID string    `gorm:"column:enrollment_id"`
	Date         time.Time `gorm:"column:date"`
	Status       string    `gorm:"column:status"`
	Note         string    `gorm:"column:note"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (attendanceModel) TableName() string { return "attendance" }
