//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Group records (T8, ADR 0036). A tenant and groups of this file's own: the
// suite's shared tenants are counted by other tests.
const tenantT8 = "88888888-8888-4888-8888-888888888888"

// groupRecordsFixture: an active pupil and a withdrawn one in group, a pupil
// moved out of it into other, and one who was only ever in other. n (1-9)
// keeps each test's rows apart in the shared database.
func groupRecordsFixture(t *testing.T, n int) (group, other, active, withdrawn string) {
	t.Helper()
	id := func(prefix string, i int) string {
		return fmt.Sprintf("%s-0000-4000-8000-0000000000%d%d", prefix, n, i)
	}
	curp := func(i int) string { return fmt.Sprintf("HEGB050312HDFRNN%d%d", n, i) }
	group, other = id("48000000", 1), id("48000000", 2)
	ctx := context.Background()
	repo := appRepo(t, tenantT8)
	active = seedStudent(t, tenantT8, id("d8000000", 1), curp(1))
	withdrawn = seedStudent(t, tenantT8, id("d8000000", 2), curp(2))
	moved := seedStudent(t, tenantT8, id("d8000000", 3), curp(3))
	elsewhere := seedStudent(t, tenantT8, id("d8000000", 4), curp(4))
	enrActive, enrWithdrawn, enrMoved, enrElsewhere := id("e8000000", 1), id("e8000000", 2), id("e8000000", 3), id("e8000000", 4)
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrActive, tenantT8, active, group)))
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrWithdrawn, tenantT8, withdrawn, group)))
	require.NoError(t, repo.SetEnrollmentStatus(ctx, enrWithdrawn, domain.EnrollmentStatusWithdrawn))
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrMoved, tenantT8, moved, group)))
	move, err := domain.NewEnrollmentTransfer(id("78000000", 1), tenantT8, enrMoved, group, other, "balanceo", enrollClock.AddDate(0, 0, 3))
	require.NoError(t, err)
	require.NoError(t, repo.TransferEnrollment(ctx, ports.TransferCommand{
		EnrollmentID: enrMoved, To: ports.GroupSeats{ID: other, Name: "1° B", Capacity: 30}, Transfer: move,
	}))
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrElsewhere, tenantT8, elsewhere, other)))
	return group, other, active, withdrawn
}

func gradeGroupOf(t *testing.T, tenant, studentID string) string {
	t.Helper()
	s, err := appRepo(t, tenant).FindByID(context.Background(), studentID)
	require.NoError(t, err)
	return s.GradeGroup()
}

func TestGroupUsageCountsEveryEnrollmentAndTransferOfTheGroup(t *testing.T) {
	group, other, _, _ := groupRecordsFixture(t, 1)
	ctx := context.Background()
	repo := appRepo(t, tenantT8)

	left, err := repo.GroupUsage(ctx, group)
	require.NoError(t, err)
	assert.Equal(t, ports.GroupUsage{Enrollments: 2, Transfers: 1}, left,
		"the active and the withdrawn enrollment, and the move out")

	joined, err := repo.GroupUsage(ctx, other)
	require.NoError(t, err)
	assert.Equal(t, ports.GroupUsage{Enrollments: 2, Transfers: 1}, joined, "the move in counts too")

	hidden, err := appRepo(t, tenantB).GroupUsage(ctx, group)
	require.NoError(t, err)
	assert.Equal(t, ports.GroupUsage{}, hidden, "RLS: another tenant sees none of it")
}

func TestRefreshGroupNameRelabelsOnlyTheActivePupilsFiles(t *testing.T) {
	group, _, active, withdrawn := groupRecordsFixture(t, 2)
	ctx := context.Background()
	repo := appRepo(t, tenantT8)

	n, err := appRepo(t, tenantB).RefreshGroupName(ctx, group, "Robado")
	require.NoError(t, err)
	assert.Zero(t, n, "RLS: another tenant changes nothing")

	n, err = repo.RefreshGroupName(ctx, group, "1° Z")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.Equal(t, "1° Z", gradeGroupOf(t, tenantT8, active))
	assert.Equal(t, "1° A", gradeGroupOf(t, tenantT8, withdrawn), "a pupil who left keeps the label they left with")

	again, err := repo.RefreshGroupName(ctx, group, "1° Z")
	require.NoError(t, err)
	assert.Zero(t, again, "idempotent")
}
