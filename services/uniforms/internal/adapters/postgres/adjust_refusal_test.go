package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// TestAdjustRefusalNamesTheRealCause (C2-002): the conditional UPDATE refuses
// two different corrections with one zero-row result. Taking the shelf below
// zero with nothing held must not be blamed on families' holds, because the
// person fixing it looks for a hold that does not exist.
func TestAdjustRefusalNamesTheRealCause(t *testing.T) {
	cases := []struct {
		name     string
		onHand   int
		reserved int
		delta    int
		want     string
	}{
		{"below zero, nothing held", 3, 0, -5, "an adjustment cannot take the count below zero"},
		{"below zero, something held", 3, 2, -5, "an adjustment cannot take the count below zero"},
		{"above zero but below the holds", 5, 4, -3, "an adjustment cannot take the count below what is held"},
		{"a race made it valid since the update", 9, 0, -3, "an adjustment cannot take the count below what is held"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := stockModel{ID: "st1", TenantID: "t1", ItemID: "i1", SizeLabel: "8",
				SizeOrder: 1, OnHand: tc.onHand, Reserved: tc.reserved}

			err := adjustRefusal(m, tc.delta)

			require.Error(t, err)
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
