package legacyimport

import "strings"

// accentless maps the accented letters Spanish writes to their plain form. Ñ is
// kept: it is a letter of its own, and only the CURP rule turns it into X.
var accentless = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U",
)

// Fold removes accents, keeping case and spacing.
func Fold(s string) string { return accentless.Replace(s) }
