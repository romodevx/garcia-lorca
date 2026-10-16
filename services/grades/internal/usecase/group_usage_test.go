package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

type fakeGroupUsageRepo struct{ asked string }

func (f *fakeGroupUsageRepo) CountGroupReportCards(_ context.Context, groupID string) (int64, error) {
	f.asked = groupID
	return 12, nil
}

func TestGroupUsageCountsTheGroupsReportCards(t *testing.T) {
	repo := &fakeGroupUsageRepo{}
	const group = "00000000-0000-4000-8000-000000000010"

	n, err := usecase.NewGroupUsageService(repo).ReportCards(context.Background(), group)

	require.NoError(t, err)
	assert.Equal(t, int64(12), n)
	assert.Equal(t, group, repo.asked)
}

func TestGroupUsageRefusesAMalformedGroup(t *testing.T) {
	repo := &fakeGroupUsageRepo{}

	_, err := usecase.NewGroupUsageService(repo).ReportCards(context.Background(), "nope")

	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Empty(t, repo.asked)
}
