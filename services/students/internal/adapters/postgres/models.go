// Package postgres implements the students repository port with GORM. It is the
// ONLY place GORM appears in students — the domain and use cases never import it.
// A double model (GORM struct here ↔ domain entity via mapper.go) keeps the
// domain framework-free. All access is tenant-scoped for RLS (mirrors iam).
package postgres

import "time"

// studentModel is the GORM row for `students`.
type studentModel struct {
	ID        string  `gorm:"column:id;primaryKey"`
	TenantID  string  `gorm:"column:tenant_id"`
	CURP      string  `gorm:"column:curp"`
	FirstName string  `gorm:"column:first_name"`
	LastName  string  `gorm:"column:last_name"`
	LevelID   *string `gorm:"column:level_id"`
	LevelName string  `gorm:"column:level_name"`
	// LegacyLevel is the pre-0007 SEP key, read (never written) so a row the
	// previous revision wrote mid-deploy still shows a level.
	LegacyLevel     *string `gorm:"column:level;<-:false"`
	GradeGroup      string  `gorm:"column:grade_group"`
	GuardianName    string  `gorm:"column:guardian_name"`
	AdmissionStatus string  `gorm:"column:admission_status"`
	// EnrollmentNumber is the matrícula (task 13, migration 0013).
	EnrollmentNumber string `gorm:"column:enrollment_number"`
	// The editable half of the file (migration 0008).
	BirthDate           *time.Time `gorm:"column:birth_date"`
	Sex                 string     `gorm:"column:sex"`
	Email               string     `gorm:"column:email"`
	Phones              string     `gorm:"column:phones"` // JSON array of {kind, number}
	AddressStreet       string     `gorm:"column:address_street"`
	AddressExtNumber    string     `gorm:"column:address_ext_number"`
	AddressIntNumber    string     `gorm:"column:address_int_number"`
	AddressNeighborhood string     `gorm:"column:address_neighborhood"`
	AddressPostalCode   string     `gorm:"column:address_postal_code"`
	AddressMunicipality string     `gorm:"column:address_municipality"`
	AddressState        string     `gorm:"column:address_state"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
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

// guardianStudentModel is the GORM row for `guardian_students`, the many-to-many
// link between a guardian (an iam user — a plain UUID, RPC-validated, never an FK)
// and a student of this context.
type guardianStudentModel struct {
	ID string `gorm:"column:id;primaryKey"`
	// GuardianUserID is NULL for a contact with no account — a grandmother the
	// school can call. It is *string and not "" because the UNIQUE index depends
	// on it: Postgres does not collide NULLs, which is exactly what lets one
	// pupil have several account-less contacts.
	GuardianUserID *string `gorm:"column:guardian_user_id"`
	TenantID       string  `gorm:"column:tenant_id"`
	StudentID      string  `gorm:"column:student_id"`
	Relationship   string  `gorm:"column:relationship"`
	FullName       string  `gorm:"column:full_name"`
	Phone          string  `gorm:"column:phone"`
	Email          string  `gorm:"column:email"`
	IsEmergency    bool    `gorm:"column:is_emergency"`
	CreatedAt      time.Time
}

func (guardianStudentModel) TableName() string { return "guardian_students" }

// medicalModel is the GORM row for `student_medical`.
type medicalModel struct {
	TenantID   string    `gorm:"column:tenant_id;primaryKey"`
	StudentID  string    `gorm:"column:student_id;primaryKey"`
	BloodType  string    `gorm:"column:blood_type"`
	Allergies  string    `gorm:"column:allergies"`
	Conditions string    `gorm:"column:conditions"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (medicalModel) TableName() string { return "student_medical" }

// photoModel is the GORM row for `student_photos`.
type photoModel struct {
	TenantID    string    `gorm:"column:tenant_id;primaryKey"`
	StudentID   string    `gorm:"column:student_id;primaryKey"`
	ObjectKey   string    `gorm:"column:object_key"`
	ContentType string    `gorm:"column:content_type"`
	SizeBytes   int       `gorm:"column:size_bytes"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (photoModel) TableName() string { return "student_photos" }

// noteModel is the GORM row for `student_notes`. The void columns are NULL
// until the note is voided.
type noteModel struct {
	ID           string     `gorm:"column:id;primaryKey"`
	TenantID     string     `gorm:"column:tenant_id"`
	StudentID    string     `gorm:"column:student_id"`
	Body         string     `gorm:"column:body"`
	AuthorUserID string     `gorm:"column:author_user_id"`
	AuthorName   string     `gorm:"column:author_name"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	VoidedAt     *time.Time `gorm:"column:voided_at"`
	VoidedBy     *string    `gorm:"column:voided_by"`
}

func (noteModel) TableName() string { return "student_notes" }
