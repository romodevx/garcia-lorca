package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// StudentService orchestrates the students domain: it creates applicants and
// drives the admission state machine, persisting via the repository and
// publishing the resulting domain events. It depends only on ports + domain.
//
//	ChangeAdmissionStatus:
//	  load ─> domain transition (enforces the machine) ─> repo.Update ─> publish events
//	                      │illegal
//	                      └── KindConflict (nothing persisted/published)
type StudentService struct {
	repo   ports.StudentRepository
	pub    ports.EventPublisher
	levels ports.LevelCatalog
	tenant string
	newID  func() string
	now    func() time.Time
	photos ports.PhotoStore
	// zone is the school's time zone: a matrícula is numbered under the
	// school's calendar year (T5b). UTC until WithSchoolZone.
	zone *time.Location
}

// NewStudentService wires the use case. `levels` is config's level catalog;
// `tenant` is the single-tenant scope for M03; `newID` mints student ids
// (injectable for deterministic tests).
func NewStudentService(repo ports.StudentRepository, pub ports.EventPublisher, levels ports.LevelCatalog, tenant string, newID func() string) *StudentService {
	return &StudentService{repo: repo, pub: pub, levels: levels, tenant: tenant, newID: newID, now: time.Now, zone: time.UTC}
}

// WithSchoolZone returns the service numbering matrículas in the school's
// calendar year of `zone`, resolved once at boot.
func (s *StudentService) WithSchoolZone(zone *time.Location) *StudentService {
	cp := *s
	cp.zone = zone
	return &cp
}

// WithClock returns the service reading the school's date from `now`
// (deterministic tests; the default is time.Now).
func (s *StudentService) WithClock(now func() time.Time) *StudentService {
	cp := *s
	cp.now = now
	return &cp
}

// GetMedical returns a pupil's medical record. Callers reach it under
// medical:read (the gateway), not students:read.
func (s *StudentService) GetMedical(ctx context.Context, studentID string) (domain.MedicalRecord, error) {
	if _, err := s.repo.FindByID(ctx, studentID); err != nil {
		return domain.MedicalRecord{}, err
	}
	return s.repo.GetMedical(ctx, studentID)
}

// UpdateMedical validates and saves a pupil's medical record.
func (s *StudentService) UpdateMedical(ctx context.Context, studentID, bloodType, allergies, conditions string) (domain.MedicalRecord, error) {
	if err := s.requireEditable(ctx, studentID); err != nil {
		return domain.MedicalRecord{}, err
	}
	rec, err := domain.NewMedicalRecord(bloodType, allergies, conditions)
	if err != nil {
		return domain.MedicalRecord{}, err
	}
	if err := s.repo.SaveMedical(ctx, studentID, rec); err != nil {
		return domain.MedicalRecord{}, err
	}
	return rec, nil
}

// requireEditable is KindNotFound for an unknown pupil and
// domain.ErrWithdrawnReadOnly for a withdrawn one (T14-009). The repository
// re-checks inside the write, so a withdrawal committed meanwhile still wins.
func (s *StudentService) requireEditable(ctx context.Context, studentID string) error {
	student, err := s.repo.FindByID(ctx, studentID)
	if err != nil {
		return err
	}
	return student.RequireEditable()
}

// WithPhotos returns the service keeping photos in `store`. Without it, photo
// calls answer KindNotFound: a deployment with no bucket has no photos, and
// still serves the rest of the file.
func (s *StudentService) WithPhotos(store ports.PhotoStore) *StudentService {
	cp := *s
	cp.photos = store
	return &cp
}

var errPhotosUnavailable = glerrors.New(glerrors.KindNotFound, "photos are not enabled")

// UploadPhoto stores a pupil's photo (JPEG/PNG by its bytes, at most 2 MB) and
// replaces any previous one, deleting its object. The object key names the
// tenant and the pupil's id and a random suffix — no personal data.
func (s *StudentService) UploadPhoto(ctx context.Context, studentID string, data []byte) (ports.PhotoRef, error) {
	if s.photos == nil {
		return ports.PhotoRef{}, errPhotosUnavailable
	}
	if err := s.requireEditable(ctx, studentID); err != nil {
		return ports.PhotoRef{}, err
	}
	photo, err := domain.NewPhoto(data)
	if err != nil {
		return ports.PhotoRef{}, err
	}
	previous, err := s.repo.PhotoRef(ctx, studentID)
	if err != nil && glerrors.KindOf(err) != glerrors.KindNotFound {
		return ports.PhotoRef{}, err
	}
	ref := ports.PhotoRef{
		Key:         s.tenant + "/students/" + studentID + "/" + s.newID(),
		ContentType: photo.ContentType(),
		Size:        len(photo.Data()),
	}
	if err := s.photos.Put(ctx, ref.Key, ref.ContentType, photo.Data()); err != nil {
		return ports.PhotoRef{}, err
	}
	if err := s.repo.SavePhotoRef(ctx, studentID, ref); err != nil {
		// Nothing points at the new object: remove it rather than let it
		// accumulate, and report both failures.
		return ports.PhotoRef{}, errors.Join(err, s.photos.Delete(ctx, ref.Key))
	}
	if previous.Key != "" && previous.Key != ref.Key {
		// Best-effort cleanup, deliberately not an error: the new photo is
		// already stored and referenced, so the upload HAS succeeded, and
		// reporting a failure would tell the office it was not saved. A failed
		// delete leaves an unreferenced object that nothing sweeps by itself;
		// the store logs its key (no personal data) so it can be removed.
		_ = s.photos.Delete(ctx, previous.Key)
	}
	return ref, nil
}

