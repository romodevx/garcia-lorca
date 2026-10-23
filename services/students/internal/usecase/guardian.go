package usecase

import (
	"context"
	"slices"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// guardianRoleName is the iam role a user must hold to be linked as a guardian.
// The family read path (a parent seeing only their children) authorizes on these
// links, so a user without the role must never acquire one.
const guardianRoleName = "guardian"

// linkableUserStatuses are the iam account statuses a guardian may be linked
// from. They are the values the iamclient adapter projects into
// ports.UserRef.Status — the string forms of iam's UserStatus enum.
//
// An ALLOW-LIST, not a deny-list: the link IS the family read path's
// authorization, so a status nobody thought about must fail closed. An
// UNSPECIFIED status is iam failing to say, and guessing "probably fine" about
// the field that decides who sees a child is the wrong way to be wrong.
//
// INVITED is in here, and the reason is the school's day: the office decides
// today who the mother is, and she opens her link tomorrow. Refusing until she
// does would mean a family cannot be recorded until it gets round to reading its
// email — and she can read nothing meanwhile anyway, because an invited account
// has no password and cannot sign in at all.
//
// SUSPENDED is not: that account could sign in once and was cut off, and handing
// it a child undoes the cutting off.
var linkableUserStatuses = []string{"USER_STATUS_ACTIVE", "USER_STATUS_INVITED"}

// GuardianService links guardians to students. It orchestrates three seams: the
// local StudentRepository (the student must exist in THIS context), the
// GuardianRepository, and the cross-service UserReader (iam, by RPC — faked in
// tests). It depends only on ports + domain.
//
//	Link: student exists (local) ─> guardian user exists, holds the guardian
//	      role, and its account is in a linkable state (RPC) ─> persist the link
//	      (unique per pair)
//
// Every account write (LinkGuardian, GrantContactAccess) runs through
// ports.FamilyAccessRepository.OnPupil: one transaction under the family
// locking rule, where a pupil in a family also gets the cap of
// domain.MaxFamilyAccounts and the sync to its siblings (ADR 0041).
type GuardianService struct {
	students  ports.StudentRepository
	guardians ports.GuardianRepository
	access    ports.FamilyAccessRepository
	users     ports.UserReader
	tenant    string
	newID     func() string
}

// NewGuardianService wires the use case. `tenant` is the single-tenant scope for
// this phase and `newID` mints link ids (injectable for deterministic tests).
func NewGuardianService(
	students ports.StudentRepository,
	guardians ports.GuardianRepository,
	access ports.FamilyAccessRepository,
	users ports.UserReader,
	tenant string,
	newID func() string,
) *GuardianService {
	return &GuardianService{
		students:  students,
		guardians: guardians,
		access:    access,
		users:     users,
		tenant:    tenant,
		newID:     newID,
	}
}

// LinkGuardian links an iam user to a student. Both references are validated
// before anything is written: a missing student, a missing user, an unknown
// relationship, a user that does not hold the guardian role, or a user whose iam
// account is not active are all KindInvalid (bad arguments, not missing
// resources); a duplicate link is KindConflict. A transport failure from iam
// propagates unchanged. On a pupil in a family the link is synced to the
// siblings, and a third account in the family is domain.ErrFamilyAccountLimit
// with nothing written.
func (s *GuardianService) LinkGuardian(ctx context.Context, guardianUserID, studentID, relationship string) (domain.GuardianLink, error) {
	// Parsed first: a malformed relationship is decidable without any I/O.
	rel, err := domain.ParseRelationship(relationship)
	if err != nil {
		return domain.GuardianLink{}, err
	}

	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.GuardianLink{}, glerrors.New(glerrors.KindInvalid, "student not found")
		}
		return domain.GuardianLink{}, err
	}
	// A draft (imported without a CURP) is not handed to a family account.
	if err := student.RequirePlaceable(); err != nil {
		return domain.GuardianLink{}, err
	}

	if err := s.requireGuardianUser(ctx, guardianUserID); err != nil {
		return domain.GuardianLink{}, err
	}

	link, err := domain.NewGuardianLink(s.newID(), s.tenant, guardianUserID, student.ID(), rel)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	eligible, err := s.eligibleFor(ctx, student, guardianUserID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	err = s.access.OnPupil(ctx, student.ID(), func(ctx context.Context, a ports.FamilyAccess) error {
		if err := a.InsertContact(ctx, link); err != nil {
			return err // KindConflict on a duplicate propagates
		}
		return s.sync().run(ctx, a, eligible)
	})
	if err != nil {
		return domain.GuardianLink{}, err
	}
	return link, nil
}

