package domain

import "strings"

// Letter is a qualitative grade (owner, 20-oct): the levels that grade with
// letters — preescolar — capture one per subject and period instead of a
// number. Letters are never averaged.
type Letter string

// The four letters, best first.
const (
	LetterA Letter = "A"
	LetterB Letter = "B"
	LetterC Letter = "C"
	LetterD Letter = "D"
)

// CodeGradeNotLetter refuses a capture on a letter level that is not A–D.
const CodeGradeNotLetter RefusalCode = "GRADE_NOT_LETTER"

var letterMeanings = map[Letter]string{
	LetterA: "Excelente",
	LetterB: "Bien",
	LetterC: "Suficiente",
	LetterD: "Requiere apoyo",
}

// ParseLetter reads a typed letter, in either case and trimmed. Anything but
// A–D is KindInvalid with CodeGradeNotLetter.
func ParseLetter(text string) (Letter, error) {
	l := Letter(strings.ToUpper(strings.TrimSpace(text)))
	if _, ok := letterMeanings[l]; !ok {
		return "", Refusal(CodeGradeNotLetter, "La calificación debe ser A, B, C o D")
	}
	return l, nil
}

// Meaning is what the letter says to a family ("Excelente").
func (l Letter) Meaning() string { return letterMeanings[l] }
