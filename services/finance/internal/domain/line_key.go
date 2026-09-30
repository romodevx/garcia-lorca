package domain

import (
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
// inscription concepts, or two fees, whose names reduce to the same LineKey.
//
// The billing run recognises an issued line by that key, so the second of such
// a pair would be taken for the first and never issued — under-billing the
// family all year without a word. Refusing it when the table is saved is the
// only moment a person is there to rename it.
//
// Deliberately NOT part of NewTuitionPlan: that constructor also rebuilds
// every plan already stored, and a rule added after the fact must not make a
// saved table unreadable — the billing run would stop for the whole level.
// The write path calls it; the read path does not.
func (p TuitionPlan) SharedLineName() error {
	concepts := make([]string, 0, len(p.enrollmentConcepts))
	for _, c := range p.enrollmentConcepts {
		concepts = append(concepts, c.Name)
	}
	if repeatedKey(concepts) {
		return glerrors.New(glerrors.KindInvalid,
			"dos conceptos de la inscripción se llaman igual salvo por acentos o mayúsculas")
	}
	fees := make([]string, 0, len(p.fees))
	for _, f := range p.fees {
		fees = append(fees, f.Name)
	}
	if repeatedKey(fees) {
		return glerrors.New(glerrors.KindInvalid,
			"dos cuotas se llaman igual salvo por acentos o mayúsculas")
	}
	return nil
}

// repeatedKey reports whether two names reduce to one LineKey.
func repeatedKey(names []string) bool {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		k := LineKey(n)
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}
