package service

import (
	"context"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// GroupUsage is what attendance keeps of one config group: the pass lists
// taken in it and the marks stamped with it.
type GroupUsage struct {
	Sessions int64
	Marks    int64
}

// GroupUsageRepo counts a group's attendance, tenant-bound. An unknown group
// is zeros: attendance does not own groups, only records of them.
type GroupUsageRepo interface {
	GroupUsage(ctx context.Context, groupID string) (GroupUsage, error)
}

// GroupUsageService answers whether a group has attendance history, so the
// gateway can refuse to delete it (T8, ADR 0036).
type GroupUsageService struct {
	repo GroupUsageRepo
}

// NewGroupUsageService wires the service.
func NewGroupUsageService(repo GroupUsageRepo) *GroupUsageService {
	return &GroupUsageService{repo: repo}
}

// GroupUsage counts the group's sessions and marks (KindInvalid on a
// malformed id).
func (s *GroupUsageService) GroupUsage(ctx context.Context, groupID string) (GroupUsage, error) {
	if err := uuid.Validate(groupID); err != nil {
		return GroupUsage{}, glerrors.New(glerrors.KindInvalid, "invalid group id")
	}
	return s.repo.GroupUsage(ctx, groupID)
}
