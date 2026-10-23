package usecase

import (
	"context"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// syncFamilyAccess is the last step of every account or membership write on a
// family (ADR 0041), inside the write's own transaction: it reads the members
// as they now are, refuses a family past domain.MaxFamilyAccounts
// (domain.ErrFamilyAccountLimit, which rolls the whole write back) and links
// every account to every placeable member. A pupil without a family has
// neither cap nor siblings: nothing to do.
func syncFamilyAccess(ctx context.Context, a ports.FamilyAccess, tenant string, newID func() string) error {
	if a.FamilyID() == "" {
		return nil
	}
	members, err := a.Members(ctx)
	if err != nil {
		return err
	}
	plan, err := domain.PlanFamilyAccess(tenant, members, newID)
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

// syncAccess is syncFamilyAccess bound to this service, the hook CreateFamily
// and AddStudentsToFamily hand the repository.
func (s *FamilyService) syncAccess(ctx context.Context, a ports.FamilyAccess) error {
	return syncFamilyAccess(ctx, a, s.tenant, s.newID)
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
