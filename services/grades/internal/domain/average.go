package domain

import "math/big"

// Average is a system-computed average (T15): the EXACT arithmetic mean of the
// grades that exist, held as a rational — or no average at all (the zero
// value): a subject without a grade has no average, which is not a zero.
//
// It is shown and stored TRUNCATED, never rounded, to two decimals (8.666… →
// 8.66), and printed without trailing zeros ("8.5", "9"). No float takes part:
// the web and the parents' app run the same table of cases
// (packages/domain/src/gradeAverage.vectors.json).
type Average struct {
	exact   *big.Rat // in grade units; nil when absent
	present bool
}

// AverageFromHundredths rebuilds a stored (already truncated) average.
func AverageFromHundredths(h int) Average {
	return Average{exact: big.NewRat(int64(h), hundredthsPerUnit), present: true}
}

// Present reports whether there is an average.
func (a Average) Present() bool { return a.present }

// Hundredths is the average truncated to whole hundredths (8.666… → 866);
// meaningless when not Present.
func (a Average) Hundredths() int {
	if !a.present {
		return 0
	}
	scaled := new(big.Rat).Mul(a.exact, big.NewRat(hundredthsPerUnit, 1))
	return int(new(big.Int).Quo(scaled.Num(), scaled.Denom()).Int64())
}

// String is the truncated average without trailing zeros ("8.66", "8.5",
// "9"), or "" when there is none.
func (a Average) String() string {
	if !a.present {
		return ""
	}
	return formatHundredths(a.Hundredths())
}

// SameAs reports whether a and b read the same: both absent, or both present
// with the same truncated value. An exact 26/3 is the same as a stored 8.66.
func (a Average) SameAs(b Average) bool {
	if a.present != b.present {
		return false
	}
	return !a.present || a.Hundredths() == b.Hundredths()
}

// SubjectAverage is a subject's average: the exact mean of its grades. No
// grades, no average.
func SubjectAverage(grades []GradeValue) Average {
	if len(grades) == 0 {
		return Average{}
	}
	sum := int64(0)
	for _, g := range grades {
		sum += int64(g.Hundredths())
	}
	return Average{exact: big.NewRat(sum, int64(len(grades))*hundredthsPerUnit), present: true}
}

// GeneralAverage is the exact mean of the subjects' EXACT averages, counting
// only subjects that have one; it is truncated only when read. No graded
// subject, no average.
func GeneralAverage(subjects []Average) Average {
	sum, n := new(big.Rat), int64(0)
	for _, s := range subjects {
		if s.present {
			sum.Add(sum, s.exact)
			n++
		}
	}
	if n == 0 {
		return Average{}
	}
	return Average{exact: sum.Quo(sum, big.NewRat(n, 1)), present: true}
}
