package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// This file declares the seams the students use cases depend on. Interfaces live
// here (consumer side); adapters (postgres, pubsub) implement them. The domain
// imports none of this — dependencies point inward.

// ListFilter narrows a ListStudents query. Zero values mean "no filter".
type ListFilter struct {
	LevelID    string
	Status     domain.AdmissionStatus
	GradeGroup string
	// IDs narrows the list to these students (empty = any); at most one page.
	IDs       []string
	PageSize  int
	PageToken string
	// Birthday narrows the list to pupils born on a calendar day (nil = any).
	Birthday *BirthdayFilter
	// ActiveEnrollment narrows the list to pupils with an active enrollment,
	// whatever their admission status except withdrawn or rejected (owner
	// decision D4).
	ActiveEnrollment bool
}

// BirthdayFilter is a calendar day without a year. IncludeFeb29 also matches
// pupils born on February 29 — asked for on February 28 of a year without one.
type BirthdayFilter struct {
	Month        int
	Day          int
	IncludeFeb29 bool
}

// StudentRepository loads and persists students. All access is tenant-scoped for
// RLS (the adapter binds app.current_tenant per transaction, as in iam).
type StudentRepository interface {
	// Create inserts a new applicant, numbering its matrícula in numberYear
	// (the school's calendar year of the registration). A duplicate CURP (per
	// tenant) is KindConflict.
	Create(ctx context.Context, s *domain.Student, numberYear int) error
	// Update persists a status transition (and any mutable fields).
	Update(ctx context.Context, s *domain.Student) error
	// UpdateDetails persists a correction of the file: names, CURP and the
	// profile. A CURP another pupil holds is KindConflict.
	UpdateDetails(ctx context.Context, s *domain.Student) error
	// GetMedical returns the pupil's medical record (empty when none was
	// captured); SaveMedical upserts it. A pupil the tenant cannot see is
	// KindNotFound on save.
	GetMedical(ctx context.Context, studentID string) (domain.MedicalRecord, error)
	// PhotoRef returns where a pupil's photo is stored (KindNotFound when the
	// pupil has none); SavePhotoRef replaces it; DeletePhotoRef removes it.
	PhotoRef(ctx context.Context, studentID string) (PhotoRef, error)
	SavePhotoRef(ctx context.Context, studentID string, ref PhotoRef) error
	DeletePhotoRef(ctx context.Context, studentID string) error
	SaveMedical(ctx context.Context, studentID string, rec domain.MedicalRecord) error
	// FindByID returns a student or KindNotFound.
	FindByID(ctx context.Context, id string) (*domain.Student, error)
	// List returns students matching the filter plus the next page token ("" = last page).
	List(ctx context.Context, f ListFilter) (students []*domain.Student, nextPageToken string, err error)
}

// EventPublisher emits the module's domain events (StudentEnrolled,
// AdmissionStatusChanged). The pubsub adapter implements it; the use case calls
// it after a successful state transition is persisted.
type EventPublisher interface {
	PublishStudentEnrolled(ctx context.Context, ev domain.StudentEnrolledEvent) error
	PublishAdmissionChanged(ctx context.Context, ev domain.AdmissionChangedEvent) error
	PublishOutboxEvent(ctx context.Context, ev OutboxEvent) error
}

// OutboxEvent is the transport-ready, proto-encoded event persisted with its
// aggregate change.
type OutboxEvent struct {
	TenantID string
	Type     string
	Payload  []byte
}

// PhotoRef is where a pupil's photo lives in the object store.
type PhotoRef struct {
	Key         string
	ContentType string
	Size        int
}

// PhotoStore keeps photo bytes in a private object store (a GCS bucket; the
// fake-gcs emulator locally). Objects are never exposed: the gateway serves
// them after its own permission check. A missing object is glerrors.ErrNotFound.
type PhotoStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// LevelCatalog resolves a level id against config's catalog of levels — the
// school's own list, not a fixed SEP one (ADR 0014). An unknown id is reported
// as glerrors.ErrNotFound (KindNotFound).
type LevelCatalog interface {
	LevelByID(ctx context.Context, id string) (domain.Level, error)
}

