package repo

import "time"

// attendanceModel is the GORM row for `attendance`: one stored attendance
// exception. EnrollmentID is a cross-service UUID (students.enrollments)
// validated by RPC — a plain column, never a FK. Date maps the `date` column, so
// only its date part is meaningful. SubjectID is NULL for a daily list's mark and
// the subject UUID for a per-class one. Status is constrained by the DB to
// present|absent|late|excused, but the service never stores "present" (a student
// with no row IS present). CreatedAt/UpdatedAt are managed on write.
type attendanceModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	EnrollmentID string    `gorm:"column:enrollment_id"`
	Date         time.Time `gorm:"column:date"`
	SubjectID    *string   `gorm:"column:subject_id"`
	Status       string    `gorm:"column:status"`
	Note         string    `gorm:"column:note"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (attendanceModel) TableName() string { return "attendance" }

// sessionModel is the GORM row for `attendance_sessions`: the record that one
// list (group·date·subject, NULL subject = daily) was taken. TakenBy/TakenByName/
// TakenAt are the first save; UpdatedAt the latest correction.
type sessionModel struct {
	ID          string    `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	TenantID    string    `gorm:"column:tenant_id"`
	GroupID     string    `gorm:"column:group_id"`
	Date        time.Time `gorm:"column:date"`
	SubjectID   *string   `gorm:"column:subject_id"`
	TakenBy     string    `gorm:"column:taken_by"`
	TakenByName string    `gorm:"column:taken_by_name"`
	TakenAt     time.Time `gorm:"column:taken_at;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updated_at;default:now()"`
}

func (sessionModel) TableName() string { return "attendance_sessions" }
