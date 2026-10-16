package usecase

import (
	"context"
	"strings"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// msgGroupNameBlank is the refusal for relabelling files with an empty name.
const msgGroupNameBlank = "El nombre del grupo no puede quedar vacío."

// GroupRecordService answers for students' records of a config group when the
// group itself is corrected (T8, ADR 0036): deleting it needs to know whether
// any pupil was ever placed there, and renaming it must reach the label the
// pupils' files carry (receipts, card, family app print it).
type GroupRecordService struct {
	repo ports.GroupRecordRepository
}

// NewGroupRecordService wires the use case.
func NewGroupRecordService(repo ports.GroupRecordRepository) *GroupRecordService {
	return &GroupRecordService{repo: repo}
}

// GroupUsage counts the enrollments and transfers of groupID.
func (s *GroupRecordService) GroupUsage(ctx context.Context, groupID string) (ports.GroupUsage, error) {
	if err := uuid.Validate(groupID); err != nil {
		return ports.GroupUsage{}, glerrors.New(glerrors.KindInvalid, "invalid group id")
	}
	return s.repo.GroupUsage(ctx, groupID)
}

// RefreshGroupName writes the group's new (trimmed) name on the files of the
// pupils actively enrolled in it, answering how many changed.
func (s *GroupRecordService) RefreshGroupName(ctx context.Context, groupID, name string) (int64, error) {
	if err := uuid.Validate(groupID); err != nil {
		return 0, glerrors.New(glerrors.KindInvalid, "invalid group id")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, glerrors.New(glerrors.KindInvalid, msgGroupNameBlank)
	}
	return s.repo.RefreshGroupName(ctx, groupID, name)
}
