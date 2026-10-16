//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Group usage (T8, ADR 0036): a tenant and group of this file's own.
const (
	tenantT8    = "88888888-8888-4888-8888-888888888888"
	usageGroup  = "48000000-0000-4000-8000-0000000000a1"
	usageOther  = "48000000-0000-4000-8000-0000000000a2"
	usageTaker  = "48000000-0000-4000-8000-0000000000a3"
	usagePupil  = "48000000-0000-4000-8000-0000000000a4"
	usagePupil2 = "48000000-0000-4000-8000-0000000000a5"
)

func TestGroupUsageCountsSessionsAndMarksOfTheGroup(t *testing.T) {
	require.NoError(t, superDB.Exec(`INSERT INTO attendance_sessions (tenant_id, group_id, date, subject_id, taken_by, taken_by_name) VALUES
		(?, ?, '2026-09-01', NULL, ?, 'Gabriela'), (?, ?, '2026-09-02', NULL, ?, 'Gabriela'), (?, ?, '2026-09-01', NULL, ?, 'Gabriela')`,
		tenantT8, usageGroup, usageTaker, tenantT8, usageGroup, usageTaker, tenantT8, usageOther, usageTaker).Error)
	require.NoError(t, superDB.Exec(`INSERT INTO attendance (tenant_id, enrollment_id, date, status, group_id) VALUES
		(?, ?, '2026-09-01', 'absent', ?), (?, ?, '2026-09-01', 'absent', ?)`,
		tenantT8, usagePupil, usageGroup, tenantT8, usagePupil2, usageOther).Error)
	r, _ := archiveRepo(t, tenantT8)

	got, err := r.GroupUsage(context.Background(), usageGroup)

	require.NoError(t, err)
	assert.Equal(t, service.GroupUsage{Sessions: 2, Marks: 1}, got)

	hidden, _ := archiveRepo(t, tenantC)
	none, err := hidden.GroupUsage(context.Background(), usageGroup)
	require.NoError(t, err)
	assert.Equal(t, service.GroupUsage{}, none, "RLS: another tenant sees none of it")
}
