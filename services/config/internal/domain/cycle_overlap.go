package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Refusal codes for the dates a school states about its year (QA CC-101/106)
// and for renaming a year (CC-103).
const (
	CodeCycleOverlap      RefusalCode = "CYCLE_OVERLAP"
	CodeCycleNameRequired RefusalCode = "CYCLE_NAME_REQUIRED"
	CodeNonWorkingOverlap RefusalCode = "NON_WORKING_OVERLAP"
	CodeCycleNameTooLong  RefusalCode = "CYCLE_NAME_TOO_LONG"
)

// maxCycleNameRunes bounds a cycle's name: it is what every selector shows.
const maxCycleNameRunes = 80

// CheckCycleName is CYCLE_NAME_TOO_LONG (KindInvalid) for a name of more than
// 80 characters (runes, so "Ñ" counts once).
//
// Called on the write paths (create, rename) and deliberately NOT by NewCycle:
// NewCycle also rebuilds every stored row, and a rule added after the fact
// must not make a saved year unreadable.
func CheckCycleName(name string) error {
	if utf8.RuneCountInString(strings.TrimSpace(name)) > maxCycleNameRunes {
		return refusal(glerrors.KindInvalid, CodeCycleNameTooLong,
			fmt.Sprintf("El nombre del ciclo admite hasta %d caracteres", maxCycleNameRunes))
	}
	return nil
}

// rangesShare reports whether two inclusive day ranges share at least one day.
func rangesShare(aFrom, aTo, bFrom, bTo time.Time) bool {
	return !aFrom.After(bTo) && !bFrom.After(aTo)
}

// describeRange is "16 de noviembre de 2026" for one day and "del … al …" for
// a run of days — how a person reads a span on the calendar.
func describeRange(from, to time.Time) string {
	if from.Equal(to) {
		return FormatSpanishDate(from)
	}
	return "del " + FormatSpanishDate(from) + " al " + FormatSpanishDate(to)
}

// RefuseCycleOverlap is CYCLE_OVERLAP (KindConflict) when c shares a day with
// any cycle of others other than c itself, naming the earliest such cycle;
// nil otherwise.
//
// Two school years that share a day make "which year is this date in" — the
// question attendance, billing and the calendar all ask — answer two ways, and
// activating the second one pauses the first for the whole school. The QA found
// it by accident (CC-101): a year from January to December inside the live
// August-to-July one was accepted and could be activated.
//
// A school has a handful of cycles, so a scan is the right tool; others is
// sorted by start first so the cycle named is the one a person meets first.
func RefuseCycleOverlap(c Cycle, others []Cycle) error {
	sorted := append([]Cycle(nil), others...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].startsOn.Before(sorted[j].startsOn) })
	for _, o := range sorted {
		if o.id == c.id || !rangesShare(c.startsOn, c.endsOn, o.startsOn, o.endsOn) {
			continue
		}
		return refusal(glerrors.KindConflict, CodeCycleOverlap,
			fmt.Sprintf("Las fechas se traslapan con el ciclo %q (%s)", o.name, describeRange(o.startsOn, o.endsOn)))
	}
	return nil
}

// Rename is the same cycle under a new name (trimmed). A blank name is
// CYCLE_NAME_REQUIRED: the name is what every selector shows.
func (c Cycle) Rename(name string) (Cycle, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Cycle{}, refusal(glerrors.KindInvalid, CodeCycleNameRequired, "Escribe el nombre del ciclo")
	}
	if err := CheckCycleName(trimmed); err != nil {
		return Cycle{}, err
	}
	c.name = trimmed
	return c, nil
}

// RefuseNonWorkingOverlap is NON_WORKING_OVERLAP (KindConflict) when p shares a
// day with a closed period of others other than p itself, naming the earliest
// such period; nil otherwise.
//
// The same day closed twice (CC-106: the 16th of November entered twice under
// two labels) is not two closures. The billing arithmetic would count it once,
// but the calendar shows it twice and deleting one entry leaves the day closed,
// which reads as a delete that did nothing.
func RefuseNonWorkingOverlap(p NonWorkingPeriod, others []NonWorkingPeriod) error {
	sorted := append([]NonWorkingPeriod(nil), others...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].from.Before(sorted[j].from) })
	for _, o := range sorted {
		if o.id == p.id || !rangesShare(p.from, p.to, o.from, o.to) {
			continue
		}
		return refusal(glerrors.KindConflict, CodeNonWorkingOverlap,
			fmt.Sprintf("Ese día ya está marcado sin clases: %q (%s)", o.label, describeRange(o.from, o.to)))
	}
	return nil
}