// requireGuardianUser resolves the user against iam and asserts it may act as a
// guardian: it holds the guardian role AND its account is in a linkable state
// (see linkableUserStatuses). Only iam's
// "does not exist" answer becomes a bad argument; any other failure (transport,
// iam internal) propagates so the edge does not blame the caller for an outage.
func (s *GuardianService) requireGuardianUser(ctx context.Context, guardianUserID string) error {
	user, err := s.users.ByID(ctx, guardianUserID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "guardian user not found")
		}
		return err
	}
	if !slices.Contains(user.Roles, guardianRoleName) {
		return glerrors.New(glerrors.KindInvalid, "user is not a guardian")
	}
	if !isLinkable(user) {
		return glerrors.New(glerrors.KindInvalid, "guardian account cannot be linked in its current state")
	}
	return nil
}

func (s *GuardianService) sync() familySync {
	return familySync{access: s.access, users: s.users, tenant: s.tenant, newID: s.newID}
}

// eligibleFor is which accounts the sync may hand on after an account write on
// student: the account just validated (vetted) and, when the pupil is in a
// family, the family's accounts iam still allows. A pupil read without a
// family has no siblings to hand anything to; should a family claim it before
// the lock, its other accounts stay ineligible (fail closed).
func (s *GuardianService) eligibleFor(ctx context.Context, student *domain.Student, vetted string) (map[string]bool, error) {
	if student.FamilyID() == "" {
		return map[string]bool{vetted: true}, nil
	}
	return s.sync().eligible(ctx, "", []string{student.ID()}, vetted)
}

// ContactInput is a person the school can reach about a pupil. Most of them
// have no account, which is the whole reason this exists.
type ContactInput struct {
	StudentID    string
	Relationship string
	FullName     string
	Phone        string
	Email        string
	IsEmergency  bool
}

// AddContact records somebody the school can call about a pupil, with no
// account and no login.
//
// Before this, recording that a grandmother may collect the child meant first
// creating an iam user for her — so the school did not record it, and the office
// kept the second number on paper while the system showed one contact per pupil
// as though that were the truth.
//
// The pupil is checked; nothing else can be. There is no user to validate,
// which is the point: a contact is a name and a number, not an account.
func (s *GuardianService) AddContact(ctx context.Context, in ContactInput) (domain.GuardianLink, error) {
	rel, err := domain.ParseRelationship(in.Relationship)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	student, err := s.students.FindByID(ctx, in.StudentID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.GuardianLink{}, glerrors.New(glerrors.KindInvalid, "student not found")
		}
		return domain.GuardianLink{}, err
	}

	contact, err := domain.NewContact(s.newID(), s.tenant, student.ID(), rel, domain.ContactPerson{
		FullName:  in.FullName,
		Phone:     in.Phone,
		Email:     in.Email,
		Emergency: in.IsEmergency,
	})
	if err != nil {
		return domain.GuardianLink{}, err
	}
	if err := s.guardians.Link(ctx, contact); err != nil {
		return domain.GuardianLink{}, err
	}
	return contact, nil
}

