package domain

import (
	"fmt"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Student is the core entity: an applicant/student moving through the admission
// state machine. Transition methods (Accept/Reject/Enroll/Withdraw) enforce the
// machine and return the domain events to publish — the entity is the single
// place admission rules live.
type Student struct {
	id        string
	tenantID  string
	curp      CURP
	firstName string
	// lastName is the PATERNAL surname. A pupil captured before the maternal
	// field existed may carry both surnames here; it is kept as written (T3).
	lastName         string
	maternalLastName string
	level            Level
	gradeGroup       string
	guardianName     string
	status           AdmissionStatus
	profile          Profile
	// enrollmentNumber is the matrícula (task 13): <year>-<five digits>, one
	// run per school and year, given when the pupil is first stored.
	enrollmentNumber string
	// bankReference is the BBVA deposit reference (ADR 0033); empty if none.
	bankReference BankReference
	// openIssues counts the pupil's unresolved import issues, as read. It is
	// a read-side count, never written through the entity.
	openIssues int
}

// AssignOpenIssues records how many import issues of the pupil are open.
func (s *Student) AssignOpenIssues(n int) { s.openIssues = n }

// OpenIssues is how many import issues of the pupil nobody resolved yet.
func (s *Student) OpenIssues() int { return s.openIssues }

// AssignEnrollmentNumber records the matrícula the store gave the pupil.
func (s *Student) AssignEnrollmentNumber(number string) { s.enrollmentNumber = number }

// EnrollmentNumber is the matrícula; empty until the pupil is stored.
func (s *Student) EnrollmentNumber() string { return s.enrollmentNumber }

// FormatEnrollmentNumber renders a matrícula: 2026-00042.
func FormatEnrollmentNumber(year, seq int) string { return fmt.Sprintf("%04d-%05d", year, seq) }

// NewApplicantParams carries the fields to create an applicant.
type NewApplicantParams struct {
	ID        string
	TenantID  string
	CURP      CURP
	FirstName string
	LastName  string // the paternal surname
	// MaternalLastName is optional: empty when not captured.
	MaternalLastName string
	Level            Level
	GradeGroup       string
	GuardianName     string
	// Profile is the file's editable half (birth date, contact, address). A new
	// applicant may arrive with its birth date (T11); the rest starts empty.
	Profile Profile
	// BankReference is optional: the zero value is a pupil without one.
	BankReference BankReference
}

// AdmissionChangedEvent is emitted on every admission transition (maps to the
// AdmissionStatusChanged proto event).
type AdmissionChangedEvent struct {
	StudentID string
	TenantID  string
	Previous  AdmissionStatus
	New       AdmissionStatus
	Reason    string
}

// StudentEnrolledEvent is emitted when a student is enrolled into a cycle (maps
// to the StudentEnrolled proto event).
type StudentEnrolledEvent struct {
	StudentID string
	TenantID  string
	CycleID   string
	LevelID   string
	// EnrolledAt is when the place starts: 00:00 Morelia of a chosen entry
	// day, or the instant of the alta.
	EnrolledAt time.Time
}

// NewApplicant creates a student in the pending admission state.
func NewApplicant(p NewApplicantParams) (*Student, error) {
	if strings.TrimSpace(p.ID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if strings.TrimSpace(p.TenantID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(p.FirstName) == "" || strings.TrimSpace(p.LastName) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "first and last name are required")
	}
	if p.Level.ID() == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "a level is required")
	}
	return &Student{
		id:               p.ID,
		tenantID:         p.TenantID,
		curp:             p.CURP,
		firstName:        strings.TrimSpace(p.FirstName),
		lastName:         strings.TrimSpace(p.LastName),
		maternalLastName: strings.TrimSpace(p.MaternalLastName),
		level:            p.Level,
		gradeGroup:       strings.TrimSpace(p.GradeGroup),
		guardianName:     strings.TrimSpace(p.GuardianName),
		status:           AdmissionPending,
		profile:          p.Profile,
		bankReference:    p.BankReference,
	}, nil
}

// NewImported creates a pupil brought from the previous system (ADR 0033).
// They were already enrolled — and paid — there, so they enter enrolled. The
// machine is walked (pending → accepted → enrolled), not jumped, so the rules
// stay in one place; the events are discarded on purpose: an import announces
// nothing (no StudentEnrolled, no AdmissionStatusChanged).
func NewImported(p NewApplicantParams) (*Student, error) {
	s, err := NewApplicant(p)
	if err != nil {
		return nil, err
	}
	if _, err := s.Accept(); err != nil {
		return nil, err
	}
	if _, _, err := s.Enroll(""); err != nil {
		return nil, err
	}
	return s, nil
}

