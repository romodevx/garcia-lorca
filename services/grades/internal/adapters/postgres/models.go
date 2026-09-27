// Package postgres implements the grades GradeEntryRepository port with GORM. It
// is the ONLY place GORM appears in grades — the domain and use cases never
// import it. A double model (GORM struct here ↔ domain entity via mapper.go)
// keeps the domain framework-free. All access is tenant-scoped for RLS (mirrors
// iam/students).
package postgres

import (
	"time"

	"github.com/lib/pq"
)

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

// monthlyGradeModel is the GORM row for `monthly_grades` (T6). Month is the
// first day of the month (date column); Value is numeric(5,2), read through a
// float64 and rounded back to exact hundredths by the mapper.
type monthlyGradeModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	EnrollmentID string    `gorm:"column:enrollment_id"`
	SubjectID    string    `gorm:"column:subject_id"`
	TeacherID    string    `gorm:"column:teacher_id"`
	Month        time.Time `gorm:"column:month;type:date"`
	Value        float64   `gorm:"column:value"`
	CapturedBy   string    `gorm:"column:captured_by"`
	CreatedAt    time.Time `gorm:"column:created_at;->"`
	UpdatedAt    time.Time `gorm:"column:updated_at;->"`
}

func (monthlyGradeModel) TableName() string { return "monthly_grades" }

// periodClosureModel is the GORM row for `period_closures` (T7). Nullable
// columns map to pointers so a never-set instant stays NULL, never 0001-01-01.
type periodClosureModel struct {
	TenantID     string     `gorm:"column:tenant_id;primaryKey"`
	TermID       string     `gorm:"column:term_id;primaryKey"`
	CycleID      string     `gorm:"column:cycle_id"`
	LevelID      string     `gorm:"column:level_id"`
	Status       string     `gorm:"column:status"`
	Version      int        `gorm:"column:version"`
	ClosedAt     *time.Time `gorm:"column:closed_at"`
	ClosedBy     *string    `gorm:"column:closed_by"`
	ClosedByName *string    `gorm:"column:closed_by_name"`
	ReopenedAt   *time.Time `gorm:"column:reopened_at"`
	// Months is the text[] snapshot of "YYYY-MM" the last close covered.
	Months pq.StringArray `gorm:"column:months;type:text[]"`
}

func (periodClosureModel) TableName() string { return "period_closures" }

// reportCardModel is the GORM row for `report_cards` (T7). Averages are
// numeric(5,2) read through *float64 (NULL = no average) and rounded back to
// exact tenths by the mapper; months is a text[] of "YYYY-MM".
type reportCardModel struct {
	ID                     string         `gorm:"column:id;primaryKey"`
	TenantID               string         `gorm:"column:tenant_id"`
	TermID                 string         `gorm:"column:term_id"`
	CycleID                string         `gorm:"column:cycle_id"`
	LevelID                string         `gorm:"column:level_id"`
	GroupID                string         `gorm:"column:group_id"`
	EnrollmentID           string         `gorm:"column:enrollment_id"`
	StudentID              string         `gorm:"column:student_id"`
	Version                int            `gorm:"column:version"`
	Status                 string         `gorm:"column:status"`
	IssuedAt               time.Time      `gorm:"column:issued_at"`
	IssuedBy               string         `gorm:"column:issued_by"`
	StudentName            string         `gorm:"column:student_name"`
	EnrollmentNumber       string         `gorm:"column:enrollment_number"`
	GroupLabel             string         `gorm:"column:group_label"`
	LevelName              string         `gorm:"column:level_name"`
	CycleName              string         `gorm:"column:cycle_name"`
	TermName               string         `gorm:"column:term_name"`
	Months                 pq.StringArray `gorm:"column:months;type:text[]"`
	GeneralAverage         *float64       `gorm:"column:general_average"`
	ScaleMinHundredths     int            `gorm:"column:scale_min_hundredths"`
	ScaleMaxHundredths     int            `gorm:"column:scale_max_hundredths"`
	ScaleDecimals          int            `gorm:"column:scale_decimals"`
	ScalePassingHundredths int            `gorm:"column:scale_passing_hundredths"`
}

func (reportCardModel) TableName() string { return "report_cards" }

// reportCardLineModel is the GORM row for `report_card_lines` (T7).
// MonthValues is the jsonb object of exact decimal strings, kept as raw JSON.
type reportCardLineModel struct {
	ReportCardID string   `gorm:"column:report_card_id;primaryKey"`
	TenantID     string   `gorm:"column:tenant_id"`
	SubjectID    string   `gorm:"column:subject_id;primaryKey"`
	SubjectName  string   `gorm:"column:subject_name"`
	Position     int      `gorm:"column:position"`
	MonthValues  string   `gorm:"column:month_values;type:jsonb"`
	TermAverage  *float64 `gorm:"column:term_average"`
}

func (reportCardLineModel) TableName() string { return "report_card_lines" }