// UpdateContact corrects a contact of a pupil: relationship, name, phone, email
// and the emergency mark. studentID is required and checked, as in
// GrantContactAccess: a contact of another pupil answers KindNotFound, so the
// answer never confirms that it exists.
func (s *GuardianService) UpdateContact(ctx context.Context, contactID, studentID string, in ContactInput) (domain.GuardianLink, error) {
	contact, err := s.contactOf(ctx, contactID, studentID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	rel, err := domain.ParseRelationship(in.Relationship)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	edited, err := contact.Edit(rel, domain.ContactPerson{
		FullName: in.FullName, Phone: in.Phone, Email: in.Email, Emergency: in.IsEmergency,
	})
	if err != nil {
		return domain.GuardianLink{}, err
	}
	if err := s.guardians.UpdateContact(ctx, edited); err != nil {
		return domain.GuardianLink{}, err
	}
	return edited, nil
}

// RemoveContact deletes a contact of a pupil. A contact that can open the app
// is refused (KindConflict): its access is revoked first, deliberately, rather
// than disappearing with a phone number.
func (s *GuardianService) RemoveContact(ctx context.Context, contactID, studentID string) error {
	contact, err := s.contactOf(ctx, contactID, studentID)
	if err != nil {
		return err
	}
	if contact.HasAccount() {
		return glerrors.New(glerrors.KindConflict, "este contacto puede abrir la app: quítale el acceso antes de borrarlo")
	}
	return s.guardians.RemoveContact(ctx, contactID)
}

// contactOf loads a contact that belongs to studentID, or KindNotFound.
func (s *GuardianService) contactOf(ctx context.Context, contactID, studentID string) (domain.GuardianLink, error) {
	contact, err := s.guardians.ByID(ctx, contactID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	if contact.StudentID() != studentID {
		return domain.GuardianLink{}, glerrors.ErrNotFound
	}
	return contact, nil
}

// GrantContactAccess turns a contact the school already recorded into a family
// that can open the app, by attaching an iam account to the row that is already
// there.
//
// WHY NOT JUST LinkGuardian. That creates a NEW row from a user id, which would
// leave the pupil with two entries for one person: the contact the office typed
// (a name and a phone) and a second one carrying only an account. A contact list
// where the same mother appears twice is a list the office stops reading.
//
// The account is validated exactly as in LinkGuardian — it must hold the
// guardian role and be ACTIVE — because the link IS the family read path's
// authorization and it makes no difference how the link came to exist.
//
// studentID is a REQUIRED argument and it is checked here, in the service, not
// only at the edge. Without it a caller holding a service credential could name
// any contact id and hand that family's child to an account of its choosing. A
// contact that belongs to another pupil answers KindNotFound rather than a
// conflict or a refusal, so the answer does not confirm that the contact exists
// (the same posture as ADR 0004 and ADR 0010).
//
// On a pupil in a family the account is synced to the siblings and capped
// (domain.ErrFamilyAccountLimit, nothing written), as in LinkGuardian.
func (s *GuardianService) GrantContactAccess(ctx context.Context, contactID, studentID, guardianUserID string) (domain.GuardianLink, error) {
	if err := s.requireGuardianUser(ctx, guardianUserID); err != nil {
		return domain.GuardianLink{}, err
	}

	contact, err := s.guardians.ByID(ctx, contactID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	if contact.StudentID() != studentID {
		return domain.GuardianLink{}, glerrors.ErrNotFound
	}
	student, err := s.placeable(ctx, studentID)
	if err != nil {
		return domain.GuardianLink{}, err
	}

	withAccount, err := contact.WithAccount(guardianUserID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	eligible, err := s.eligibleFor(ctx, student, guardianUserID)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	err = s.access.OnPupil(ctx, studentID, func(ctx context.Context, a ports.FamilyAccess) error {
		if err := a.AttachAccount(ctx, withAccount); err != nil {
			return err
		}
		return s.sync().run(ctx, a, eligible)
	})
	if err != nil {
		return domain.GuardianLink{}, err
	}
	return withAccount, nil
}

// placeable loads the pupil and refuses a draft (domain.ErrDraftNotPlaceable):
// an app account is not given to the family of a pupil the school cannot
// identify.
func (s *GuardianService) placeable(ctx context.Context, studentID string) (*domain.Student, error) {
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if err := student.RequirePlaceable(); err != nil {
		return nil, err
	}
	return student, nil
}

// UnlinkGuardian removes a guardian ↔ student link. An unknown link is
// KindNotFound. A pupil in a family is domain.ErrFamilyManagedAccess and
// nothing is deleted: the family's sync would link the account back from a
// sibling, so access is revoked from the family (RevokeFamilyAccess). The
// check and the delete share OnPupil's transaction, so a family confirmed
// over the pupil meanwhile cannot slip between them.
func (s *GuardianService) UnlinkGuardian(ctx context.Context, guardianUserID, studentID string) error {
	return s.access.OnPupil(ctx, studentID, func(ctx context.Context, a ports.FamilyAccess) error {
		if a.FamilyID() != "" {
			return domain.ErrFamilyManagedAccess
		}
		return a.RemoveLink(ctx, guardianUserID, studentID)
	})
}

// ListGuardiansForStudent returns the links of one student (its guardians).
func (s *GuardianService) ListGuardiansForStudent(ctx context.Context, studentID string) ([]domain.GuardianLink, error) {
	return s.guardians.ListByStudent(ctx, studentID)
}

// ListStudentsForGuardian returns the links of one guardian (their students). It
// is the seam the gateway resolves a family caller's children through.
func (s *GuardianService) ListStudentsForGuardian(ctx context.Context, guardianUserID string) ([]domain.GuardianLink, error) {
	return s.guardians.ListByGuardian(ctx, guardianUserID)
}