// ErrDraftNotPlaceable refuses to enroll, move, or link a family to a draft:
// the school does not know who the pupil is until their CURP is captured
// (PLAN-import-avisos D3). CodeStudentDraft is its machine code.
var ErrDraftNotPlaceable = glerrors.New(glerrors.KindConflict,
	"ese alumno es un borrador sin CURP: captura su CURP antes de inscribirlo, cambiarlo de grupo o ligarlo a una familia")

// CodeStudentDraft is the X-Error-Code of ErrDraftNotPlaceable.
const CodeStudentDraft = "STUDENT_DRAFT"

// NewImportedDraft creates a pupil the legacy import brought without a valid
// CURP, as a draft: stored so nothing of the file is lost, with no enrollment.
// A pupil that has a CURP is imported, never drafted.
func NewImportedDraft(p NewApplicantParams) (*Student, error) {
	if !p.CURP.IsZero() {
		return nil, glerrors.New(glerrors.KindInvalid, "a pupil with a CURP is imported, not drafted")
	}
	s, err := NewApplicant(p)
	if err != nil {
		return nil, err
	}
	s.status = AdmissionDraft
	return s, nil
}

// IsDraft reports a pupil imported without a valid CURP.
func (s *Student) IsDraft() bool { return s.status == AdmissionDraft }

// RequirePlaceable is ErrDraftNotPlaceable for a draft, else nil: a draft is
// not enrolled, transferred, or linked to a family account.
func (s *Student) RequirePlaceable() error {
	if s.IsDraft() {
		return ErrDraftNotPlaceable
	}
	return nil
}

// Clone returns an independent copy, to compare a file before and after a
// correction (ChangedIssueFields).
func (s *Student) Clone() *Student {
	cp := *s
	cp.profile.phones = append([]Phone(nil), s.profile.phones...)
	return &cp
}

// Rehydrate rebuilds a Student from persisted state (used by the repository; it
// does NOT run the applicant validations, since the row was already valid).
func Rehydrate(p NewApplicantParams, status AdmissionStatus) *Student {
	return &Student{
		id: p.ID, tenantID: p.TenantID, curp: p.CURP,
		firstName: p.FirstName, lastName: p.LastName, maternalLastName: p.MaternalLastName, level: p.Level,
		gradeGroup: p.GradeGroup, guardianName: p.GuardianName, status: status,
		profile: p.Profile, bankReference: p.BankReference,
	}
}

// Accept moves a pending applicant to accepted.
func (s *Student) Accept() (AdmissionChangedEvent, error) {
	return s.transition(AdmissionAccepted, "")
}

// Reject terminally rejects a pending applicant.
func (s *Student) Reject(reason string) (AdmissionChangedEvent, error) {
	return s.transition(AdmissionRejected, reason)
}

// Withdraw terminally withdraws a pending, accepted, or enrolled student.
func (s *Student) Withdraw(reason string) (AdmissionChangedEvent, error) {
	return s.transition(AdmissionWithdrawn, reason)
}

// Enroll enrolls an accepted student into a cycle, returning both the
// StudentEnrolled event and the AdmissionStatusChanged event.
func (s *Student) Enroll(cycleID string) (StudentEnrolledEvent, AdmissionChangedEvent, error) {
	changed, err := s.transition(AdmissionEnrolled, "")
	if err != nil {
		return StudentEnrolledEvent{}, AdmissionChangedEvent{}, err
	}
	enrolled := StudentEnrolledEvent{
		StudentID: s.id,
		TenantID:  s.tenantID,
		CycleID:   cycleID,
		LevelID:   s.level.ID(),
	}
	return enrolled, changed, nil
}

// transition validates a move against the state machine, applies it, and returns
// the change event. An illegal move is a KindConflict (FailedPrecondition).
func (s *Student) transition(to AdmissionStatus, reason string) (AdmissionChangedEvent, error) {
	if !s.status.CanTransitionTo(to) {
		return AdmissionChangedEvent{}, glerrors.New(glerrors.KindConflict,
			"illegal admission transition: "+s.status.String()+" → "+to.String())
	}
	previous := s.status
	s.status = to
	return AdmissionChangedEvent{
		StudentID: s.id,
		TenantID:  s.tenantID,
		Previous:  previous,
		New:       to,
		Reason:    reason,
	}, nil
}

