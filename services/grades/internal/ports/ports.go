package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

// This file declares the seams the grades use cases depend on. Interfaces live
// here (consumer side); adapters (config/students Connect clients, postgres)
// implement them. The domain imports none of this — dependencies point inward.
//
// The grades context has no local ownership of assignments or enrollments: it
// combines config (which teaching assignment = group+subject+teacher+cycle) with
// students (who is enrolled in the group) and stores grade_entries. Both
// cross-service views are minimal, read-only projections carrying only what the
// capture knot and roster need — never PII.

// AssignmentRef is the grades-owned view of a config teaching assignment: the
// group it teaches, and the subject/teacher/cycle it binds. subjectID and
// teacherID are DERIVED from here by the use case, never client-supplied.
type AssignmentRef struct {
	ID        string
	GroupID   string
	SubjectID string
	TeacherID string
	CycleID   string
}

// TermRef is the grades-owned view of a config term: its identity, cycle and
// the level it belongs to (a term is per cycle AND per level), plus its name,
// its order and the calendar months it covers (T7; ascending, may be empty).
type TermRef struct {
	ID      string
	CycleID string
	LevelID string
	Name    string
	Ordinal int
	Months  []domain.Month
}

// AssignmentReader validates teaching-assignment and term references against the
// config bounded context (by RPC). It is a consumer-defined seam: the use case
// depends on this interface and stays testable with a fake, while the real
// adapter is a Connect client to config. A missing assignment/term is reported
// as glerrors.ErrNotFound (KindNotFound); any other error propagates unchanged.
type AssignmentReader interface {
	ByID(ctx context.Context, id string) (AssignmentRef, error)
	TermByID(ctx context.Context, id string) (TermRef, error)
	// ListTerms lists a cycle's terms, of one level when levelID is set (one
	// read), in ordinal order.
	ListTerms(ctx context.Context, cycleID, levelID string) ([]TermRef, error)
}

// EnrollmentRef is the grades-owned view of a students enrollment: enough to
// place the student in a group of a cycle and know whether the enrollment is
// live. StudentID is carried through to the roster (names are resolved in the
// web from the students list — grades stays free of student PII).
type EnrollmentRef struct {
	ID        string
	StudentID string
	GroupID   string
	CycleID   string
	Status    string
	// EnrolledAt is when the pupil enrolled (zero when unknown): a period
	// expects no grade of theirs before that month.
	EnrolledAt time.Time
}

// EnrollmentReader reads enrollments from the students bounded context (by RPC).
// A missing enrollment is reported as glerrors.ErrNotFound (KindNotFound); any
// other error propagates unchanged.
type EnrollmentReader interface {
	ByID(ctx context.Context, id string) (EnrollmentRef, error)
	ByGroup(ctx context.Context, groupID string) ([]EnrollmentRef, error)
	// ByCycle lists every enrollment of a cycle, whatever its status.
	ByCycle(ctx context.Context, cycleID string) ([]EnrollmentRef, error)
	// TransfersIntoGroup lists every move INTO a group (one read per sheet).
	TransfersIntoGroup(ctx context.Context, groupID string) ([]TransferRef, error)
}

// TransferRef is the grades-owned view of a students group move: which
// enrollment moved into which group, and when. A transfer rewrites the
// enrollment's group in place, so this is the only record of when a pupil
// joined the group they sit in now.
type TransferRef struct {
	EnrollmentID string
	ToGroupID    string
	MovedAt      time.Time
}

// GradeEntryRepository loads and persists grade entries. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction,
// as in students/iam).
type GradeEntryRepository interface {
	// UpsertGradeEntry inserts a grade entry, or updates the value of the
	// existing row on the (tenant, enrollment, subject, term) key — one grade per
	// student·subject·term ("capturar/editar"). Returns the stored entry.
	UpsertGradeEntry(ctx context.Context, e domain.GradeEntry) (domain.GradeEntry, error)
	// GetForKeys batch-loads the grade entries for many enrollments under the
	// same (subject, term), in ONE tenant-bound query, so building a roster never
	// fans out into a grade lookup per student (N+1). The result is keyed by
	// enrollment_id and holds an entry ONLY for enrollments that have a grade;
	// absent enrollments are simply missing from the map. An empty enrollmentIDs
	// slice yields an empty map without touching the database.
	GetForKeys(ctx context.Context, enrollmentIDs []string, subjectID, termID string) (map[string]domain.GradeEntry, error)
	// ListGradeEntries returns entries for an enrollment in a term; an empty
	// string skips that filter.
	ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]domain.GradeEntry, error)
}

