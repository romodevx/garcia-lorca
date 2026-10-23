package usecase

import (
	"context"
	"slices"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// familySync links a family's accounts to every placeable member (ADR 0041).
// iam is asked again BEFORE the locked step: an account suspended, deleted or
// stripped of the guardian role since it was linked still counts toward the
// cap but is never handed to another pupil — a sync must not do what
// LinkGuardian would refuse.
type familySync struct {
	access ports.FamilyAccessRepository
	users  ports.UserReader
	tenant string
	newID  func() string
}

// eligible resolves through iam every account already on the pupils, on
// familyID's members and on the pupils' families (an unlocked read: one iam
// call per distinct account, at most MaxFamilyAccounts + 1 in practice).
// vetted accounts were just validated by the caller and are not asked again.
// An account iam does not know is ineligible; an iam failure fails the write.
// An account that reaches the family only after this read is ineligible too:
// the sync never propagates what nobody checked.
func (f familySync) eligible(ctx context.Context, familyID string, studentIDs []string, vetted ...string) (map[string]bool, error) {
	known, err := f.access.AccountIDs(ctx, familyID, studentIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(known)+len(vetted))
	for _, id := range vetted {
		out[id] = true
	}
	for _, id := range known {
		if _, seen := out[id]; seen {
			continue
		}
		ok, err := linkableAccount(ctx, f.users, id)
		if err != nil {
			return nil, err
		}
		out[id] = ok
	}
	return out, nil
}

// run is the last step of every account or membership write on a family,
// inside the write's own transaction: it reads the members as they now are,
// refuses a family past domain.MaxFamilyAccounts (domain.ErrFamilyAccountLimit,
// which rolls the whole write back) and links every eligible account to every
// placeable member. A pupil without a family has neither cap nor siblings.
func (f familySync) run(ctx context.Context, a ports.FamilyAccess, eligible map[string]bool) error {
	if a.FamilyID() == "" {
		return nil
	}
	members, err := a.Members(ctx)
	if err != nil {
		return err
	}
	plan, err := domain.PlanFamilyAccess(f.tenant, members, f.newID, func(account string) bool { return eligible[account] })
	if err != nil {
		return err
	}
	for _, l := range plan.Attach {
		if err := a.AttachAccount(ctx, l); err != nil {
			return err
		}
	}
	for _, l := range plan.Insert {
		if err := a.InsertContact(ctx, l); err != nil {
			return err
		}
	}
	return nil
}

// hook is run bound to eligible: what CreateFamily and AddStudentsToFamily
// hand the repository.
func (f familySync) hook(eligible map[string]bool) ports.FamilyAccessFunc {
	return func(ctx context.Context, a ports.FamilyAccess) error { return f.run(ctx, a, eligible) }
}

// linkableAccount asks iam whether an account may still be linked to a pupil:
// it holds the guardian role and is in a linkable status. Not found is false;
// any other failure is returned.
func linkableAccount(ctx context.Context, users ports.UserReader, id string) (bool, error) {
	user, err := users.ByID(ctx, id)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isLinkable(user), nil
}

func isLinkable(user ports.UserRef) bool {
	return slices.Contains(user.Roles, guardianRoleName) && slices.Contains(linkableUserStatuses, user.Status)
}

func (s *FamilyService) sync() familySync {
	return familySync{access: s.families, users: s.users, tenant: s.tenant, newID: s.newID}
}

// AddFamilyContact records one adult (no account) on every pupil of the family
// still at school, drafts included, under the family's lock. A pupil that
// already has a contact with the same email or phone keeps it. It returns one
// contact per such pupil, new or existing, in student id order. The person is
// judged as in AddContact; an unknown family is KindNotFound.
func (s *FamilyService) AddFamilyContact(ctx context.Context, familyID, relationship string, person domain.ContactPerson) ([]domain.GuardianLink, error) {
	id, err := canonicalID(familyID, "family id")
	if err != nil {
		return nil, err
	}
	rel, err := domain.ParseRelationship(relationship)
	if err != nil {
		return nil, err
	}
	// Judged before any I/O: a bad person is the caller's error even for an
	// unknown family.
	if _, err := domain.PlanFamilyContact(s.tenant, nil, rel, person, s.newID); err != nil {
		return nil, err
	}
	var out []domain.GuardianLink
	err = s.families.OnFamily(ctx, id, func(ctx context.Context, a ports.FamilyAccess) error {
		members, err := a.Members(ctx)
		if err != nil {
			return err
		}
		plan, err := domain.PlanFamilyContact(s.tenant, members, rel, person, s.newID)
		if err != nil {
			return err
		}
		for _, c := range plan.Insert {
			if err := a.InsertContact(ctx, c); err != nil {
				return err
			}
		}
		out = plan.Contacts
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeFamilyAccess takes an account off every pupil of the family, under the
// family's lock: the contacts stay without it (a nameless legacy link is
// deleted). It answers how many contacts lost the account; none is
// KindNotFound. The account's links to pupils outside the family stay.
func (s *FamilyService) RevokeFamilyAccess(ctx context.Context, familyID, guardianUserID string) (int, error) {
	fid, err := canonicalID(familyID, "family id")
	if err != nil {
		return 0, err
	}
	uid, err := canonicalID(guardianUserID, "guardian user id")
	if err != nil {
		return 0, err
	}
	var revoked int
	err = s.families.OnFamily(ctx, fid, func(ctx context.Context, a ports.FamilyAccess) error {
		n, err := a.DetachAccount(ctx, uid)
		if err != nil {
			return err
		}
		if n == 0 {
			return glerrors.ErrNotFound
		}
		revoked = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	return revoked, nil
}