// Accessors.
func (s *Student) ID() string               { return s.id }
func (s *Student) TenantID() string         { return s.tenantID }
func (s *Student) CURP() CURP               { return s.curp }
func (s *Student) FirstName() string        { return s.firstName }
func (s *Student) LastName() string         { return s.lastName }
func (s *Student) MaternalLastName() string { return s.maternalLastName }
func (s *Student) Level() Level             { return s.level }
func (s *Student) GradeGroup() string       { return s.gradeGroup }
func (s *Student) GuardianName() string     { return s.guardianName }
func (s *Student) Status() AdmissionStatus  { return s.status }
func (s *Student) Profile() Profile         { return s.profile }

// BankReference is the pupil's BBVA reference; zero when none.
func (s *Student) BankReference() BankReference { return s.bankReference }

// ChangeBankReference sets (or, with the zero value, clears) the reference.
// Uniqueness per school is the store's to enforce.
func (s *Student) ChangeBankReference(ref BankReference) { s.bankReference = ref }

// UpdateDetails corrects the file: names, CURP and the profile. The level and
// the group are NOT here — a group changes by transfer, which keeps its history
// and its seat count. The first name and the paternal surname stay required, as
// when the pupil was created; the maternal surname is optional.
//
// The CURP is required except on a draft, and a draft given one is promoted to
// pending — the status any new applicant has, from which the caller may enroll
// them (the import's intended group rides on the draft's curp issue). Whether
// the CURP is another pupil's is the store's to say (ErrCURPTaken).
func (s *Student) UpdateDetails(firstName, lastName, maternalLastName string, curp CURP, profile Profile) error {
	_, err := s.CorrectDetails(firstName, lastName, maternalLastName, curp, profile)
	return err
}

// CorrectDetails is UpdateDetails returning the admission change a draft's
// promotion made (nil when the status did not move), to publish once stored.
func (s *Student) CorrectDetails(firstName, lastName, maternalLastName string, curp CURP, profile Profile) (*AdmissionChangedEvent, error) {
	if err := s.RequireEditable(); err != nil {
		return nil, err
	}
	first, last := strings.TrimSpace(firstName), strings.TrimSpace(lastName)
	if first == "" || last == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "first and last name are required")
	}
	if curp.IsZero() && !s.IsDraft() {
		return nil, glerrors.New(glerrors.KindInvalid, "a CURP is required")
	}
	var promoted *AdmissionChangedEvent
	if s.IsDraft() && !curp.IsZero() {
		ev, err := s.transition(AdmissionPending, "")
		if err != nil {
			return nil, err
		}
		promoted = &ev
	}
	s.firstName, s.lastName, s.curp, s.profile = first, last, curp, profile
	s.maternalLastName = strings.TrimSpace(maternalLastName)
	return promoted, nil
}

// CodeStudentWithdrawn is the machine code of ErrWithdrawnReadOnly (the
// X-Error-Code the gateway forwards): a 409 the screen must not read as a
// duplicate CURP.
const CodeStudentWithdrawn = "STUDENT_WITHDRAWN"

// ErrWithdrawnReadOnly refuses a correction of a withdrawn pupil's identity,
// medical record or photo: the school already closed the file. Contacts are
// not covered — the remaining debt is still collected by phone.
var ErrWithdrawnReadOnly = glerrors.New(glerrors.KindConflict, "ese alumno está dado de baja: su ficha ya no se corrige")

// RequireEditable is ErrWithdrawnReadOnly for a withdrawn pupil, else nil.
func (s *Student) RequireEditable() error {
	if s.status == AdmissionWithdrawn {
		return ErrWithdrawnReadOnly
	}
	return nil
}

// PlaceIn records the group the pupil ended up in.
//
// The field used to be free text a secretary typed ("1° A") with nothing
// checking it against the groups that exist. It is now written by the system
// when capacity chooses a group, so the record and the placement cannot drift
// apart — which they did the moment somebody typed a group that was full, or one
// that had never been created.
func (s *Student) PlaceIn(groupName string) {
	s.gradeGroup = strings.TrimSpace(groupName)
}
