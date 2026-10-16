package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const someGroup = "00000000-0000-4000-8000-000000000010"

type fakeGroupRecords struct {
	usage       ports.GroupUsage
	askedGroup  string
	renamedTo   string
	refreshedOn int64
}

func (f *fakeGroupRecords) GroupUsage(_ context.Context, groupID string) (ports.GroupUsage, error) {
	f.askedGroup = groupID
	return f.usage, nil
}

func (f *fakeGroupRecords) RefreshGroupName(_ context.Context, groupID, name string) (int64, error) {
	f.askedGroup, f.renamedTo = groupID, name
	return f.refreshedOn, nil
}

func TestGroupUsageAsksTheRepository(t *testing.T) {
	repo := &fakeGroupRecords{usage: ports.GroupUsage{Enrollments: 3, Transfers: 1}}
	svc := usecase.NewGroupRecordService(repo)

	got, err := svc.GroupUsage(context.Background(), someGroup)

	require.NoError(t, err)
	assert.Equal(t, ports.GroupUsage{Enrollments: 3, Transfers: 1}, got)
	assert.Equal(t, someGroup, repo.askedGroup)
}

func TestRefreshGroupNameWritesTheTrimmedName(t *testing.T) {
	repo := &fakeGroupRecords{refreshedOn: 25}
	svc := usecase.NewGroupRecordService(repo)

	n, err := svc.RefreshGroupName(context.Background(), someGroup, "  3° B ")

	require.NoError(t, err)
	assert.Equal(t, int64(25), n)
	assert.Equal(t, "3° B", repo.renamedTo)
}

func TestGroupRecordsRefuseBadInputBeforeTheRepository(t *testing.T) {
	cases := map[string]func(*usecase.GroupRecordService) error{
		"usage, malformed group": func(s *usecase.GroupRecordService) error {
			_, err := s.GroupUsage(context.Background(), "not-a-uuid")
			return err
		},
		"refresh, malformed group": func(s *usecase.GroupRecordService) error {
			_, err := s.RefreshGroupName(context.Background(), "not-a-uuid", "B")
			return err
		},
		"refresh, blank name": func(s *usecase.GroupRecordService) error {
			_, err := s.RefreshGroupName(context.Background(), someGroup, "  ")
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeGroupRecords{}
			err := call(usecase.NewGroupRecordService(repo))
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Empty(t, repo.askedGroup, "the repository is never asked")
		})
	}
}
