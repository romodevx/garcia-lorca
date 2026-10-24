package domain

// Families from an import (PLAN-familias-importacion.md, ADR 0041 amendment).
// When the office confirms an import, the families that are VERY likely are
// built without asking; the doubtful ones stay suggestions. "Very likely" is
// stricter than a suggestion: the same two surnames AND a contact value in
// common, pair by pair, so a cousin who shares the surnames but not the phone
// stays out.

// ImportPupil is a pupil still at school as PlanImportFamilies needs it. The
// slice handed in is in display order (surnames, then first name).
type ImportPupil struct {
	ID               string
	LastName         string
	MaternalLastName string
	// FamilyID is the pupil's family, "" for none.
	FamilyID string
	// HasAccount: an app account is linked to the pupil itself.
	HasAccount bool
	// InBatch: the import being confirmed created this pupil.
	InBatch bool
}

// ImportFamilyState is an existing family as read: its members (any status)
// and the distinct app accounts linked to them.
type ImportFamilyState struct {
	Pupils   int
	Accounts int
}

// ImportFamilyInput is what PlanImportFamilies works on. Contacts are every
// email and phone of the pupils — their own file's and their contacts' — as
// SuggestionContact rows (GuardianUserID is not read here). Families is keyed
// by family id; a family missing from it is never joined. Common is counted
// over the whole school, as for suggestions.
type ImportFamilyInput struct {
	Pupils    []ImportPupil
	Contacts  []SuggestionContact
	Families  map[string]ImportFamilyState
	Dismissed map[string]bool
	Common    CommonContactValues
}

// ImportJoin adds family-less pupils to an existing family.
type ImportJoin struct {
	FamilyID   string
	StudentIDs []string
}

// ImportFamilyPlan is what an import builds: new families (each its pupils in
// display order), pupils joining an existing family, and how many groups were
// left to the office (they stay, or become, suggestions).
type ImportFamilyPlan struct {
	Create  [][]string
	Join    []ImportJoin
	Skipped int
}

// PlanImportFamilies decides which families an import builds. Pure: same
// inputs, same plan, in the same order.
//
// Union-find over the pupils: two pupils are united only when they share a
// non-empty SurnameKey AND a contact value (ContactEmailKey or
// ContactPhoneKey). A value in common across the school, or on more than
// MaxPupilsPerSharedContact distinct pupils here, links nobody. Every
// component of 2..MaxFamilyPupils pupils with at least one InBatch pupil is
// judged:
//   - nobody in a family → Create, unless a member has an app account
//     (building the family would hand it the siblings: security H1) or the
//     group's SuggestionSignature was dismissed → Skipped;
//   - the members touch exactly one family, known in Families, with no
//     account, no family-less member has one, and the total stays within
//     MaxFamilyPupils → Join with the family-less members (none left: a
//     re-run, nothing to do);
//   - anything else (two families, a family with accounts, too many pupils)
//     → Skipped.
//
// Components are in display order of their first pupil.
//
// Complexity: O((n+c)·α(n)) for n pupils and c contacts (path halving and
// union by size); the maps are O(n+c) memory.
func PlanImportFamilies(in ImportFamilyInput) ImportFamilyPlan {
	index := make(map[string]int, len(in.Pupils))
	surnames := make([]string, len(in.Pupils))
	for i, p := range in.Pupils {
		index[p.ID] = i
		surnames[i] = SurnameKey(p.LastName, p.MaternalLastName)
	}
	sets := newDisjointSets(len(in.Pupils))
	for _, holders := range importContactHolders(in.Contacts, index, in.Common) {
		uniteSameSurname(sets, holders, surnames)
	}

	var plan ImportFamilyPlan
	for _, members := range importComponents(sets, len(in.Pupils)) {
		pupils := make([]ImportPupil, len(members))
		for i, m := range members {
			pupils[i] = in.Pupils[m]
		}
		plan.judge(pupils, in)
	}
	return plan
}

