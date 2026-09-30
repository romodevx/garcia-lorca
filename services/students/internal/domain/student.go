package domain

import (
	"fmt"
	"strings"

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
}

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
	// applicant starts with an empty one.
	Profile Profile
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
	}, nil
}

// Rehydrate rebuilds a Student from persisted state (used by the repository; it
// does NOT run the applicant validations, since the row was already valid).
func Rehydrate(p NewApplicantParams, status AdmissionStatus) *Student {
	return &Student{
		id: p.ID, tenantID: p.TenantID, curp: p.CURP,
		firstName: p.FirstName, lastName: p.LastName, maternalLastName: p.MaternalLastName, level: p.Level,
		gradeGroup: p.GradeGroup, guardianName: p.GuardianName, status: status,
		profile: p.Profile,
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

// Withdraw terminally withdraws an accepted or enrolled student.
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

// UpdateDetails corrects the file: names, CURP and the profile. The level and
// the group are NOT here — a group changes by transfer, which keeps its history
// and its seat count. The first name and the paternal surname stay required, as
// when the pupil was created; the maternal surname is optional.
func (s *Student) UpdateDetails(firstName, lastName, maternalLastName string, curp CURP, profile Profile) error {
	first, last := strings.TrimSpace(firstName), strings.TrimSpace(lastName)
	if first == "" || last == "" {
		return glerrors.New(glerrors.KindInvalid, "first and last name are required")
	}
	s.firstName, s.lastName, s.curp, s.profile = first, last, curp, profile
	s.maternalLastName = strings.TrimSpace(maternalLastName)
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
