package domain

import "strings"

// foldedLetters maps the Spanish accented letters and ñ, in either case, to
// their base letter — the translate() of config_fold_name.
var foldedLetters = map[rune]rune{
	'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u', 'ü': 'u', 'ñ': 'n',
	'Á': 'a', 'É': 'e', 'Í': 'i', 'Ó': 'o', 'Ú': 'u', 'Ü': 'u', 'Ñ': 'n',
}

// FoldName is the key two level names are compared by: "PREPARATORIA",
// " Preparatoria " and "Preparatória" are one level. It folds áéíóúüñ (either
// case) to the base letter, lower-cases ASCII, collapses runs of ASCII
// whitespace to one space and trims the ends.
//
// It is the exact twin of the SQL config_fold_name (migrations/config/0019),
// which backs the unique index the import's find-or-create relies on: the two
// must agree on every input, so neither touches anything outside that set (a
// "À" or a no-break space stays as it is in both).
func FoldName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	pendingSpace := false
	for _, r := range name {
		if isASCIISpace(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

// CleanName tidies a name for storage: runs of ASCII whitespace become one
// space and the ends are trimmed, case and accents kept. It uses FoldName's
// notion of whitespace, so FoldName(CleanName(s)) == FoldName(s).
func CleanName(name string) string {
	return strings.Join(strings.FieldsFunc(name, isASCIISpace), " ")
}

func foldRune(r rune) rune {
	if base, ok := foldedLetters[r]; ok {
		return base
	}
	if 'A' <= r && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// isASCIISpace is the C locale's isspace — what the SQL \s matches under
// COLLATE "C".
func isASCIISpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}
