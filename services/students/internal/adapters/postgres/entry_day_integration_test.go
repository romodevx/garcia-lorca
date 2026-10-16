//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// T9: a chosen entry day is stored as 00:00 Morelia (06:00 UTC) and read back
// as that same instant, on both enrol writes, so finance's school-day reading
// of enrolled_at lands on the day the office typed.
func TestAnEntryDayPersistsAsMoreliaMidnight(t *testing.T) {
	zone, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	entry := time.Date(2026, time.August, 24, 0, 0, 0, 0, zone)
	repo := appRepo(t, tenantA)
	ctx := context.Background()

	byGroup := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000901", "HEGB050312HDFRNN91")
	byGrade := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000902", "HEGB050312HDFRNN92")
	const enrGroup, enrGrade = "e0000000-0000-0000-0000-000000000901", "e0000000-0000-0000-0000-000000000902"

	e, err := domain.NewEnrollment(enrGroup, tenantA, byGroup, cycleUUID, groupOne, "", domain.EnrollmentStatusActive, entry)
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, e))

	draft, err := domain.NewEnrollment(enrGrade, tenantA, byGrade, cycleUUID, groupThree, "", domain.EnrollmentStatusActive, entry)
	require.NoError(t, err)
	_, err = repo.CreateEnrollmentInFirstFreeGroup(ctx, draft, []ports.GroupSeats{
		{ID: groupThree, Name: "1° C", CycleID: cycleUUID, Capacity: 30},
	})
	require.NoError(t, err)

	for _, id := range []string{enrGroup, enrGrade} {
		got, err := repo.GetEnrollment(ctx, id)
		require.NoError(t, err)
		assert.True(t, got.EnrolledAt().Equal(entry), "%s enrolled_at = %s, want %s", id, got.EnrolledAt(), entry)
		assert.Equal(t, time.Date(2026, time.August, 24, 6, 0, 0, 0, time.UTC), got.EnrolledAt().UTC())
		assert.Equal(t, "2026-08-24", got.EnrolledAt().In(zone).Format(time.DateOnly))
	}
}
