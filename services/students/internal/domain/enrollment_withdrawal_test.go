package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrollmentWithdraw(t *testing.T) {
	enrolledAt := time.Date(2026, time.January, 12, 23, 45, 0, 0, time.FixedZone("west", -7*60*60))
	withdrawnOn := time.Date(2026, time.February, 4, 1, 30, 0, 0, time.FixedZone("east", 3*60*60))
	now := time.Date(2026, time.February, 4, 1, 0, 0, 0, time.UTC)
	newActive := func(t *testing.T) domain.Enrollment {
		t.Helper()
		e, err := domain.NewEnrollment("enr-1", "tenant-1", "student-1", "cycle-1", "group-1", "", domain.EnrollmentStatusActive, enrolledAt)
		require.NoError(t, err)
		return e
	}

	t.Run("records UTC calendar date and trims reason", func(t *testing.T) {
		got, err := newActive(t).Withdraw(withdrawnOn, "  cambio de domicilio  ", now)
		require.NoError(t, err)
		assert.Equal(t, domain.EnrollmentStatusWithdrawn, got.Status())
		assert.Equal(t, time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC), got.WithdrawnOn())
		assert.Equal(t, "cambio de domicilio", got.WithdrawalReason())
		assert.Equal(t, got.WithdrawnOn(), got.EndedAt())
	})
	t.Run("defaults a missing date to today", func(t *testing.T) {
		today := now.UTC()
		got, err := newActive(t).Withdraw(time.Time{}, "", now)
		require.NoError(t, err)
		assert.Equal(t, time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC), got.WithdrawnOn())
	})
	t.Run("accepts exactly 500 runes", func(t *testing.T) {
		got, err := newActive(t).Withdraw(withdrawnOn, strings.Repeat("界", 500), now)
		require.NoError(t, err)
		assert.Len(t, []rune(got.WithdrawalReason()), 500)
	})

	tests := []struct {
		name   string
		status domain.EnrollmentStatus
		on     time.Time
		reason string
	}{
		{name: "not active", status: domain.EnrollmentStatusTransferred, on: withdrawnOn, reason: "motivo"},
		{name: "future date", status: domain.EnrollmentStatusActive, on: now.AddDate(10, 0, 0), reason: "motivo"},
		{name: "before enrollment day", status: domain.EnrollmentStatusActive, on: time.Date(2026, 1, 11, 23, 59, 0, 0, time.UTC), reason: "motivo"},
		{name: "reason over 500 runes", status: domain.EnrollmentStatusActive, on: withdrawnOn, reason: strings.Repeat("界", 501)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := domain.NewEnrollment("enr-1", "tenant-1", "student-1", "cycle-1", "group-1", "", tt.status, enrolledAt)
			require.NoError(t, err)
			_, err = e.Withdraw(tt.on, tt.reason, now)
			want := glerrors.KindInvalid
			if tt.name == "not active" {
				want = glerrors.KindConflict
			}
			assert.Equal(t, want, glerrors.KindOf(err))
		})
	}
}

func TestEnrollmentWithdrawUsesCallerClockForFutureDate(t *testing.T) {
	now := time.Date(2026, time.January, 1, 1, 0, 0, 0, time.UTC)
	enrolledAt := time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)
	e, err := domain.NewEnrollment("enr-1", "tenant-1", "student-1", "cycle-1", "group-1", "", domain.EnrollmentStatusActive, enrolledAt)
	require.NoError(t, err)

	_, err = e.Withdraw(time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), "motivo", now)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}
