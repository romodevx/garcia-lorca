package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// CC-110: MATERNAL was saved with a monthly fee of $0 and answered 200. An
// active table that charges nothing a month issues ten settled $0 charges per
// pupil, which read as "paid" on every statement.
func TestUpsertTuitionPlanRefusesAnActiveTableWithNoMonthlyFee(t *testing.T) {
	cases := map[string]struct {
		monthly  int64
		active   bool
		schedule []domain.Installment
		refused  bool
	}{
		"zero monthly":                       {monthly: 0, active: true, refused: true},
		"negative monthly":                   {monthly: -100, active: true, refused: true},
		"one cent is a price":                {monthly: 1, active: true},
		"an inactive table charges nothing":  {monthly: 0, active: false},
		"a named schedule carries the price": {monthly: 0, active: true, schedule: []domain.Installment{{Month: 9, AmountCents: 300000}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.cycles.byID[testCycle] = ports.CycleRef{ID: testCycle, StartsOn: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}

			_, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
				CycleID: testCycle, LevelID: testLevel, MonthlyCents: tc.monthly,
				Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: tc.active, Schedule: tc.schedule,
			})

			if !tc.refused {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			if tc.monthly == 0 {
				assert.Equal(t, "La colegiatura mensual debe ser mayor a $0", glerrors.SafeMessage(err))
			}
		})
	}
}