// importContactHolders returns, per linking contact value, the distinct
// pupils (indices, first-seen order) holding it: neither common across the
// school nor on more than MaxPupilsPerSharedContact of these pupils.
func importContactHolders(contacts []SuggestionContact, index map[string]int, common CommonContactValues) [][]int {
	type key struct {
		reason SuggestionReason
		value  string
	}
	type membership struct {
		key
		pupil int
	}
	holders := map[key][]int{}
	var order []key
	seen := map[membership]bool{}
	add := func(k key, i int) {
		if k.value == "" || seen[membership{k, i}] || common.has(k.reason, k.value) {
			return
		}
		seen[membership{k, i}] = true
		if _, known := holders[k]; !known {
			order = append(order, k)
		}
		holders[k] = append(holders[k], i)
	}
	for _, c := range contacts {
		i, ok := index[c.StudentID]
		if !ok {
			continue
		}
		add(key{ReasonSharedContactEmail, ContactEmailKey(c.Email)}, i)
		add(key{ReasonSharedContactPhone, ContactPhoneKey(c.Phone)}, i)
	}
	out := make([][]int, 0, len(order))
	for _, k := range order {
		if h := holders[k]; len(h) >= 2 && len(h) <= MaxPupilsPerSharedContact {
			out = append(out, h)
		}
	}
	return out
}

// uniteSameSurname unites the holders of one contact value that also share a
// non-empty SurnameKey: both conditions, pair by pair.
func uniteSameSurname(sets *disjointSets, holders []int, surnames []string) {
	first := map[string]int{} // surname key → first holder with it
	for _, i := range holders {
		key := surnames[i]
		if key == "" {
			continue
		}
		if f, ok := first[key]; ok {
			sets.union(f, i)
			continue
		}
		first[key] = i
	}
}

// importComponents returns the components of two or more pupils (indices,
// ascending), in order of their first member.
func importComponents(sets *disjointSets, n int) [][]int {
	byRoot := map[int]int{}
	var all [][]int
	for i := range n {
		root := sets.find(i)
		at, ok := byRoot[root]
		if !ok {
			at = len(all)
			byRoot[root] = at
			all = append(all, nil)
		}
		all[at] = append(all[at], i)
	}
	out := all[:0]
	for _, c := range all {
		if len(c) >= 2 {
			out = append(out, c)
		}
	}
	return out
}

// judge adds one component's outcome to the plan (see PlanImportFamilies).
func (plan *ImportFamilyPlan) judge(members []ImportPupil, in ImportFamilyInput) {
	if !anyInBatch(members) {
		return
	}
	if len(members) > MaxFamilyPupils {
		plan.Skipped++
		return
	}
	families := map[string]bool{}
	var familyless []ImportPupil
	for _, p := range members {
		if p.FamilyID == "" {
			familyless = append(familyless, p)
			continue
		}
		families[p.FamilyID] = true
	}
	switch len(families) {
	case 0:
		plan.create(members, in.Dismissed)
	case 1:
		for familyID := range families {
			plan.join(familyID, familyless, in.Families)
		}
	default:
		plan.Skipped++
	}
}

func (plan *ImportFamilyPlan) create(members []ImportPupil, dismissed map[string]bool) {
	ids := importIDs(members)
	if anyAccount(members) || dismissed[SuggestionSignature(ids)] {
		plan.Skipped++
		return
	}
	plan.Create = append(plan.Create, ids)
}

func (plan *ImportFamilyPlan) join(familyID string, joiners []ImportPupil, families map[string]ImportFamilyState) {
	if len(joiners) == 0 {
		return // everyone is in already: a re-run
	}
	state, known := families[familyID]
	if !known || state.Accounts > 0 || anyAccount(joiners) || state.Pupils+len(joiners) > MaxFamilyPupils {
		plan.Skipped++
		return
	}
	plan.Join = append(plan.Join, ImportJoin{FamilyID: familyID, StudentIDs: importIDs(joiners)})
}

func anyInBatch(pupils []ImportPupil) bool {
	for _, p := range pupils {
		if p.InBatch {
			return true
		}
	}
	return false
}

func anyAccount(pupils []ImportPupil) bool {
	for _, p := range pupils {
		if p.HasAccount {
			return true
		}
	}
	return false
}

func importIDs(pupils []ImportPupil) []string {
	out := make([]string, len(pupils))
	for i, p := range pupils {
		out[i] = p.ID
	}
	return out
}