// Photo returns a pupil's photo, or KindNotFound when there is none.
func (s *StudentService) Photo(ctx context.Context, studentID string) (domain.Photo, error) {
	if s.photos == nil {
		return domain.Photo{}, errPhotosUnavailable
	}
	ref, err := s.repo.PhotoRef(ctx, studentID)
	if err != nil {
		return domain.Photo{}, err
	}
	data, err := s.photos.Get(ctx, ref.Key)
	if err != nil {
		return domain.Photo{}, err
	}
	return domain.NewPhoto(data)
}

// HasPhoto reports whether Photo would find one, without reading its bytes:
// one reference lookup. A deployment without a bucket has none (T13).
func (s *StudentService) HasPhoto(ctx context.Context, studentID string) (bool, error) {
	if s.photos == nil {
		return false, nil
	}
	_, err := s.repo.PhotoRef(ctx, studentID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeletePhoto removes a pupil's photo: the reference first, so the file never
// points at a missing object, then the object.
func (s *StudentService) DeletePhoto(ctx context.Context, studentID string) error {
	if s.photos == nil {
		return errPhotosUnavailable
	}
	if err := s.requireEditable(ctx, studentID); err != nil {
		return err
	}
	ref, err := s.repo.PhotoRef(ctx, studentID)
	if err != nil {
		return err
	}
	if err := s.repo.DeletePhotoRef(ctx, studentID); err != nil {
		return err
	}
	return s.photos.Delete(ctx, ref.Key)
}

// UpdateDetailsInput is a correction of a pupil's file as captured: names, CURP
// and the profile. Level and group are not here: a group changes by transfer.
type UpdateDetailsInput struct {
	ID        string
	FirstName string
	LastName  string // the paternal surname
	// MaternalLastName is optional (T3).
	MaternalLastName string
	CURP             string
	Profile          domain.ProfileInput
	// BankReference is the BBVA reference (ADR 0033): nil keeps the stored
	// one, "" clears it, anything else replaces it.
	BankReference *string
}

// UpdateDetails validates and persists a correction of the pupil's file. A CURP
// another pupil already holds is KindConflict.
func (s *StudentService) UpdateDetails(ctx context.Context, in UpdateDetailsInput) (*domain.Student, error) {
	student, err := s.repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	curp, err := domain.NewCURP(in.CURP)
	if err != nil {
		return nil, err
	}
	profile, err := domain.NewProfile(in.Profile, s.now())
	if err != nil {
		return nil, err
	}
	if err := student.UpdateDetails(in.FirstName, in.LastName, in.MaternalLastName, curp, profile); err != nil {
		return nil, err
	}
	if in.BankReference != nil {
		ref, err := domain.NewBankReference(*in.BankReference)
		if err != nil {
			return nil, err
		}
		student.ChangeBankReference(ref)
	}
	if err := s.repo.UpdateDetails(ctx, student); err != nil {
		return nil, err
	}
	return student, nil
}

// CreateApplicantInput is the raw input for a new applicant. CURP is validated
// here; LevelID is resolved against config's catalog, which also supplies the
// level's name — the caller never names the level itself.
type CreateApplicantInput struct {
	CURP      string
	FirstName string
	LastName  string // the paternal surname
	// MaternalLastName is optional (T3).
	MaternalLastName string
	LevelID          string
	GradeGroup       string
	GuardianName     string
	// BirthDate is optional at intake (T11): ISO "2019-03-12", or empty.
	BirthDate string
	// BankReference is the optional BBVA reference (ADR 0033).
	BankReference string
}

// CreateApplicant validates the input, builds a pending applicant, and persists
// it. A duplicate CURP (per tenant) surfaces as KindConflict from the repo.
func (s *StudentService) CreateApplicant(ctx context.Context, in CreateApplicantInput) (*domain.Student, error) {
	curp, err := domain.NewCURP(in.CURP)
	if err != nil {
		return nil, err
	}
	bankRef, err := domain.NewBankReference(in.BankReference)
	if err != nil {
		return nil, err
	}
	level, err := s.levelOf(ctx, in.LevelID)
	if err != nil {
		return nil, err
	}
	// Only the birth date is captured at intake; the rest of the profile is
	// filled from the file. Same rule as an edit: a date after today is a typo.
	profile, err := domain.NewProfile(domain.ProfileInput{BirthDate: in.BirthDate}, s.now())
	if err != nil {
		return nil, err
	}
	student, err := domain.NewApplicant(domain.NewApplicantParams{
		ID:               s.newID(),
		TenantID:         s.tenant,
		CURP:             curp,
		FirstName:        in.FirstName,
		LastName:         in.LastName,
		MaternalLastName: in.MaternalLastName,
		Level:            level,
		GradeGroup:       in.GradeGroup,
		GuardianName:     in.GuardianName,
		Profile:          profile,
		BankReference:    bankRef,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, student, s.now().In(s.zone).Year()); err != nil {
		return nil, err
	}
	return student, nil
}

// levelOf resolves a level id against config's catalog. A level config does not
// know is the caller's mistake (KindInvalid), not a missing student.
func (s *StudentService) levelOf(ctx context.Context, id string) (domain.Level, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Level{}, glerrors.New(glerrors.KindInvalid, "a level is required")
	}
	// Config ids are uuids. Anything else (an old SEP key like "kinder") is
	// refused here, where it is a 400 — in config it would be a query error.
	if _, err := uuid.Parse(id); err != nil {
		return domain.Level{}, glerrors.New(glerrors.KindInvalid, "level id is not a level id")
	}
	level, err := s.levels.LevelByID(ctx, id)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return domain.Level{}, glerrors.Wrap(glerrors.KindInvalid, "level not found", err)
	}
	return level, err
}

// ChangeStatusInput drives one admission transition.
type ChangeStatusInput struct {
	ID      string
	Target  domain.AdmissionStatus
	Reason  string // for reject/withdraw
	CycleID string // for enroll
}

// ChangeAdmissionStatus applies the requested transition to a student. The
// domain enforces the state machine (illegal → KindConflict); on success it
// persists and publishes the events. Events are published only after a
// successful Update, so a failed write publishes nothing.
func (s *StudentService) ChangeAdmissionStatus(ctx context.Context, in ChangeStatusInput) (*domain.Student, error) {
	student, err := s.repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	changed, enrolled, err := applyTransition(student, in)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, student); err != nil {
		return nil, err
	}

	if err := s.pub.PublishAdmissionChanged(ctx, changed); err != nil {
		return nil, err
	}
	if enrolled != nil {
		if err := s.pub.PublishStudentEnrolled(ctx, *enrolled); err != nil {
			return nil, err
		}
	}
	return student, nil
}

