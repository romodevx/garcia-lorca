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
	ID                    string  `gorm:"column:id;primaryKey"`
	TenantID              string  `gorm:"column:tenant_id"`
	Name                  string  `gorm:"column:name"`
	SEPCode               string  `gorm:"column:sep_code"`
	EnrollmentGranularity string  `gorm:"column:enrollment_granularity"`
	AttendanceMode        string  `gorm:"column:attendance_mode"`
	GradingEnabled        bool    `gorm:"column:grading_enabled"`
	GradingLetters        bool    `gorm:"column:grading_letters"`
	GradeMin              float64 `gorm:"column:grade_min"`
	GradeMax              float64 `gorm:"column:grade_max"`
	GradeDecimals         int     `gorm:"column:grade_decimals"`
	GradePassing          float64 `gorm:"column:grade_passing"`
	// Status is read-only here: only the level deletion's own statements
	// (BeginLevelDeletion, its compensation, a restore) change it.
	Status string `gorm:"column:status;->"`
	// SortOrder is read-only here too: an insert leaves it to the column
	// default and the insert trigger (0020: after the last level), and only
	// ReorderLevels rewrites it.
	SortOrder int       `gorm:"column:sort_order;->"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
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

// termMonthModel is the GORM row for `term_months`: one calendar month (its
// first day) a term covers. cycle_id/level_id are the term's, denormalised so
// the DB can hold "a month belongs to one term per cycle+level".
type termMonthModel struct {
	TenantID string    `gorm:"column:tenant_id;primaryKey"`
	TermID   string    `gorm:"column:term_id;primaryKey"`
	CycleID  string    `gorm:"column:cycle_id"`
	LevelID  string    `gorm:"column:level_id"`
	Month    time.Time `gorm:"column:month;primaryKey;type:date"`
}

func (termMonthModel) TableName() string { return "term_months" }

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

// schoolProfileModel is the `school_profile` row: the school's one set of
// details (task 12).
type schoolProfileModel struct {
	TenantID  string    `gorm:"column:tenant_id;primaryKey"`
	Name      string    `gorm:"column:name"`
	LegalName string    `gorm:"column:legal_name"`
	RFC       string    `gorm:"column:rfc"`
	CCT       string    `gorm:"column:cct"`
	Address   string    `gorm:"column:address"`
	Phone     string    `gorm:"column:phone"`
	Legend    string    `gorm:"column:legend"`
	CLABE     string    `gorm:"column:clabe"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName pins the row to its table.
func (schoolProfileModel) TableName() string { return "school_profile" }