// LevelRef is the grades-owned view of the level a group belongs to: whether it
// captures numeric grades at all and on which scale (config, T6).
type LevelRef struct {
	ID             string
	GradingEnabled bool
	Scale          domain.GradingScale
}

// CycleRef is the grades-owned view of a config cycle: its name and the dates
// whose months bound monthly capture. Either date may be zero (not set).
type CycleRef struct {
	ID       string
	Name     string
	StartsOn time.Time
	EndsOn   time.Time
}

// CatalogReader reads the rest of the academic catalog the monthly knot needs
// from config (by RPC). A missing group/cycle is glerrors.ErrNotFound.
type CatalogReader interface {
	// LevelOfGroup resolves group → grade level → level, with its scale. A
	// grading level whose scale cannot be read is a KindInvalid refusal.
	LevelOfGroup(ctx context.Context, groupID string) (LevelRef, error)
	// LevelIDOfGroup resolves group → grade level → level id only, for checks
	// that must not depend on the scale (the legacy term check).
	LevelIDOfGroup(ctx context.Context, groupID string) (string, error)
	// ActiveCycle is the school's active cycle; false when none is active.
	ActiveCycle(ctx context.Context) (CycleRef, bool, error)
	// GroupsOfLevel lists the ids of the groups of a level in a cycle.
	GroupsOfLevel(ctx context.Context, cycleID, levelID string) ([]string, error)
	CycleByID(ctx context.Context, id string) (CycleRef, error)
	// AssignmentsByGroup lists the teaching assignments of a group in a cycle.
	AssignmentsByGroup(ctx context.Context, cycleID, groupID string) ([]AssignmentRef, error)
	// LevelCatalog reads, in a fixed number of calls whatever the school's
	// size, everything a period of the level needs: its name and scale, its
	// groups in the cycle (labelled "1° A"), their teaching assignments and the
	// names of those subjects and teachers (T7).
	LevelCatalog(ctx context.Context, cycleID, levelID string) (LevelCatalog, error)
}

// GroupRef is a group of a level with the label a report card prints ("1° A").
type GroupRef struct {
	ID    string
	Label string
}

// LevelCatalog is one level's academic catalog in one cycle, as a period's
// progress and report cards read it.
type LevelCatalog struct {
	LevelName      string
	GradingEnabled bool
	Scale          domain.GradingScale
	Groups         []GroupRef
	Assignments    []AssignmentRef
	SubjectNames   map[string]string
	TeacherNames   map[string]string
}

// StudentRef is the grades-owned view of a pupil for a report card: the full
// name and the matricula, copied into the snapshot at issue time.
type StudentRef struct {
	ID               string
	Name             string
	EnrollmentNumber string
}

// StudentDirectory names pupils by id in as few reads as the students service
// allows (it pages ids by 200). A pupil it does not know is simply absent.
type StudentDirectory interface {
	StudentsByIDs(ctx context.Context, ids []string) (map[string]StudentRef, error)
}