// GroupRef is the students-owned view of a config group. It is the minimum the
// enroll knot needs: the group's identity and the cycle it derives from.
type GroupRef struct {
	ID      string
	CycleID string
	// Name is what the pupil's record says they are in ("1° A"). It travels with
	// the reference so BOTH enrolment paths can write it: the one that takes a
	// group and the one that takes a grade. Only one of them used to, which meant
	// a record could keep whatever text somebody typed while the placement said
	// something else.
	Name         string
	GradeLevelID string
	// Capacity is the seats the group holds; a transfer into it is refused
	// when they are taken.
	Capacity int
}

// CycleRef is the students-owned view of a config cycle: what an enrolment
// with a chosen entry day is bounded by. StartsOn is a calendar date (midnight
// UTC, from config's date column).
type CycleRef struct {
	ID       string
	StartsOn time.Time
}

// TermRef is the students-owned view of a config term (its identity and cycle).
type TermRef struct {
	ID      string
	CycleID string
}

// CatalogValidator validates cross-service references against the config bounded
// context (by RPC). It is a consumer-defined seam: the use case depends on this
// interface and stays testable with a fake, while the real adapter is a Connect
// client to config. A missing group/term is reported as glerrors.ErrNotFound
// (KindNotFound); any other error propagates unchanged.
type CatalogValidator interface {
	GroupByID(ctx context.Context, id string) (GroupRef, error)
	TermByID(ctx context.Context, id string) (TermRef, error)
	// CycleByID returns a cycle's first day, the earliest entry day an
	// enrolment into it may carry. A missing cycle is glerrors.ErrNotFound.
	CycleByID(ctx context.Context, id string) (CycleRef, error)
	// GroupsOfGrade returns the ACTIVE cycle's groups for a grade, in the order a
	// school fills them (A, then B, then C), each with the seats it holds.
	//
	// The cycle is resolved by config, not passed in: a caller choosing the year
	// a child joins is a caller who can put them in the wrong one.
	GroupsOfGrade(ctx context.Context, gradeLevelID string) ([]GroupSeats, error)
	// LevelOfGrade returns the config level a grade belongs to, so a pupil is
	// never placed in another level's group. A missing grade is
	// glerrors.ErrNotFound.
	LevelOfGrade(ctx context.Context, gradeLevelID string) (string, error)
}

// TransferCommand is one transfer: which enrollment, the group it moves to
// (with its name and seats), and the history row that records why.
type TransferCommand struct {
	EnrollmentID string
	To           GroupSeats
	Transfer     domain.EnrollmentTransfer
}

// GroupSeats is a group and how many pupils it may hold. Occupancy is NOT here:
// who is enrolled is students' own data, and asking config for it would make two
// services disagree about the same count.
type GroupSeats struct {
	ID       string
	Name     string
	CycleID  string
	Capacity int
}

