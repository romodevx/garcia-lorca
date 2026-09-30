package usecase

import (
	"sort"
	"strings"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// issuedLines is what the billing run has already issued in a cycle, indexed by
// what each line IS rather than by the reference it happens to carry.
//
// Why it exists (T1, review 2026-09-30): a line's reference is built from the
// plan's shape — a single amount is `enrollment:<cycle>:<student>`, a concept is
// `enrollment:<cycle>:<student>:<slug>` — and from Slugify of its name, which
// drops accented letters. "Already issued" was an exact match on that
// reference, so converting a plan to concepts, or fixing the accent of a
// concept, minted a second reference for a line the family already owed, and
// ON CONFLICT had nothing to stop. The old unpaid charge stayed and the new one
// joined it: "3 conceptos y uno aparece dos veces".
//
// The references already in the ledger are not rewritten. The run adopts them:
// a line it wants to issue whose identity is already in the ledger is issued
// under the EXISTING reference, so the insert is a no-op and the re-dating and
// re-pricing reach the charge the family actually holds.
type issuedLines map[string][]string

// newIssuedLines indexes the lines already issued.
func newIssuedLines(lines []ports.IssuedLine) issuedLines {
	out := issuedLines{}
	for _, l := range lines {
		id, ok := lineIdentity(l.StudentID, l.ExternalRef, l.Description)
		if !ok {
			continue
		}
		out[id] = append(out[id], l.ExternalRef)
	}
	for _, refs := range out {
		// Deterministic choice when a ledger already holds the same line twice:
		// the same reference on every run (see adopt for which one wins).
		sort.Strings(refs)
	}
	return out
}

// adopt returns the charge to issue for a wanted line: the line itself when the
// ledger does not have it (or has it under the same reference), or the line
// carrying the reference it was ALREADY issued under.
//
// priceable says whether the run may re-price the charge by this line's ladder.
// It is false when the line was issued in the other shape: a single-amount
// inscription is the price of the WHOLE old inscription, not a rung of one
// concept's ladder, and moving it by that ladder is a decision about money
// nobody made.
func (ix issuedLines) adopt(c domain.Charge) (adopted domain.Charge, priceable bool, err error) {
	ref := c.ExternalRef()
	id, ok := lineIdentity(c.StudentID(), ref, c.Description())
	if !ok {
		return c, true, nil
	}
	refs := ix[id]
	if len(refs) == 0 {
		return c, true, nil
	}
	chosen := refs[0]
	for _, existing := range refs {
		if existing == ref {
			return c, true, nil
		}
	}
	for _, existing := range refs {
		// A ledger that already holds the line in BOTH shapes (the defect this
		// fixes, left in place) must keep answering to the charge of the
		// wanted line's own shape: that is the one its ladder re-prices.
		if sameShape(ref, existing) {
			chosen = existing
			break
		}
	}
	adopted, err = c.WithExternalRef(chosen)
	if err != nil {
		return domain.Charge{}, false, err
	}
	return adopted, sameShape(ref, chosen), nil
}

// adoptAll applies adopt to every line, returning the lines to issue and the
// subset the run may re-price.
func (ix issuedLines) adoptAll(lines []domain.Charge) (issue, priceable []domain.Charge, err error) {
	issue = make([]domain.Charge, 0, len(lines))
	priceable = make([]domain.Charge, 0, len(lines))
	for _, line := range lines {
		adopted, ok, err := ix.adopt(line)
		if err != nil {
			return nil, nil, err
		}
		issue = append(issue, adopted)
		if ok {
			priceable = append(priceable, adopted)
		}
	}
	return issue, priceable, nil
}

// lineIdentity is what a generated line IS: whose, which kind, which name (as
// LineKey reads it) and, for a fee, which month. Reports false for a reference
// the run does not build from a name.
//
// The name is read from the DESCRIPTION, not from the reference, because the
// slug in an old reference has already lost its accented letters and cannot be
// compared with anything. The description is what enrollmentCharges and
// feeDescription wrote, and for a single-amount inscription it is the catalog
// name "Inscripción" — which is exactly the concept a converted plan seeds.
func lineIdentity(studentID, ref, description string) (string, bool) {
	switch {
	case strings.HasPrefix(ref, domain.EnrollmentRefPrefix):
		return studentID + "|enrollment|" + domain.LineKey(description), true
	case strings.HasPrefix(ref, domain.FeeRefPrefix):
		month := ref[strings.LastIndex(ref, ":")+1:]
		return studentID + "|fee|" + domain.LineKey(feeNameOf(description)) + "|" + month, true
	}
	return "", false
}

// sameShape reports whether two inscription references were minted by the same
// kind of plan: both from a concept, or both from a single amount.
func sameShape(a, b string) bool {
	return isConceptRef(a) == isConceptRef(b)
}

// conceptRefSeparators is how many colons an inscription minted from a concept
// carries: `enrollment:<cycle>:<student>:<slug>`.
const conceptRefSeparators = 3

// isConceptRef reports whether a reference is an inscription line minted from a
// concept, as opposed to the single-amount `enrollment:<cycle>:<student>`.
// Cycle and student ids are UUIDs, so the colons counted are the separators.
func isConceptRef(ref string) bool {
	return strings.HasPrefix(ref, domain.EnrollmentRefPrefix) && strings.Count(ref, ":") == conceptRefSeparators
}
