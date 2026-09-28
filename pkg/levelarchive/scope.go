package levelarchive

import (
	"regexp"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Scope is everything one level deletion reaches, as the gateway resolved it:
// the level, its structure (from config) and its people (from students). Each
// service reads only the ids it owns data for.
type Scope struct {
	LevelID       string
	GradeLevelIDs []string
	GroupIDs      []string
	SubjectIDs    []string
	TermIDs       []string
	EnrollmentIDs []string
	StudentIDs    []string
}

var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// canonicalID lowercases and trims id and checks its uuid shape. The archive
// compares ids as text in places (JSON payloads), so "ABC…" and "abc…" must be
// one id before anything is compared.
func canonicalID(id string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(id))
	return c, uuidShape.MatchString(c)
}

// NormalizeBatchID canonicalizes a batch id, refusing anything but a uuid.
func NormalizeBatchID(id string) (string, error) { return NormalizeID(id, "batch id") }

// NormalizeID canonicalizes any id of the deletion (what names it in the
// refusal), refusing anything but a uuid with KindInvalid.
func NormalizeID(id, what string) (string, error) {
	c, ok := canonicalID(id)
	if !ok {
		return "", glerrors.New(glerrors.KindInvalid, what+" must be a uuid")
	}
	return c, nil
}

// Normalize canonicalizes every id, drops duplicates and refuses a missing
// level or any malformed id (KindInvalid).
func (s Scope) Normalize() (Scope, error) {
	level, ok := canonicalID(s.LevelID)
	if !ok {
		return Scope{}, glerrors.New(glerrors.KindInvalid, "level id must be a uuid")
	}
	out := Scope{LevelID: level}
	for _, f := range []struct {
		in  []string
		out *[]string
	}{
		{s.GradeLevelIDs, &out.GradeLevelIDs},
		{s.GroupIDs, &out.GroupIDs},
		{s.SubjectIDs, &out.SubjectIDs},
		{s.TermIDs, &out.TermIDs},
		{s.EnrollmentIDs, &out.EnrollmentIDs},
		{s.StudentIDs, &out.StudentIDs},
	} {
		ids, err := canonicalIDs(f.in)
		if err != nil {
			return Scope{}, err
		}
		*f.out = ids
	}
	return out, nil
}

func canonicalIDs(in []string) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
		c, ok := canonicalID(id)
		if !ok {
			return nil, glerrors.New(glerrors.KindInvalid, "scope ids must be uuids")
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out, nil
}