// EnrollmentRepository loads and persists enrollments and their transfer history.
// All access is tenant-scoped for RLS (the adapter binds app.current_tenant per
// transaction, as in students/iam).
type EnrollmentRepository interface {
	// CreateEnrollment places e in the chosen group (seats) under the same
	// per-group lock as the other placements. A full group, one with no size,
	// or a duplicate (student, cycle) per tenant is KindConflict.
	CreateEnrollment(ctx context.Context, e domain.Enrollment, seats GroupSeats) error
	// CreateEnrollmentInFirstFreeGroup places a pupil in the first of `groups`
	// that still has room, and returns which one, ALL INSIDE ONE TRANSACTION.
	//
	// Choosing outside the transaction would be a race with a real cost: two
	// secretaries enrolling at the same moment would each read 24 of 25 seats
	// taken in 1°A and each take the last one. The adapter serialises per group
	// so the second caller sees the first's pupil and moves on to 1°B.
	//
	// Every group full is glerrors.KindConflict — the school has to open another
	// group, which is a decision, not something to do on its behalf.
	CreateEnrollmentInFirstFreeGroup(
		ctx context.Context,
		e domain.Enrollment,
		groups []GroupSeats,
	) (chosen GroupSeats, err error)
	// GetEnrollment returns an enrollment or KindNotFound.
	GetEnrollment(ctx context.Context, id string) (domain.Enrollment, error)
	// ListEnrollments returns enrollments matching the given filters; an empty
	// string skips that filter.
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error)
	// TransferEnrollment moves an enrollment in ONE transaction: it reserves the
	// target group and refuses it at capacity (KindConflict), writes the
	// transfer history row, updates the enrollment's group, and names the new
	// group on the pupil's file.
	TransferEnrollment(ctx context.Context, cmd TransferCommand) error
	// ListTransfers returns the moves of one student's enrollments, oldest
	// moved_at first (empty when the pupil never moved).
	ListTransfers(ctx context.Context, studentID string) ([]domain.EnrollmentTransfer, error)
	// ListTransfersIntoGroup returns every move INTO one group, oldest moved_at
	// first — one read for a whole class list.
	ListTransfersIntoGroup(ctx context.Context, groupID string) ([]domain.EnrollmentTransfer, error)
	// SetEnrollmentStatus updates an enrollment's status (e.g. to withdrawn).
	SetEnrollmentStatus(ctx context.Context, id string, status domain.EnrollmentStatus) error
	// WithdrawEnrollment atomically stores the dated withdrawal and its outbox event.
	WithdrawEnrollment(ctx context.Context, e domain.Enrollment) error
	// DrainOutbox publishes due events and records success or retry backoff.
	DrainOutbox(ctx context.Context, limit int, publish func(context.Context, OutboxEvent) error) (int, error)
}

// GuardianRepository loads and persists guardian ↔ student links. All access is
// tenant-scoped for RLS (the adapter binds app.current_tenant per transaction, as
// in students/enrollments).
type GuardianRepository interface {
	// Link inserts a guardian ↔ student link. A duplicate
	// (guardian, student) per tenant is KindConflict.
	Link(ctx context.Context, l domain.GuardianLink) error
	// Unlink removes a link. An unknown (or invisible) link is KindNotFound.
	Unlink(ctx context.Context, guardianUserID, studentID string) error
	// ByID returns one contact/link by its own id. A row the bound tenant cannot
	// see is KindNotFound, so RLS makes a cross-tenant read indistinguishable
	// from a missing contact.
	ByID(ctx context.Context, id string) (domain.GuardianLink, error)
	// UpdateContact persists an edited contact (relationship, person, the
	// emergency mark). The account is NOT written here. Unknown is KindNotFound.
	UpdateContact(ctx context.Context, l domain.GuardianLink) error
	// RemoveContact deletes a contact by id. Unknown is KindNotFound.
	RemoveContact(ctx context.Context, id string) error
	// AttachAccount sets the iam account on a contact that has none, keeping the
	// row (and the person on it) as it is. The write is conditional on the
	// contact STILL having no account: two grants racing must not let the second
	// overwrite the first, so a contact that already has one is KindConflict.
	AttachAccount(ctx context.Context, l domain.GuardianLink) error
	// ListByStudent returns the links of one student (its guardians).
	ListByStudent(ctx context.Context, studentID string) ([]domain.GuardianLink, error)
	// ListByGuardian returns the links of one guardian (their students).
	ListByGuardian(ctx context.Context, guardianUserID string) ([]domain.GuardianLink, error)
}

// UserRef is the students-owned view of an iam user: the minimum the guardian
// link needs to decide whether the referenced principal may be a guardian — its
// identity, account status, and the names of the roles it holds. Both are
// decision inputs and both are enforced: only an ACTIVE user holding the guardian
// role may be linked. Status carries iam's UserStatus enum in its string form
// (e.g. "USER_STATUS_ACTIVE"), which is what the iamclient adapter projects.
type UserRef struct {
	ID     string
	Status string
	Roles  []string
}

// UserReader resolves a user against the iam bounded context (by RPC). Like
// CatalogValidator it is a consumer-defined seam: the use case depends on this
// interface and stays testable with a fake, while the real adapter is a Connect
// client to iam. A missing user is reported as glerrors.ErrNotFound
// (KindNotFound); any other error propagates unchanged.
type UserReader interface {
	ByID(ctx context.Context, userID string) (UserRef, error)
}

