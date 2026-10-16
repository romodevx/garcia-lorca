package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

type fakeGroupUsageRepo struct{ asked string }

func (f *fakeGroupUsageRepo) GroupUsage(_ context.Context, groupID string) (service.GroupUsage, error) {
	f.asked = groupID
	return service.GroupUsage{Sessions: 40, Marks: 7}, nil
}

func TestGroupUsageCountsTheGroupsAttendance(t *testing.T) {
	repo := &fakeGroupUsageRepo{}
	const group = "00000000-0000-4000-8000-000000000010"

	got, err := service.NewGroupUsageService(repo).GroupUsage(context.Background(), group)

	require.NoError(t, err)
	assert.Equal(t, service.GroupUsage{Sessions: 40, Marks: 7}, got)
	assert.Equal(t, group, repo.asked)
}

func TestGroupUsageRefusesAMalformedGroup(t *testing.T) {
	repo := &fakeGroupUsageRepo{}

	_, err := service.NewGroupUsageService(repo).GroupUsage(context.Background(), "nope")

	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Empty(t, repo.asked)
}
