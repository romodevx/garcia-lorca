package domain

import "fmt"

// Average is a report-card average held in exact tenths (8.5 = 85), or no
// average at all (the zero value): a subject without a grade in the term has
// no trimester grade, which is not the same as a zero.
//
// Every average is computed with integer arithmetic on exact hundredths and
// rounded HALF-UP to one decimal. A float64 would read 8.45 as 8.4499… and
// round it down; the owner's rule is that 8.45 is 8.5.
type Average struct {
	tenths  int
	present bool
}

// hundredthsPerTenth is how many hundredths make a tenth.
const hundredthsPerTenth = 10

// AverageFromTenths rebuilds a stored average.
func AverageFromTenths(tenths int) Average { return Average{tenths: tenths, present: true} }

// Present reports whether there is an average.
func (a Average) Present() bool { return a.present }

// Tenths is the average in tenths (meaningless when not Present).
func (a Average) Tenths() int { return a.tenths }

// String is the average with exactly one decimal ("8.5", "10.0"), or "" when
// there is none.
func (a Average) String() string {
	if !a.present {
		return ""
	}
	return fmt.Sprintf("%d.%d", a.tenths/hundredthsPerTenth, a.tenths%hundredthsPerTenth)
}

// SubjectAverage is a subject's trimester grade: the arithmetic mean of its
// captured monthly grades, one decimal, half-up. No grades, no average.
func SubjectAverage(grades []GradeValue) Average {
	if len(grades) == 0 {
		return Average{}
	}
	sum := 0
	for _, g := range grades {
		sum += g.Hundredths()
	}
	return Average{tenths: roundHalfUp(sum, len(grades)*hundredthsPerTenth), present: true}
}

// GeneralAverage is the mean of the subjects' trimester grades, counting only
// subjects that have one, one decimal, half-up. No graded subject, no average.
func GeneralAverage(subjects []Average) Average {
	sum, n := 0, 0
	for _, s := range subjects {
		if s.present {
			sum += s.tenths
			n++
		}
	}
	if n == 0 {
		return Average{}
	}
	return Average{tenths: roundHalfUp(sum, n), present: true}
}

// roundHalfUp is numerator/denominator rounded half-up, for a non-negative
// numerator and a positive denominator: floor((2n + d) / 2d).
func roundHalfUp(numerator, denominator int) int {
	return (2*numerator + denominator) / (2 * denominator)
}