// NoteRepository keeps the notes staff write about a pupil. All access is
// tenant-scoped. AddNote refuses a pupil the tenant cannot see (KindNotFound);
// NotesOf returns voided notes too, newest first; VoidNote records a void once
// and answers KindConflict to a second one.
type NoteRepository interface {
	AddNote(ctx context.Context, n domain.Note) error
	NotesOf(ctx context.Context, studentID string) ([]domain.Note, error)
	NoteByID(ctx context.Context, id string) (domain.Note, error)
	VoidNote(ctx context.Context, n domain.Note) error
	// EditNote stores an edit and its revision in one transaction. It applies
	// only while the note is not voided and still says rev.PreviousBody;
	// otherwise KindConflict (two edits at once, or a stale page).
	EditNote(ctx context.Context, n domain.Note, rev domain.NoteRevision) error
}

// CardRepository keeps the codes of the pupils' virtual cards (task 13).
type CardRepository interface {
	// ActiveCard returns a pupil's active code, if any.
	ActiveCard(ctx context.Context, studentID string) (domain.CardToken, bool, error)
	// SaveCard stores a newly issued code.
	SaveCard(ctx context.Context, c domain.CardToken) error
	// RevokeCards retires every active code of a pupil.
	RevokeCards(ctx context.Context, studentID string, at time.Time) error
	// EverIssued reports whether the pupil ever had a card (a revoked one counts).
	EverIssued(ctx context.Context, studentID string) (bool, error)
	// FindCard returns the card carrying a token, active or not, or KindNotFound.
	FindCard(ctx context.Context, token string) (domain.CardToken, error)
}

// LevelPeople is what deleting a level reaches in students (ADR 0021): the
// pupils placed in it (kept, with the level cleared), the enrollments in its
// groups (archived) and the counts by kind.
type LevelPeople struct {
	Counts        levelarchive.Counts
	StudentIDs    []string
	EnrollmentIDs []string
}

// LevelArchiveRepository moves students' part of a level into the trash and
// back (ADR 0021).
//
//   - PreviewLevelDeletion changes nothing.
//   - ArchiveLevel archives the enrollments in the scope's groups (and their
//     transfers) and clears the level of its pupils, in ONE transaction; a
//     retry of the same batch answers the first counts.
//   - RestoreLevelBatch puts them back; a pupil given another level meanwhile
//     keeps it (the pupils' snapshot is consumed: final), and an enrollment
//     that now conflicts is skipped and STAYS archived — reported, and named
//     in UnrestoredEnrollmentIDs so grades and attendance keep its rows
//     archived too. A retry tries what is still archived. A batch with no
//     archive here is levelarchive.ErrBatchNotFound.
//   - PurgeLevelBatch drops one batch's archive (the gateway's sweep).
//   - PurgeLevelArchives drops archives filed before cutoff.
type LevelArchiveRepository interface {
	PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (LevelPeople, error)
	ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestore, error)
	PurgeLevelBatch(ctx context.Context, batchID string) (int64, error)
	PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error)
}

// LevelRestore is what students' restore did: the report, and the batch's
// enrollments still in the trash after it (they conflicted).
type LevelRestore struct {
	Report                  levelarchive.Report
	UnrestoredEnrollmentIDs []string
}

// GroupUsage is what students keeps of one config group: every enrollment
// ever placed in it (any status) and every transfer out of or into it.
type GroupUsage struct {
	Enrollments int64
	Transfers   int64
}

// GroupRecordRepository answers for the records students keeps of one config
// group (T8, ADR 0036): how many there are, so the gateway can refuse to
// delete a group with history, and the group label on the pupils' files,
// rewritten after config renames the group.
//
//   - GroupUsage counts, tenant-bound; an unknown group is zeros (students
//     does not own groups, only records of them).
//   - RefreshGroupName sets grade_group = name on the file of every pupil
//     ACTIVELY enrolled in the group and answers how many files changed
//     (0 on a repeat: idempotent).
type GroupRecordRepository interface {
	GroupUsage(ctx context.Context, groupID string) (GroupUsage, error)
	RefreshGroupName(ctx context.Context, groupID, name string) (int64, error)
}
