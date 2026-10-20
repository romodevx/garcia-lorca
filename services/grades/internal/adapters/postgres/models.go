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

// periodGradeModel is the GORM row for `period_grades` (T15). Value is the
// numeric(5,2) column read and written as exact decimal TEXT ("9", "9.00"),
// never through a float.
type periodGradeModel struct {
	ID           string `gorm:"column:id;primaryKey"`
	TenantID     string `gorm:"column:tenant_id"`
	EnrollmentID string `gorm:"column:enrollment_id"`
	SubjectID    string `gorm:"column:subject_id"`
	PeriodID     string `gorm:"column:period_id"`
	// Value or Letter: a cell holds exactly one (migration 0011).
	Value          *string   `gorm:"column:value;type:numeric"`
	Letter         *string   `gorm:"column:letter"`
	CapturedBy     string    `gorm:"column:captured_by"`
	CapturedByName string    `gorm:"column:captured_by_name"`
	CreatedAt      time.Time `gorm:"column:created_at;->"`
	UpdatedAt      time.Time `gorm:"column:updated_at;->"`
}

func (periodGradeModel) TableName() string { return "period_grades" }

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
	// ReopenedBy / ReopenedByName / ReopenReason audit the latest reopen
	// (CC-308, grades/0010).
	ReopenedBy     *string `gorm:"column:reopened_by"`
	ReopenedByName *string `gorm:"column:reopened_by_name"`
	ReopenReason   *string `gorm:"column:reopen_reason"`
	// Months is the text[] snapshot of "YYYY-MM" the last close covered.
	Months pq.StringArray `gorm:"column:months;type:text[]"`
}

func (periodClosureModel) TableName() string { return "period_closures" }

// reportCardModel is the GORM row for `report_cards` (T7). Averages are
// numeric(5,2) read and written as exact decimal text (NULL = no average);
// months is a text[] of "YYYY-MM" (cards issued before T15) and periods the
// jsonb array of the card's period columns (T15).
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
	TermOrdinal            int            `gorm:"column:term_ordinal"`
	Months                 pq.StringArray `gorm:"column:months;type:text[]"`
	Periods                string         `gorm:"column:periods;type:jsonb"`
	GeneralAverage         *string        `gorm:"column:general_average;type:numeric"`
	ScaleMinHundredths     int            `gorm:"column:scale_min_hundredths"`
	ScaleMaxHundredths     int            `gorm:"column:scale_max_hundredths"`
	ScaleDecimals          int            `gorm:"column:scale_decimals"`
	ScalePassingHundredths int            `gorm:"column:scale_passing_hundredths"`
	// GradingLetters is NULL on cards issued (or archived) before 0011: numbers.
	GradingLetters *bool `gorm:"column:grading_letters"`
}

func (reportCardModel) TableName() string { return "report_cards" }

// reportCardLineModel is the GORM row for `report_card_lines` (T7).
// MonthValues (before T15) and PeriodValues (T15) are jsonb objects of exact
// decimal strings, kept as raw JSON; TermAverage is exact decimal text.
type reportCardLineModel struct {
	ReportCardID string  `gorm:"column:report_card_id;primaryKey"`
	TenantID     string  `gorm:"column:tenant_id"`
	SubjectID    string  `gorm:"column:subject_id;primaryKey"`
	SubjectName  string  `gorm:"column:subject_name"`
	Position     int     `gorm:"column:position"`
	MonthValues  string  `gorm:"column:month_values;type:jsonb"`
	PeriodValues string  `gorm:"column:period_values;type:jsonb"`
	TermAverage  *string `gorm:"column:term_average;type:numeric"`
}

func (reportCardLineModel) TableName() string { return "report_card_lines" }

// outboxModel is the GORM row for `grades_outbox`: an event queued in the
// transaction that produced it. The database mints the id (a queue entry has
// no domain identity), so the default tag keeps GORM from sending "".
type outboxModel struct {
	ID            string     `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	TenantID      string     `gorm:"column:tenant_id"`
	EventType     string     `gorm:"column:event_type"`
	Payload       []byte     `gorm:"column:payload;type:jsonb"`
	CreatedAt     time.Time  `gorm:"column:created_at;default:now()"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	Attempts      int        `gorm:"column:attempts"`
	NextAttemptAt time.Time  `gorm:"column:next_attempt_at;default:now()"`
}

func (outboxModel) TableName() string { return "grades_outbox" }
