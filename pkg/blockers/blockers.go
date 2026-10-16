// Package blockers words, in Spanish, what keeps a record from being deleted:
// "2 grupos y 8 materias". A refusal that names what blocks it and how many is
// one a person can act on; "está en uso" is not.
package blockers

import (
	"strconv"
	"strings"
)

// Count is how many rows of one kind block a deletion, with the noun for one
// and for many ("grupo", "grupos").
type Count struct {
	N    int64
	One  string
	Many string
}

// phrase is "1 grupo" / "2 grupos".
func (c Count) phrase() string {
	noun := c.Many
	if c.N == 1 {
		noun = c.One
	}
	return strconv.FormatInt(c.N, 10) + " " + noun
}

// Describe joins the non-zero counts as a Spanish list ("a, b y c"), in the
// order given. It is "" when nothing blocks.
func Describe(counts ...Count) string {
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		if c.N > 0 {
			parts = append(parts, c.phrase())
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " y " + parts[len(parts)-1]
	}
}
