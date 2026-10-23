package domain

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// A family groups siblings the office confirmed (ADR 0041). A pupil belongs to
// at most one; students.family_id is that membership and the repository owns
// it. The family does NOT decide which child an app account sees: that stays
// guardian_students (ADR 0004).

const (
	// MaxFamilyNameRunes bounds the name, as the families CHECK does.
	MaxFamilyNameRunes = 120
	// MaxFamilyPupils is the most pupils one family holds. More than that is
	// not a family: it is a shared value (a school phone) chaining strangers.
	MaxFamilyPupils = 20
)

// Family is a confirmed group of siblings.
type Family struct {
	id         string
	tenantID   string
	name       string
	studentIDs []string
	// accountCount is how many distinct app accounts are linked to the
	// members, as read. A read-side count, never written through the entity.
	accountCount int
	createdAt    time.Time
}

// FamilyParams carries a family read back from storage.
type FamilyParams struct {
	ID           string
	TenantID     string
	Name         string
	StudentIDs   []string
	AccountCount int
	CreatedAt    time.Time
}

// NewFamily builds a family the office is confirming: a name of 1..120 runes
// (trimmed) and 1..20 distinct pupils.
func NewFamily(id, tenantID, name string, studentIDs []string, createdAt time.Time) (Family, error) {
	if strings.TrimSpace(id) == "" {
		return Family{}, glerrors.New(glerrors.KindInvalid, "family id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Family{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	checked, err := checkFamilyName(name)
	if err != nil {
		return Family{}, err
	}
	if err := checkMembers(studentIDs); err != nil {
		return Family{}, err
	}
	return Family{
		id: id, tenantID: tenantID, name: checked,
		studentIDs: slices.Clone(studentIDs), createdAt: createdAt,
	}, nil
}

// RestoreFamily rebuilds a family from storage without re-judging it: the
// row was valid when written, and a family emptied by a concurrent write must
// still be readable.
func RestoreFamily(p FamilyParams) Family {
	return Family{
		id: p.ID, tenantID: p.TenantID, name: p.Name,
		studentIDs: slices.Clone(p.StudentIDs), accountCount: p.AccountCount, createdAt: p.CreatedAt,
	}
}

// Rename returns the family under a new name, judged as on creation.
func (f Family) Rename(name string) (Family, error) {
	checked, err := checkFamilyName(name)
	if err != nil {
		return Family{}, err
	}
	f.name = checked
	f.studentIDs = slices.Clone(f.studentIDs)
	return f, nil
}

// AddStudents returns the family with more pupils. A pupil already a member is
// KindConflict; nobody to add, a repeated id or growing past MaxFamilyPupils is
// KindInvalid.
func (f Family) AddStudents(studentIDs []string) (Family, error) {
	if len(studentIDs) == 0 {
		return Family{}, glerrors.New(glerrors.KindInvalid, "at least one pupil is required")
	}
	for _, id := range studentIDs {
		if slices.Contains(f.studentIDs, id) {
			return Family{}, glerrors.New(glerrors.KindConflict, "the pupil is already in this family")
		}
	}
	grown := append(slices.Clone(f.studentIDs), studentIDs...)
	if err := checkMembers(grown); err != nil {
		return Family{}, err
	}
	f.studentIDs = grown
	return f, nil
}

// Accessors.
func (f Family) ID() string           { return f.id }
func (f Family) TenantID() string     { return f.tenantID }
func (f Family) Name() string         { return f.name }
func (f Family) StudentIDs() []string { return slices.Clone(f.studentIDs) }
func (f Family) AccountCount() int    { return f.accountCount }
func (f Family) CreatedAt() time.Time { return f.createdAt }

func checkFamilyName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if n := utf8.RuneCountInString(trimmed); n == 0 || n > MaxFamilyNameRunes {
		return "", glerrors.New(glerrors.KindInvalid, "a family name has 1 to 120 characters")
	}
	return trimmed, nil
}

func checkMembers(studentIDs []string) error {
	if len(studentIDs) == 0 || len(studentIDs) > MaxFamilyPupils {
		return glerrors.New(glerrors.KindInvalid, "a family has 1 to 20 pupils")
	}
	seen := make(map[string]bool, len(studentIDs))
	for _, id := range studentIDs {
		if strings.TrimSpace(id) == "" {
			return glerrors.New(glerrors.KindInvalid, "student id is required")
		}
		if seen[id] {
			return glerrors.New(glerrors.KindInvalid, "a pupil is listed twice")
		}
		seen[id] = true
	}
	return nil
}

// ProposedFamilyName is how a family is called when the office names none:
// the pupil's surnames as stored ("García López").
func ProposedFamilyName(lastName, maternalLastName string) string {
	return strings.TrimSpace(strings.TrimSpace(lastName) + " " + strings.TrimSpace(maternalLastName))
}

// surnameFold uppercases-then-strips the Spanish accents, the diaeresis and the
// eñe, so "Peña" and "PENA" key alike; a hyphen ("Pérez-Gil") is a space.
var surnameFold = strings.NewReplacer(
	"Á", "A", "À", "A", "É", "E", "È", "E", "Í", "I", "Ì", "I",
	"Ó", "O", "Ò", "O", "Ú", "U", "Ù", "U", "Ü", "U", "Ñ", "N",
	"-", " ",
)

// foldName is a name in the one spelling two captures of it share: upper case,
// no accents, no hyphens, single spaces.
func foldName(s string) string {
	return strings.Join(strings.Fields(surnameFold.Replace(strings.ToUpper(s))), " ")
}

// minSurnameTokens: one surname alone ("García", "de la Cruz") is shared by
// too many unrelated families to suggest anything.
const minSurnameTokens = 2

// surnameParticles join or introduce surnames without being one: they stay in
// the key but do not count towards minSurnameTokens.
var surnameParticles = map[string]bool{"DE": true, "LA": true, "DEL": true, "LOS": true, "LAS": true, "Y": true}

// SurnameKey is the comparison key of a pupil's surnames: both joined, folded
// by foldName. A legacy pupil carrying both in lastName ("GARCÍA LÓPEZ", "")
// keys like a captured one ("Garcia", "Lopez"). Empty when fewer than two
// tokens are surnames rather than particles.
func SurnameKey(lastName, maternalLastName string) string {
	key := foldName(lastName + " " + maternalLastName)
	significant := 0
	for _, token := range strings.Fields(key) {
		if !surnameParticles[token] {
			significant++
		}
	}
	if significant < minSurnameTokens {
		return ""
	}
	return key
}

// SuggestionSignature identifies exactly one group of pupils: the SHA-256 (hex)
// of the sorted, deduplicated ids joined with ",". A new sibling changes it.
func SuggestionSignature(studentIDs []string) string {
	ids := slices.Clone(studentIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	return hex.EncodeToString(sum[:])
}

// SuggestionReason is why a group of pupils looks like one family. The values
// order the reasons of a suggestion.
type SuggestionReason uint8

const (
	// ReasonUnspecified is the zero value.
	ReasonUnspecified SuggestionReason = iota
	// ReasonSameSurnames: same paternal + maternal surname (SurnameKey).
	ReasonSameSurnames
	// ReasonSharedContactEmail: a contact of each carries the same email.
	ReasonSharedContactEmail
	// ReasonSharedContactPhone: a contact of each carries the same phone.
	ReasonSharedContactPhone
	// ReasonSharedAccount: the same app account is linked to each.
	ReasonSharedAccount
)

// SuggestionConfidence: a contact in common is high, surnames alone medium.
type SuggestionConfidence uint8

const (
	// ConfidenceUnspecified is the zero value.
	ConfidenceUnspecified SuggestionConfidence = iota
	// ConfidenceMedium: surnames only (cousins share surnames too).
	ConfidenceMedium
	// ConfidenceHigh: a contact or an account in common.
	ConfidenceHigh
)

// SuggestionPupil is a pupil without a family, as SuggestFamilies needs it.
// The slice handed in is in display order (surnames, then first name).
type SuggestionPupil struct {
	ID               string
	LastName         string
	MaternalLastName string
}

// SuggestionContact is one contact of a pupil: any of its values may be empty.
type SuggestionContact struct {
	StudentID      string
	Email          string
	Phone          string
	GuardianUserID string
}

// FamilySuggestion is a group of pupils that look like siblings.
type FamilySuggestion struct {
	Signature    string
	ProposedName string
	// StudentIDs is in display order.
	StudentIDs []string
	Reasons    []SuggestionReason
	Confidence SuggestionConfidence
}

const (
	// MaxPupilsPerSharedContact: a contact value on more pupils than this is
	// not a parent (a reception desk number, the school's own email). It links
	// nobody: such values are dropped before any union, so they cannot chain
	// the school together.
	MaxPupilsPerSharedContact = 6
	// MinPhoneDigits: shorter numbers are extensions or typos, not a signal.
	MinPhoneDigits = 10
)

// CommonContactValues are the contact values more than
// MaxPupilsPerSharedContact pupils still at school carry, counted over the
// WHOLE school — pupils already in a family included, or confirming families
// would shrink a reception number's count below the cap and let it chain the
// rest. Keys are ContactEmailKey, ContactPhoneKey and the account id.
type CommonContactValues struct {
	Emails   map[string]bool
	Phones   map[string]bool
	Accounts map[string]bool
}

func (c CommonContactValues) has(reason SuggestionReason, value string) bool {
	switch reason {
	case ReasonSharedContactEmail:
		return c.Emails[value]
	case ReasonSharedContactPhone:
		return c.Phones[value]
	case ReasonSharedAccount:
		return c.Accounts[value]
	default:
		return false
	}
}

// ContactEmailKey is the comparison key of a contact email: trimmed, lower case.
func ContactEmailKey(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// SuggestFamilies proposes groups of pupils without a family that look like
// siblings. Pure: same inputs, same answer, in the same order.
//
// Union-find over the pupils: an edge joins two pupils with the same non-empty
// SurnameKey, the same contact email (ContactEmailKey), the same contact phone
// (ContactPhoneKey) or the same guardian account. Contact values in common, or
// on more than MaxPupilsPerSharedContact of these pupils, link nobody. A
// component of 2 to MaxFamilyPupils pupils is a suggestion unless its
// signature is in dismissed. A larger component (a common surname pair, say
// HERNÁNDEZ HERNÁNDEZ ×25) is not a family: it is united again by contact and
// account edges alone, and the 2..MaxFamilyPupils components of that are
// suggested. Contacts of pupils not in `pupils` are ignored.
//
// Complexity: O((n+c)·α(n)) for n pupils and c contacts (path compression and
// union by size), plus O(n+c) more per oversized component (at most n/21,
// in practice one or two) and O(k log k) to sort k suggestions.
func SuggestFamilies(
	pupils []SuggestionPupil, contacts []SuggestionContact, dismissed map[string]bool, common CommonContactValues,
) []FamilySuggestion {
	index := make(map[string]int, len(pupils))
	everyone := make([]int, len(pupils))
	for i, p := range pupils {
		index[p.ID] = i
		everyone[i] = i
	}
	buckets := suggestionBuckets(pupils, contacts, index, common)

	var out []FamilySuggestion
	for _, c := range unite(everyone, buckets, anyReason) {
		for _, family := range familySized(c, buckets) {
			s := buildSuggestion(pupils, family.members, family.reasons)
			if !dismissed[s.Signature] {
				out = append(out, s)
			}
		}
	}
	slices.SortFunc(out, compareSuggestions)
	return out
}

// component is a group of pupils (indices, ascending: display order) and the
// reasons that hold it together.
type component struct {
	members []int
	reasons map[SuggestionReason]bool
}

func anyReason(SuggestionReason) bool { return true }

// contactReason is every edge but surnames: what still means "one family"
// inside a crowd that only shares a common surname pair.
func contactReason(r SuggestionReason) bool { return r != ReasonSameSurnames }

// familySized returns c when it can be a family (2..MaxFamilyPupils), the
// contact-held families inside it when it is larger, nothing when smaller.
func familySized(c component, buckets []bucket) []component {
	switch {
	case len(c.members) < 2:
		return nil
	case len(c.members) <= MaxFamilyPupils:
		return []component{c}
	}
	var out []component
	for _, sub := range unite(c.members, buckets, contactReason) {
		if len(sub.members) >= 2 && len(sub.members) <= MaxFamilyPupils {
			out = append(out, sub)
		}
	}
	return out
}

// unite runs union-find over universe (ascending pupil indices) with the edges
// of the buckets whose reason links, and returns the components in display
// order of their first member. A reason is kept on a component when two of its
// members share that bucket — linking or not, so a family split out of a
// surname crowd still says it shares the surnames.
func unite(universe []int, buckets []bucket, links func(SuggestionReason) bool) []component {
	pos := make(map[int]int, len(universe))
	for p, i := range universe {
		pos[i] = p
	}
	sets := newDisjointSets(len(universe))
	for _, b := range buckets {
		if !links(b.reason) {
			continue
		}
		first := -1
		for _, m := range b.members {
			p, ok := pos[m]
			switch {
			case !ok:
			case first < 0:
				first = p
			default:
				sets.union(first, p)
			}
		}
	}

	reasons := map[int]map[SuggestionReason]bool{}
	for _, b := range buckets {
		perRoot := map[int]int{}
		for _, m := range b.members {
			if p, ok := pos[m]; ok {
				perRoot[sets.find(p)]++
			}
		}
		for root, n := range perRoot {
			if n < 2 {
				continue
			}
			if reasons[root] == nil {
				reasons[root] = map[SuggestionReason]bool{}
			}
			reasons[root][b.reason] = true
		}
	}

	byRoot := map[int]int{} // root → position in out
	var out []component
	for p, i := range universe {
		root := sets.find(p)
		at, ok := byRoot[root]
		if !ok {
			at = len(out)
			byRoot[root] = at
			out = append(out, component{reasons: reasons[root]})
		}
		out[at].members = append(out[at].members, i)
	}
	return out
}

// bucket is the pupils (by index, ascending, distinct) that share one value.
type bucket struct {
	reason  SuggestionReason
	members []int
}

// suggestionBuckets groups the pupils by each shared value, keeping only the
// values two or more pupils share — and, for contact values, neither a common
// one nor one on more than MaxPupilsPerSharedContact of these pupils.
func suggestionBuckets(pupils []SuggestionPupil, contacts []SuggestionContact, index map[string]int, common CommonContactValues) []bucket {
	type key struct {
		reason SuggestionReason
		value  string
	}
	type membership struct {
		key
		pupil int
	}
	members := map[key][]int{}
	seen := map[membership]bool{}
	add := func(k key, i int) {
		if k.value == "" || seen[membership{k, i}] || common.has(k.reason, k.value) {
			return
		}
		seen[membership{k, i}] = true
		members[k] = append(members[k], i)
	}
	for i, p := range pupils {
		add(key{ReasonSameSurnames, SurnameKey(p.LastName, p.MaternalLastName)}, i)
	}
	for _, c := range contacts {
		i, ok := index[c.StudentID]
		if !ok {
			continue
		}
		add(key{ReasonSharedContactEmail, ContactEmailKey(c.Email)}, i)
		add(key{ReasonSharedContactPhone, ContactPhoneKey(c.Phone)}, i)
		add(key{ReasonSharedAccount, strings.TrimSpace(c.GuardianUserID)}, i)
	}
	var out []bucket
	for k, m := range members {
		if len(m) < 2 {
			continue
		}
		if k.reason != ReasonSameSurnames && len(m) > MaxPupilsPerSharedContact {
			continue
		}
		slices.Sort(m)
		out = append(out, bucket{reason: k.reason, members: m})
	}
	return out
}

// ContactPhoneKey is the comparison key of a contact phone: its digits, or ""
// when fewer than MinPhoneDigits (too short to mean anything).
func ContactPhoneKey(phone string) string {
	digits := onlyDigits(phone)
	if len(digits) < MinPhoneDigits {
		return ""
	}
	return digits
}

func buildSuggestion(pupils []SuggestionPupil, members []int, reasonSet map[SuggestionReason]bool) FamilySuggestion {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = pupils[m].ID
	}
	reasons := make([]SuggestionReason, 0, len(reasonSet))
	confidence := ConfidenceMedium
	for r := range reasonSet {
		reasons = append(reasons, r)
		if r != ReasonSameSurnames {
			confidence = ConfidenceHigh
		}
	}
	slices.Sort(reasons)
	first := pupils[members[0]]
	return FamilySuggestion{
		Signature:    SuggestionSignature(ids),
		ProposedName: ProposedFamilyName(first.LastName, first.MaternalLastName),
		StudentIDs:   ids,
		Reasons:      reasons,
		Confidence:   confidence,
	}
}

// compareSuggestions: HIGH first, then by proposed name (accent- and
// case-insensitive, then as written), then by signature, which is unique.
func compareSuggestions(a, b FamilySuggestion) int {
	return cmp.Or(
		cmp.Compare(b.Confidence, a.Confidence),
		cmp.Compare(foldName(a.ProposedName), foldName(b.ProposedName)),
		cmp.Compare(a.ProposedName, b.ProposedName),
		cmp.Compare(a.Signature, b.Signature),
	)
}

// disjointSets is union-find with path compression and union by size.
type disjointSets struct {
	parent []int
	size   []int
}

func newDisjointSets(n int) *disjointSets {
	d := &disjointSets{parent: make([]int, n), size: make([]int, n)}
	for i := range d.parent {
		d.parent[i] = i
		d.size[i] = 1
	}
	return d
}

func (d *disjointSets) find(x int) int {
	for d.parent[x] != x {
		d.parent[x] = d.parent[d.parent[x]] // path halving
		x = d.parent[x]
	}
	return x
}

func (d *disjointSets) union(a, b int) {
	ra, rb := d.find(a), d.find(b)
	if ra == rb {
		return
	}
	if d.size[ra] < d.size[rb] {
		ra, rb = rb, ra
	}
	d.parent[rb] = ra
	d.size[ra] += d.size[rb]
}
