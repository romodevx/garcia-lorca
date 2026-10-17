package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestCentsPesos(t *testing.T) {
	for _, tc := range []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{50000, "$500.00"},
		{3230000, "$32,300.00"},
		{123456789, "$1,234,567.89"},
	} {
		assert.Equal(t, tc.want, domain.Cents(tc.cents).Pesos(), "cents %d", tc.cents)
	}
}

// CC-202: a deposit is applied in full or not taken. What the cashier is told
// depends on why: the chosen charges are already paid (two cashiers, the same
// month), or the amount is past what the charges may take.
func TestRequireFullyApplied(t *testing.T) {
	for _, tc := range []struct {
		name               string
		deposit, unapplied int64
		chosen             bool
		wantErr            bool
		wantText           string
	}{
		{"everything applied", 320000, 0, false, false, ""},
		{"everything applied to the chosen", 320000, 0, true, false, ""},
		{"the chosen charges are paid already", 320000, 320000, true, true, "ya está pagado"},
		{"nothing owed at all", 320000, 320000, false, true, "no tiene adeudos"},
		{"past the chosen charges", 670000, 350000, true, true, "excede lo que deben los conceptos elegidos (máximo $3,200.00)"},
		{"past the whole debt", 9999900, 6769900, false, true, "excede lo que el alumno debe (máximo $32,300.00)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.RequireFullyApplied(tc.deposit, tc.unapplied, tc.chosen)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Contains(t, glerrors.SafeMessage(err), tc.wantText)
		})
	}
}
