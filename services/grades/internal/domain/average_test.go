package domain_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

// averageVectors is the ONE table of cases the web, the parents' app and this
// service run (T15): packages/domain/src/gradeAverage.vectors.json.
type averageVectors struct {
	Subject []struct {
		Values []string `json:"values"`
		Want   string   `json:"want"`
	} `json:"subject"`
	General []struct {
		Subjects [][]string `json:"subjects"`
		Want     string     `json:"want"`
	} `json:"general"`
}

func loadAverageVectors(t *testing.T) averageVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "domain", "src", "gradeAverage.vectors.json"))
	require.NoError(t, err)
	var v averageVectors
	require.NoError(t, json.Unmarshal(raw, &v))
	require.NotEmpty(t, v.Subject)
	require.NotEmpty(t, v.General)
	return v
}

// parseValues reads decimal text as exact grade values (at most 2 decimals).
func parseValues(t *testing.T, texts []string) []domain.GradeValue {
	t.Helper()
	scale, err := domain.NewGradingScale(0, 10000, 100, 2)
	require.NoError(t, err)
	out := make([]domain.GradeValue, 0, len(texts))
	for _, s := range texts {
		v, err := scale.ParseValue(s)
		require.NoError(t, err, s)
		out = append(out, v)
	}
	return out
}

// T15: a subject's average is the EXACT mean of its grades, truncated (never
// rounded) to 2 decimals, printed without trailing zeros.
func TestSubjectAverage_SharedVectors(t *testing.T) {
	for _, tc := range loadAverageVectors(t).Subject {
		t.Run(tc.Want, func(t *testing.T) {
			got := domain.SubjectAverage(parseValues(t, tc.Values))
			assert.Equal(t, tc.Want, got.String(), "values %v", tc.Values)
			assert.Equal(t, tc.Want != "", got.Present())
		})
	}
}

// The general average is the exact mean of the subjects' EXACT averages,
// truncated once at the end (subjects without grades are ignored).
func TestGeneralAverage_SharedVectors(t *testing.T) {
	for _, tc := range loadAverageVectors(t).General {
		t.Run(tc.Want, func(t *testing.T) {
			subjects := make([]domain.Average, 0, len(tc.Subjects))
			for _, s := range tc.Subjects {
				subjects = append(subjects, domain.SubjectAverage(parseValues(t, s)))
			}
			assert.Equal(t, tc.Want, domain.GeneralAverage(subjects).String(), "subjects %v", tc.Subjects)
		})
	}
}

// A stored average (report_cards / report_card_lines keep 2 decimals) reads
// back exactly as it was frozen.
func TestAverageFromHundredths(t *testing.T) {
	a := domain.AverageFromHundredths(866)
	assert.True(t, a.Present())
	assert.Equal(t, 866, a.Hundredths())
	assert.Equal(t, "8.66", a.String())
	assert.Equal(t, "8.5", domain.AverageFromHundredths(850).String())
	assert.Equal(t, "10", domain.AverageFromHundredths(1000).String())
}

// Two averages are the same when they print the same: a re-close comparing a
// fresh exact average (26/3) with the stored, truncated one (8.66) must not
// see a change.
func TestAverageSameAs(t *testing.T) {
	fresh := domain.SubjectAverage(parseValues(t, []string{"9", "8", "9"}))
	assert.True(t, fresh.SameAs(domain.AverageFromHundredths(866)))
	assert.False(t, fresh.SameAs(domain.AverageFromHundredths(867)))
	assert.True(t, domain.Average{}.SameAs(domain.Average{}))
	assert.False(t, fresh.SameAs(domain.Average{}))
}
