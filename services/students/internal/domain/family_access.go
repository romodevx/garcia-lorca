package domain

import (
	"cmp"
	"slices"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Family access (ADR 0041, T2). guardian_students stays the authorization of
// the family app (ADR 0004); a family only decides that every account linked
// to one of its pupils is linked to every placeable sibling, and that it holds
// at most MaxFamilyAccounts distinct accounts.

// MaxFamilyAccounts is how many distinct app accounts one family may hold:
// the two parents.
const MaxFamilyAccounts = 2

// CodeFamilyAccountLimit is the X-Error-Code of ErrFamilyAccountLimit.
const CodeFamilyAccountLimit = "FAMILY_ACCOUNT_LIMIT"

// ErrFamilyAccountLimit refuses a write that would leave a family with more
// than MaxFamilyAccounts distinct accounts. Nothing of that write is stored.
var ErrFamilyAccountLimit = glerrors.New(glerrors.KindConflict,
	"una familia tiene como máximo dos cuentas de la app: quita el acceso de una antes de dar otro")

// FamilyMember is one pupil of a family as an account write sees it, read
// under the family's lock: its admission status and every contact it has.
type FamilyMember struct {
	StudentID string
	Status    AdmissionStatus
	Contacts  []GuardianLink
}

// AtSchool reports a pupil that has not left: neither withdrawn nor rejected.
func (m FamilyMember) AtSchool() bool {
	return m.Status != AdmissionWithdrawn && m.Status != AdmissionRejected
}

// receivesAccounts: only a pupil still at school whom the school can identify
// (not a draft) is handed an account.
func (m FamilyMember) receivesAccounts() bool { return m.AtSchool() && m.Status != AdmissionDraft }

// FamilyAccessPlan is what brings a family in sync: accounts to attach to
// account-less contacts the siblings already have, and contacts to insert.
type FamilyAccessPlan struct {
	Attach []GuardianLink
	Insert []GuardianLink
}

// PlanFamilyAccess checks the cap and plans the sync of a family, given its
// members as they are now (the write that triggered it already applied).
//
// Every distinct account on any member counts, withdrawn members included
// (they are the same parents); more than MaxFamilyAccounts is
// ErrFamilyAccountLimit. Then each account U is linked to every member that
// receives accounts and has no link with U: on the member's account-less
// contact with the same email as U's source contact when there is one, else on
// a new contact copying the source's relationship, name, phone and email
// (never the emergency mark, which is the office's call per pupil). U's source
// is its contact on the lowest-id member carrying it.
//
// Pure and deterministic: accounts in id order, then members in id order.
// O(m·c) for m members and c contacts per member (m ≤ 20, c small).
func PlanFamilyAccess(tenantID string, members []FamilyMember, newID func() string) (FamilyAccessPlan, error) {
	sorted := sortedMembers(members)
	sources := accountSources(sorted)
	if len(sources) > MaxFamilyAccounts {
		return FamilyAccessPlan{}, ErrFamilyAccountLimit
	}
	accounts := make([]string, 0, len(sources))
	for account := range sources {
		accounts = append(accounts, account)
	}
	slices.Sort(accounts)

	var plan FamilyAccessPlan
	taken := map[string]bool{} // contacts this plan already gives an account
	for _, account := range accounts {
		source := sources[account]
		for _, m := range sorted {
			if !m.receivesAccounts() || hasAccount(m, account) {
				continue
			}
			if c, ok := contactWithEmail(m, source.Email(), taken); ok {
				taken[c.ID()] = true
				withAccount, err := c.WithAccount(account)
				if err != nil {
					return FamilyAccessPlan{}, err
				}
				plan.Attach = append(plan.Attach, withAccount)
				continue
			}
			copied, err := NewGuardianLink(newID(), tenantID, account, m.StudentID, source.Relationship())
			if err != nil {
				return FamilyAccessPlan{}, err
			}
			plan.Insert = append(plan.Insert, copied.WithPerson(ContactPerson{
				FullName: source.FullName(), Phone: source.Phone(), Email: source.Email(),
			}))
		}
	}
	return plan, nil
}

// FamilyContactPlan is one adult recorded on a family: Contacts holds one
// contact per member still at school (the new one or the one that already
// carried this person), in member id order; Insert the new ones.
type FamilyContactPlan struct {
	Contacts []GuardianLink
	Insert   []GuardianLink
}

// PlanFamilyContact records person (no account) on every member still at
// school, drafts included. A member that already has a contact with the same
// email or the same phone keeps it and gets no second one. The person is
// judged as on AddContact even when nobody needs a new contact.
func PlanFamilyContact(tenantID string, members []FamilyMember, rel Relationship, person ContactPerson, newID func() string) (FamilyContactPlan, error) {
	checked, err := checkPerson(person, false)
	if err != nil {
		return FamilyContactPlan{}, err
	}
	if !rel.Valid() {
		return FamilyContactPlan{}, glerrors.New(glerrors.KindInvalid, "a valid relationship is required")
	}
	emailKey := ContactEmailKey(checked.Email)
	phone := onlyDigits(checked.Phone)

	var plan FamilyContactPlan
	for _, m := range sortedMembers(members) {
		if !m.AtSchool() {
			continue
		}
		if c, ok := samePerson(m, emailKey, phone); ok {
			plan.Contacts = append(plan.Contacts, c)
			continue
		}
		c, err := NewContact(newID(), tenantID, m.StudentID, rel, checked)
		if err != nil {
			return FamilyContactPlan{}, err
		}
		plan.Contacts = append(plan.Contacts, c)
		plan.Insert = append(plan.Insert, c)
	}
	return plan, nil
}

// sortedMembers clones members in student id order, each with its contacts in
// id order: the order every plan is decided in.
func sortedMembers(members []FamilyMember) []FamilyMember {
	out := slices.Clone(members)
	for i := range out {
		out[i].Contacts = slices.Clone(out[i].Contacts)
		slices.SortFunc(out[i].Contacts, func(a, b GuardianLink) int { return cmp.Compare(a.ID(), b.ID()) })
	}
	slices.SortFunc(out, func(a, b FamilyMember) int { return cmp.Compare(a.StudentID, b.StudentID) })
	return out
}

// accountSources maps each distinct account to its contact on the lowest-id
// member carrying it (members already sorted).
func accountSources(sorted []FamilyMember) map[string]GuardianLink {
	out := map[string]GuardianLink{}
	for _, m := range sorted {
		for _, c := range m.Contacts {
			if _, seen := out[c.GuardianUserID()]; c.HasAccount() && !seen {
				out[c.GuardianUserID()] = c
			}
		}
	}
	return out
}

func hasAccount(m FamilyMember, account string) bool {
	return slices.ContainsFunc(m.Contacts, func(c GuardianLink) bool { return c.GuardianUserID() == account })
}

// contactWithEmail is the member's first account-less contact, not already
// taken by this plan, whose email matches (case-insensitive, non-empty).
func contactWithEmail(m FamilyMember, email string, taken map[string]bool) (GuardianLink, bool) {
	key := ContactEmailKey(email)
	if key == "" {
		return GuardianLink{}, false
	}
	for _, c := range m.Contacts {
		if !c.HasAccount() && !taken[c.ID()] && ContactEmailKey(c.Email()) == key {
			return c, true
		}
	}
	return GuardianLink{}, false
}

// samePerson is the member's first contact with this email key or these phone
// digits (each only when non-empty).
func samePerson(m FamilyMember, emailKey, phone string) (GuardianLink, bool) {
	for _, c := range m.Contacts {
		if emailKey != "" && ContactEmailKey(c.Email()) == emailKey {
			return c, true
		}
		if phone != "" && onlyDigits(c.Phone()) == phone {
			return c, true
		}
	}
	return GuardianLink{}, false
}
