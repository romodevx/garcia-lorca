// Package studentname is the one place a pupil's name is put together and
// ordered. The school reads its lists surname first — "Pérez Núñez Ana Sofía" —
// and alphabetically by surname, in Spanish (accents and case do not change the
// order; ñ sorts after n). Every service that shows, prints or snapshots a
// pupil's name goes through here, so the lists, the receipts, the report cards
// and the grade sheets cannot disagree.
package studentname

import (
	"sort"
	"strings"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Parts are the pieces of a pupil's name as the file keeps them. Paternal may
// hold both surnames on a pupil captured before the maternal field existed;
// it is shown as-is.
type Parts struct {
	First    string
	Paternal string
	Maternal string
}

// Format renders "Paterno Materno Nombre(s)", skipping the empty pieces.
func Format(p Parts) string {
	pieces := make([]string, 0, 3)
	for _, s := range []string{p.Paternal, p.Maternal, p.First} {
		if t := strings.TrimSpace(s); t != "" {
			pieces = append(pieces, t)
		}
	}
	return strings.Join(pieces, " ")
}

// Sort orders items by paternal surname, then maternal, then first name, in
// Spanish collation that ignores accents and case. Items with the same name
// keep their order. A collator is not safe for concurrent use, so each call
// builds its own.
func Sort[T any](items []T, parts func(T) Parts) {
	col := collate.New(language.Spanish, collate.Loose)
	sort.SliceStable(items, func(i, j int) bool {
		return compare(col, parts(items[i]), parts(items[j])) < 0
	})
}

func compare(col *collate.Collator, a, b Parts) int {
	if c := col.CompareString(strings.TrimSpace(a.Paternal), strings.TrimSpace(b.Paternal)); c != 0 {
		return c
	}
	if c := col.CompareString(strings.TrimSpace(a.Maternal), strings.TrimSpace(b.Maternal)); c != 0 {
		return c
	}
	return col.CompareString(strings.TrimSpace(a.First), strings.TrimSpace(b.First))
}
