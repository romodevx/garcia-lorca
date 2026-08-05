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
