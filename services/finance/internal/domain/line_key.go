package domain

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// LineKey is the identity of a billed line's NAME: the letters and digits in
// it, lower-cased, with the accents taken off ("Inscripción", "INSCRIPCION" and
// "inscripcion" are one key, "inscripcion").
//
// It exists because a person re-spells what they typed, and the charge a family
// already owes must not change identity when they do. Slugify cannot be that
// identity: it DROPS every non-ASCII letter, so "Inscripción" becomes
// "inscripcin" while the same word typed without its accent becomes
// "inscripcion" — two references for one line, and the billing run issued the
// second next to the first (T1, review 2026-09-30).
//
// Slugify still builds the references already issued, and must keep doing so:
// changing it would re-mint every reference in the ledger. LineKey is what
// decides whether a line has ALREADY been issued, whatever its reference says.
func LineKey(name string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		if unicode.Is(unicode.Mn, r) {
			// The accent a decomposed "ó" carries, never a letter of its own.
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SharedLineName reports two lines of the plan that would bill as ONE: two
// inscription concepts whose names reduce to the same LineKey, or two fees that
// do and share a month (a fee's month is part of its identity, so the same
// name in different months is two lines).
//
// The billing run recognises an issued line by that identity, so the second of
// such a pair would be taken for the first and never issued — under-billing the
// family all year without a word. Refusing it when the table is saved is the
// only moment a person is there to rename it.
//
// Deliberately NOT part of NewTuitionPlan: that constructor also rebuilds
// every plan already stored, and a rule added after the fact must not make a
// saved table unreadable — the billing run would stop for the whole level.
// The write path calls it; the read path does not.
func (p TuitionPlan) SharedLineName() error {
	seen := make(map[string]bool, len(p.enrollmentConcepts))
	for _, c := range p.enrollmentConcepts {
		k := LineKey(c.Name)
		if seen[k] {
			return glerrors.New(glerrors.KindInvalid,
				"two lines of the inscription differ only in accents, case or punctuation")
		}
		seen[k] = true
	}
	feeMonths := map[string]bool{}
	for _, f := range p.fees {
		k := LineKey(f.Name)
		for _, m := range uniqueMonths(f.Months) {
			id := fmt.Sprintf("%s|%d", k, m)
			if feeMonths[id] {
				return glerrors.New(glerrors.KindInvalid,
					"two fees collected in the same month differ only in accents, case or punctuation")
			}
			feeMonths[id] = true
		}
	}
	return nil
}

// uniqueMonths drops a month listed twice within ONE fee: that is the fee's
// own shape (validated elsewhere), not a clash with another fee.
func uniqueMonths(months []int) []int {
	seen := make(map[int]bool, len(months))
	out := make([]int, 0, len(months))
	for _, m := range months {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}
