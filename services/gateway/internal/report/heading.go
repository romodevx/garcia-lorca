package report

import (
	"fmt"
	"time"
)

// RangeLabel names an inclusive span of days in Spanish: one day reads as the
// day, more as "Del … al …".
func RangeLabel(from, to time.Time) string {
	if from.Equal(to) {
		return longDay(from)
	}
	return fmt.Sprintf("Del %s al %s", longDay(from), longDay(to))
}

// SchoolHeading is the issuer's lines a report opens with: the school's name,
// its legal name and its RFC, skipping whatever is empty.
func SchoolHeading(name, legalName, rfc string) []string {
	if rfc != "" {
		rfc = "RFC " + rfc
	}
	out := make([]string, 0, 3)
	for _, line := range []string{name, legalName, rfc} {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
