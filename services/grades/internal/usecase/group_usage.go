package usecase

import (
	"context"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// GroupUsageService answers whether a group has report-card history, so the
// gateway can refuse to delete it (T8, ADR 0036).
type GroupUsageService struct {
	repo ports.GroupUsageRepository
}

// NewGroupUsageService wires the use case.
func NewGroupUsageService(repo ports.GroupUsageRepository) *GroupUsageService {
	return &GroupUsageService{repo: repo}
}

// ReportCards counts the group's report cards (KindInvalid on a malformed id).
func (s *GroupUsageService) ReportCards(ctx context.Context, groupID string) (int64, error) {
	if err := uuid.Validate(groupID); err != nil {
		return 0, glerrors.New(glerrors.KindInvalid, "invalid group id")
	}
	return s.repo.CountGroupReportCards(ctx, groupID)
}
