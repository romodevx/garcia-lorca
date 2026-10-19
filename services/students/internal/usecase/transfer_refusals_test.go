package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// TestTransferRefusalsAreSpanish (CC-405): the office reads these refusals as
// the service wrote them, like the full-group 409 ("ese grupo ya no tiene
// lugares"). An English 400 reached the screen for a withdrawn pupil and for
// a move into the group they are already in.
func TestTransferRefusalsAreSpanish(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T, svc *usecase.EnrollmentService, e domain.Enrollment, catalog *fakeCatalog) string
		want    string
	}{
		{
			name: "withdrawn pupil",
			arrange: func(t *testing.T, svc *usecase.EnrollmentService, e domain.Enrollment, catalog *fakeCatalog) string {
				catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", Capacity: 30}
				_, err := svc.Withdraw(context.Background(), e.ID(), e.EnrolledAt(), "se mudó")
				require.NoError(t, err)
				return "grp-2"
			},
			want: "el alumno está dado de baja: no se puede cambiar de grupo",
		},
		{
			name: "same group",
			arrange: func(_ *testing.T, _ *usecase.EnrollmentService, _ domain.Enrollment, _ *fakeCatalog) string {
				return "grp-1"
			},
			want: "el alumno ya está en ese grupo",
		},
		{
			name: "another cycle",
			arrange: func(_ *testing.T, _ *usecase.EnrollmentService, _ domain.Enrollment, catalog *fakeCatalog) string {
				catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-other", Capacity: 30}
				return "grp-2"
			},
			want: "ese grupo es de otro ciclo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, students, _, catalog, _ := newEnrollService(t)
			e := enrolledStudent(t, svc, students, catalog)
			to := tc.arrange(t, svc, e, catalog)

			_, err := svc.Transfer(context.Background(), e.ID(), to, "why")

			require.Error(t, err)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Equal(t, tc.want, glerrors.SafeMessage(err))
		})
	}
}