// applyTransition dispatches to the right domain method for the target status.
func applyTransition(student *domain.Student, in ChangeStatusInput) (domain.AdmissionChangedEvent, *domain.StudentEnrolledEvent, error) {
	switch in.Target {
	case domain.AdmissionAccepted:
		ev, err := student.Accept()
		return ev, nil, err
	case domain.AdmissionRejected:
		ev, err := student.Reject(in.Reason)
		return ev, nil, err
	case domain.AdmissionWithdrawn:
		ev, err := student.Withdraw(in.Reason)
		return ev, nil, err
	case domain.AdmissionEnrolled:
		enrolled, ev, err := student.Enroll(in.CycleID)
		if err != nil {
			return domain.AdmissionChangedEvent{}, nil, err
		}
		return ev, &enrolled, nil
	default:
		return domain.AdmissionChangedEvent{}, nil, glerrors.New(glerrors.KindInvalid, "unsupported target admission status")
	}
}

// GetStudent returns a student by id (KindNotFound if absent).
func (s *StudentService) GetStudent(ctx context.Context, id string) (*domain.Student, error) {
	return s.repo.FindByID(ctx, id)
}

// ListStudents returns students matching the filter plus the next page token.
func (s *StudentService) ListStudents(ctx context.Context, f ports.ListFilter) ([]*domain.Student, string, error) {
	if f.LevelID != "" {
		if _, err := uuid.Parse(f.LevelID); err != nil {
			return nil, "", glerrors.New(glerrors.KindInvalid, "level id is not a level id")
		}
	}
	if err := validateStudentIDs(f.IDs); err != nil {
		return nil, "", err
	}
	if f.Birthday != nil && !isCalendarDay(f.Birthday.Month, f.Birthday.Day) {
		return nil, "", glerrors.New(glerrors.KindInvalid, "birthday is not a day of the year")
	}
	return s.repo.List(ctx, f)
}

// leapYear is any leap year: a calendar day is valid when it exists in one, so
// February 29 is a day somebody can be born on.
const leapYear = 2024

// isCalendarDay reports whether month/day is a real day of some year. time.Date
// normalizes an impossible day (April 31 → May 1), so a round trip is the check.
func isCalendarDay(month, day int) bool {
	if month < 1 || month > 12 || day < 1 {
		return false
	}
	d := time.Date(leapYear, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return int(d.Month()) == month && d.Day() == day
}

// MaxListIDs is how many students one ids-filtered list may name: one page.
const MaxListIDs = 200

// validateStudentIDs checks an ids filter: at most MaxListIDs uuids.
func validateStudentIDs(ids []string) error {
	if len(ids) > MaxListIDs {
		return glerrors.New(glerrors.KindInvalid, "too many student ids")
	}
	for _, id := range ids {
		if err := uuid.Validate(id); err != nil {
			return glerrors.New(glerrors.KindInvalid, "student id is not a student id")
		}
	}
	return nil
}
