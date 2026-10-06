package legacyimport

import (
	"errors"
	"strings"
)

// Name warnings, as stable codes the screen words.
const (
	// WarnSurnamesMismatch: the CURP fits no way of splitting the surnames;
	// the first legal split was used and the office should check it.
	WarnSurnamesMismatch = "surnames_curp_mismatch"
	// WarnSurnamesAmbiguous: more than one split fits the CURP.
	WarnSurnamesAmbiguous = "surnames_ambiguous"
	// WarnNameWithoutComma: "APELLIDOS, NOMBRES" came without its comma and
	// the order was inferred from the CURP.
	WarnNameWithoutComma = "name_without_comma"
	// WarnFirstNameMismatch: the given name's initial does not match the CURP.
	WarnFirstNameMismatch = "first_name_curp_mismatch"
)

// ErrNameUnreadable is a name with no surname or no given name to keep.
var ErrNameUnreadable = errors.New("legacyimport: the name has no surname or no given name")

// Name is a pupil's name split the way the students service stores it.
type Name struct {
	First, Paternal, Maternal string
	Warnings                  []string
}

// particles never stand alone as a surname: they bind to the word after them
// ("DE LA CRUZ") and the CURP skips them when it takes its letters.
var particles = map[string]bool{
	"DA": true, "DAS": true, "DE": true, "DEL": true, "DER": true, "DI": true, "DIE": true, "DD": true,
	"EL": true, "LA": true, "LAS": true, "LE": true, "LES": true, "LOS": true, "MAC": true, "MC": true,
	"VAN": true, "VON": true, "Y": true,
}

// compoundFirst are the given names the CURP skips when a second one follows
// (María José takes its letter from José… unless it is the second name too).
var compoundFirst = map[string]bool{"MARIA": true, "MA": true, "M": true, "JOSE": true, "J": true}

// SplitName separates "APELLIDOS, NOMBRES" into given name, paternal and
// maternal surname. A surname of several words cannot be split by spacing
// alone ("GARCIA DE LA TORRE LOPEZ" is two surnames, but which two?), so the
// split is checked against the CURP's first three letters — the paternal
// surname's initial and first inner vowel, and the maternal's initial — which
// were derived from the real surnames when the CURP was issued.
func SplitName(full, curp string) (Name, error) {
	key := curpKey(curp)
	before, after, hasComma := strings.Cut(full, ",")
	if !hasComma {
		return splitWithoutComma(strings.Fields(full), key)
	}
	surnames, names := strings.Fields(before), strings.Fields(after)
	if len(surnames) == 0 || len(names) == 0 {
		return Name{}, ErrNameUnreadable
	}
	n := placeSurnames(surnames, names, key)
	return n, nil
}

// placeSurnames picks the split of the surname words the CURP agrees with.
func placeSurnames(surnames, names []string, key string) Name {
	var fits []int
	firstLegal := 0
	for k := 1; k <= len(surnames); k++ {
		if particles[foldWord(surnames[k-1])] {
			continue // a particle binds to the next word; it cannot end a surname
		}
		if firstLegal == 0 {
			firstLegal = k
		}
		if key != "" && surnamesFit(surnames[:k], surnames[k:], key) {
			fits = append(fits, k)
		}
	}
	if firstLegal == 0 {
		firstLegal = len(surnames)
	}
	k, warnings := firstLegal, []string(nil)
	switch {
	case len(fits) == 1:
		k = fits[0]
	case len(fits) > 1:
		// Mexican compound surnames are paternal far more often than not
		// ("Espinosa de los Monteros Mendoza"): the longest paternal wins, and
		// the office is told to check.
		k, warnings = fits[len(fits)-1], []string{WarnSurnamesAmbiguous}
	default:
		warnings = []string{WarnSurnamesMismatch}
	}
	if key != "" && len(fits) > 0 && !firstNameFits(names, key) {
		warnings = append(warnings, WarnFirstNameMismatch)
	}
	return Name{
		First:    strings.Join(names, " "),
		Paternal: strings.Join(surnames[:k], " "),
		Maternal: strings.Join(surnames[k:], " "),
		Warnings: warnings,
	}
}

// splitWithoutComma tries every reading of a name typed without its comma —
// surnames first, as the previous system writes them, then given names first —
// and keeps the one the whole CURP prefix agrees with.
func splitWithoutComma(words []string, key string) (Name, error) {
	if len(words) < 2 || key == "" {
		return Name{}, ErrNameUnreadable
	}
	for cut := len(words) - 1; cut >= 1; cut-- {
		for _, order := range [2]bool{true, false} {
			surnames, names := words[:cut], words[cut:]
			if !order {
				names, surnames = words[:cut], words[cut:]
			}
			for k := 1; k <= len(surnames); k++ {
				if particles[foldWord(surnames[k-1])] {
					continue
				}
				if surnamesFit(surnames[:k], surnames[k:], key) && firstNameFits(names, key) {
					return Name{
						First:    strings.Join(names, " "),
						Paternal: strings.Join(surnames[:k], " "),
						Maternal: strings.Join(surnames[k:], " "),
						Warnings: []string{WarnNameWithoutComma},
					}, nil
				}
			}
		}
	}
	return Name{}, ErrNameUnreadable
}

func surnamesFit(paternal, maternal []string, key string) bool {
	p := significant(paternal)
	if p == "" {
		return false
	}
	second := innerVowel(p)
	third := byte('X')
	if m := significant(maternal); m != "" {
		third = m[0]
	}
	return key[0] == p[0] && (key[1] == second || key[1] == 'X') && key[2] == third
}

func firstNameFits(names []string, key string) bool {
	folded := make([]string, len(names))
	for i, n := range names {
		folded[i] = foldWord(n)
	}
	if len(folded) >= 2 && compoundFirst[folded[0]] {
		folded = folded[1:]
	}
	first := significant(folded)
	return first != "" && key[3] == first[0]
}

// significant is the first word of a name that is not a particle, folded the
// way the CURP writes letters; the last word when all of them are particles.
func significant(words []string) string {
	for _, w := range words {
		if f := foldWord(w); f != "" && !particles[f] {
			return f
		}
	}
	if len(words) == 0 {
		return ""
	}
	return foldWord(words[len(words)-1])
}

// innerVowel is the first vowel after the initial, or X when there is none.
func innerVowel(word string) byte {
	for i := 1; i < len(word); i++ {
		if strings.IndexByte("AEIOU", word[i]) >= 0 {
			return word[i]
		}
	}
	return 'X'
}

// foldWord upper-cases, drops accents and punctuation and writes Ñ as X, the
// way the CURP spells a name.
func foldWord(w string) string {
	w = strings.ToUpper(Fold(w))
	w = strings.ReplaceAll(w, "Ñ", "X")
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, w)
}

// curpKey is the CURP's four name letters, or "" when there is no CURP of the
// right shape to check against.
func curpKey(curp string) string {
	c := strings.ToUpper(strings.TrimSpace(curp))
	if len(c) < 4 {
		return ""
	}
	for i := 0; i < 4; i++ {
		if c[i] < 'A' || c[i] > 'Z' {
			return ""
		}
	}
	return c[:4]
}