// MonthlyGradeRepository loads and persists monthly grades. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction).
type MonthlyGradeRepository interface {
	// UpsertMonthlyGrade inserts the grade or updates value/teacher/captured_by
	// of the existing row on the (tenant, enrollment, subject, month) key, and
	// returns the stored row (with its id and updated_at). When lock names a
	// term, the write re-checks that term's closure INSIDE its transaction,
	// serialised against a close, and refuses a closed one with MONTH_LOCKED.
	UpsertMonthlyGrade(ctx context.Context, g domain.MonthlyGrade, lock PeriodLock) (domain.MonthlyGrade, error)
	// DeleteMonthlyGrade removes the grade on the key; no row is not an error.
	// lock works as for UpsertMonthlyGrade.
	DeleteMonthlyGrade(ctx context.Context, enrollmentID, subjectID string, month domain.Month, lock PeriodLock) error
	// PeriodClosures reads the stored closures of these terms in ONE query,
	// keyed by term id; a term never closed is absent (open, version 0).
	PeriodClosures(ctx context.Context, termIDs []string) (map[string]domain.PeriodClosure, error)
	// MonthlyGradesForEnrollments lists, in ONE query, every grade of these
	// enrollments in these months. Empty ids or months returns nothing.
	MonthlyGradesForEnrollments(ctx context.Context, enrollmentIDs []string, months []domain.Month) ([]domain.MonthlyGrade, error)
	// MonthlyGradesForSheet batch-loads, in ONE query, the grades of many
	// enrollments for one subject and month, keyed by enrollment id (absent =
	// not captured). Empty enrollmentIDs returns an empty map without querying.
	MonthlyGradesForSheet(ctx context.Context, enrollmentIDs []string, subjectID string, month domain.Month) (map[string]domain.MonthlyGrade, error)
	// MonthlyGradesForEnrollment lists every monthly grade of one enrollment.
	MonthlyGradesForEnrollment(ctx context.Context, enrollmentID string) ([]domain.MonthlyGrade, error)
	// CountScaleViolations counts, in ONE query, the grades of these
	// enrollments that check refuses. Empty enrollmentIDs counts 0.
	CountScaleViolations(ctx context.Context, enrollmentIDs []string, check ScaleCheck) (int, error)
}

// ScaleCheck is a proposed level scale to judge captured grades against.
// GradingEnabled=false (disabling numeric grading) refuses every grade.
type ScaleCheck struct {
	GradingEnabled bool
	Scale          domain.GradingScale
}

// PeriodLock names the term whose closure guards a monthly write; the zero
// value (no term covers the month) guards nothing.
type PeriodLock struct {
	TermID string
}

// PeriodTx is what a period's close or reopen may do inside its one
// transaction, which holds the term's exclusive lock: no grade of the term can
// be written until it commits.
type PeriodTx interface {
	// Closure is the locked term's stored closure; found is false when the term
	// was never closed.
	Closure(ctx context.Context) (closure domain.PeriodClosure, found bool, err error)
	// MonthlyGrades reads the enrollments' grades in these months, under the lock.
	MonthlyGrades(ctx context.Context, enrollmentIDs []string, months []domain.Month) ([]domain.MonthlyGrade, error)
	// SaveClosure stores the closure (insert or update every column).
	SaveClosure(ctx context.Context, c domain.PeriodClosure) error
	// IssueReportCards marks EVERY published report card of the term
	// superseded (a withdrawn pupil's included), then inserts the cards with
	// their lines.
	IssueReportCards(ctx context.Context, cards []domain.ReportCard) error
}

// ReportCardFilter narrows a report-card list; empty fields do not filter.
type ReportCardFilter struct {
	TermID    string
	GroupID   string
	StudentID string
}

// PeriodRepository stores period closures and report cards (T7). All access
// is tenant-scoped for RLS.
type PeriodRepository interface {
	// WithPeriodLock runs fn in ONE transaction holding the term's exclusive
	// lock; fn's error rolls everything back.
	WithPeriodLock(ctx context.Context, termID string, fn func(ctx context.Context, tx PeriodTx) error) error
	// LatestReportCards lists the PUBLISHED report cards matching filter
	// (without lines), ordered by student name.
	LatestReportCards(ctx context.Context, filter ReportCardFilter) ([]domain.ReportCard, error)
	// ReportCard reads one report card with its lines (any status). Unknown id
	// is glerrors.ErrNotFound.
	ReportCard(ctx context.Context, id string) (domain.ReportCard, error)
}

// ReportCardPublisher announces an issued report card (GradesPublished).
type ReportCardPublisher interface {
	PublishReportCardIssued(ctx context.Context, rc domain.ReportCard) error
}
