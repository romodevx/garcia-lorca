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

// activeUserStatus is the only iam account status a guardian may be linked from.
// It is the value the iamclient adapter projects into ports.UserRef.Status — the
// string form of iam's USER_STATUS_ACTIVE enum. A suspended account must not gain
// a link, for the same reason a role-less user must not: the link IS the family
// read path's authorization.
const activeUserStatus = "USER_STATUS_ACTIVE"

// GuardianService links guardians to students. It orchestrates three seams: the
// local StudentRepository (the student must exist in THIS context), the
// GuardianRepository, and the cross-service UserReader (iam, by RPC — faked in
// tests). It depends only on ports + domain.
//
//	Link: student exists (local) ─> guardian user exists, holds the guardian
//	      role, and its account is active (RPC) ─> persist the link
//	      (unique per pair)
type GuardianService struct {
	students  ports.StudentRepository
	guardians ports.GuardianRepository
	users     ports.UserReader
	tenant    string
	newID     func() string
}

// NewGuardianService wires the use case. `tenant` is the single-tenant scope for
// this phase and `newID` mints link ids (injectable for deterministic tests).
func NewGuardianService(
	students ports.StudentRepository,
	guardians ports.GuardianRepository,
	users ports.UserReader,
	tenant string,
	newID func() string,
) *GuardianService {
	return &GuardianService{
		students:  students,
		guardians: guardians,
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
// propagates unchanged.
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

	if err := s.requireGuardianUser(ctx, guardianUserID); err != nil {
		return domain.GuardianLink{}, err
	}

	link, err := domain.NewGuardianLink(s.newID(), s.tenant, guardianUserID, student.ID(), rel)
	if err != nil {
		return domain.GuardianLink{}, err
	}
	if err := s.guardians.Link(ctx, link); err != nil {
		return domain.GuardianLink{}, err // KindConflict on a duplicate propagates
	}
	return link, nil
}

// requireGuardianUser resolves the user against iam and asserts it may act as a
// guardian: it holds the guardian role AND its account is active. Only iam's
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
	if user.Status != activeUserStatus {
		return glerrors.New(glerrors.KindInvalid, "guardian account is not active")
	}
	return nil
}

// UnlinkGuardian removes a guardian ↔ student link. An unknown link is
// KindNotFound.
func (s *GuardianService) UnlinkGuardian(ctx context.Context, guardianUserID, studentID string) error {
	return s.guardians.Unlink(ctx, guardianUserID, studentID)
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
