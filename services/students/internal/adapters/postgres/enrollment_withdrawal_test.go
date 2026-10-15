package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestEnrollmentWithdrawalMapperRoundTrip(t *testing.T) {
	enrolledAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	withdrawnOn := time.Date(2026, 10, 1, 18, 0, 0, 0, time.FixedZone("west", -7*60*60))
	enrollment, err := domain.NewEnrollment("enrollment-a", "tenant-a", "student-a", "cycle-a", "group-a", "", domain.EnrollmentStatusActive, enrolledAt)
	require.NoError(t, err)
	withdrawn, err := enrollment.Withdraw(withdrawnOn, "  family moved  ")
	require.NoError(t, err)

	stored := enrollmentFromDomain(withdrawn)
	require.NotNil(t, stored.WithdrawnOn)
	require.NotNil(t, stored.WithdrawalReason)
	assert.Equal(t, "family moved", *stored.WithdrawalReason)

	got, err := enrollmentToDomain(stored)
	require.NoError(t, err)
	assert.Equal(t, domain.EnrollmentStatusWithdrawn, got.Status())
	assert.Equal(t, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), got.WithdrawnOn())
	assert.Equal(t, got.WithdrawnOn(), got.EndedAt())
	assert.Equal(t, "family moved", got.WithdrawalReason())
}
