// Package postgres implements the config CatalogRepository port with GORM. It is
// the ONLY place GORM appears in config — the domain and use cases never import
// it. A double model (GORM row structs here ↔ domain entities via mapper.go)
// keeps the domain framework-free. All access is tenant-scoped for RLS: the
// adapter binds app.current_tenant per transaction via pkg/authctx (ADR 0001).
package postgres

import "time"

// cycleModel is the GORM row for `cycles`. status is text (lowercase enum:
// draft/active/closed); starts_on/ends_on are SQL `date` carried as time.Time.
type cycleModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	Name      string    `gorm:"column:name"`
	Status    string    `gorm:"column:status"`
	StartsOn  time.Time `gorm:"column:starts_on;type:date"`
	EndsOn    time.Time `gorm:"column:ends_on;type:date"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (cycleModel) TableName() string { return "cycles" }

// levelModel is the GORM row for `levels`. enrollment_granularity is text in the
// DDL's UPPERCASE domain (CYCLE/TERM) — see mapper.go for the casing bridge.
type levelModel struct {
	ID                    string    `gorm:"column:id;primaryKey"`
	TenantID              string    `gorm:"column:tenant_id"`
	Name                  string    `gorm:"column:name"`
	SEPCode               string    `gorm:"column:sep_code"`
	EnrollmentGranularity string    `gorm:"column:enrollment_granularity"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (levelModel) TableName() string { return "levels" }

// gradeLevelModel is the GORM row for `grade_levels` (child of level).
type gradeLevelModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	LevelID   string    `gorm:"column:level_id"`
	Name      string    `gorm:"column:name"`
	Ordinal   int       `gorm:"column:ordinal"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (gradeLevelModel) TableName() string { return "grade_levels" }

// subjectModel is the GORM row for `subjects` (child of grade level).
type subjectModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	GradeLevelID string    `gorm:"column:grade_level_id"`
	Name         string    `gorm:"column:name"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (subjectModel) TableName() string { return "subjects" }

// termModel is the GORM row for `terms` (per cycle+level). starts_on/ends_on are
// SQL `date` carried as time.Time.
type termModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	CycleID   string    `gorm:"column:cycle_id"`
	LevelID   string    `gorm:"column:level_id"`
	Name      string    `gorm:"column:name"`
	Ordinal   int       `gorm:"column:ordinal"`
	StartsOn  time.Time `gorm:"column:starts_on;type:date"`
	EndsOn    time.Time `gorm:"column:ends_on;type:date"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (termModel) TableName() string { return "terms" }

// groupModel is the GORM row for `groups` (per cycle+grade level).
type groupModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	CycleID      string    `gorm:"column:cycle_id"`
	GradeLevelID string    `gorm:"column:grade_level_id"`
	Name         string    `gorm:"column:name"`
	Capacity     int       `gorm:"column:capacity"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (groupModel) TableName() string { return "groups" }

// teacherModel is the GORM row for `teachers` (a catalog person record, NOT a
// login account). user_id is a nullable reserved slot for a future IAM link; it
// is carried as *string so DB NULL ↔ domain "" (see mapper.go).
type teacherModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	FirstName string    `gorm:"column:first_name"`
	LastName  string    `gorm:"column:last_name"`
	Email     string    `gorm:"column:email"`
	UserID    *string   `gorm:"column:user_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (teacherModel) TableName() string { return "teachers" }

// teachingAssignmentModel is the GORM row for `teaching_assignments` (the knot:
// teacher ↔ group ↔ subject ↔ cycle). cycle_id is derived from the group by the
// use case, so every reference here is a real FK to a sibling config table.
type teachingAssignmentModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	CycleID   string    `gorm:"column:cycle_id"`
	GroupID   string    `gorm:"column:group_id"`
	SubjectID string    `gorm:"column:subject_id"`
	TeacherID string    `gorm:"column:teacher_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (teachingAssignmentModel) TableName() string { return "teaching_assignments" }

// nonWorkingPeriodModel is the GORM row for `non_working_periods` (M16): the
// days the school is shut. Stored as a RANGE with both ends inclusive; the
// domain expands it into days for the working-day arithmetic.
type nonWorkingPeriodModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	CycleID   string    `gorm:"column:cycle_id"`
	Label     string    `gorm:"column:label"`
	Kind      string    `gorm:"column:kind"`
	StartsOn  time.Time `gorm:"column:starts_on"`
	EndsOn    time.Time `gorm:"column:ends_on"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (nonWorkingPeriodModel) TableName() string { return "non_working_periods" }

// schoolEventModel is the GORM row for `school_events` (M16): the agenda, as
// opposed to the closed days. Kept in its own table so an activity can never
// move a due date — see the migration for why that matters.
type schoolEventModel struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TenantID    string    `gorm:"column:tenant_id"`
	CycleID     string    `gorm:"column:cycle_id"`
	Title       string    `gorm:"column:title"`
	Description string    `gorm:"column:description"`
	StartsOn    time.Time `gorm:"column:starts_on"`
	EndsOn      time.Time `gorm:"column:ends_on"`
	// NULL in both means the whole school. At most one is ever set.
	LevelID   *string   `gorm:"column:level_id"`
	GroupID   *string   `gorm:"column:group_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (schoolEventModel) TableName() string { return "school_events" }
