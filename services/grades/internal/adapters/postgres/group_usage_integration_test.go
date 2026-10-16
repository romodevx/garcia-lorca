//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Group usage (T8, ADR 0036): a tenant and group of this file's own.
const (
	tenantT8   = "88888888-8888-4888-8888-888888888888"
	usageGroup = "48000000-0000-4000-8000-0000000000c1"
	usageOther = "48000000-0000-4000-8000-0000000000c2"
)

func TestCountGroupReportCardsCountsEveryVersionOfTheGroup(t *testing.T) {
	insert := func(group, enrollment string, version int, status string) {
		require.NoError(t, superDB.Exec(`INSERT INTO report_cards (tenant_id, term_id, cycle_id, level_id, group_id, enrollment_id, student_id,
			version, status, issued_at, issued_by, student_name, months, scale_min_hundredths, scale_max_hundredths, scale_decimals, scale_passing_hundredths)
			VALUES (?, gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), ?, ?, gen_random_uuid(), ?, ?, now(), gen_random_uuid(), 'Ana', '{}', 500, 1000, 1, 600)`,
			tenantT8, group, enrollment, version, status).Error)
	}
	insert(usageGroup, "e8000000-0000-4000-8000-0000000000c1", 1, "superseded")
	insert(usageGroup, "e8000000-0000-4000-8000-0000000000c2", 2, "published")
	insert(usageOther, "e8000000-0000-4000-8000-0000000000c3", 1, "published")

	n, err := appRepo(t, tenantT8).CountGroupReportCards(context.Background(), usageGroup)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "a superseded card is still history")

	hidden, err := appRepo(t, tenantG).CountGroupReportCards(context.Background(), usageGroup)
	require.NoError(t, err)
	assert.Zero(t, hidden, "RLS: another tenant sees none of it")
}
