package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// ImportIssueService reads and accepts the import issues of a pupil
// (PLAN-import-avisos): what the legacy import could not keep as the file
// wrote it. Correcting an issue is not here — saving the file does it
// (StudentService.UpdateDetails). Permissions and the module flag are the
// gateway's.
type ImportIssueService struct {
	issues   ports.ImportIssueRepository
	students ports.StudentRepository
	now      func() time.Time
}

// NewImportIssueService wires the use case; `now` stamps resolutions.
func NewImportIssueService(issues ports.ImportIssueRepository, students ports.StudentRepository, now func() time.Time) *ImportIssueService {
	return &ImportIssueService{issues: issues, students: students, now: now}
}

// ListImportIssues returns the pupil's issues (resolved ones too with
// includeResolved). An unknown pupil is KindNotFound.
func (s *ImportIssueService) ListImportIssues(ctx context.Context, studentID string, includeResolved bool) ([]domain.ImportIssue, error) {
	if _, err := s.students.FindByID(ctx, studentID); err != nil {
		return nil, err
	}
	return s.issues.ListImportIssues(ctx, studentID, includeResolved)
}

// AcceptImportIssue closes an open issue as fine as stored, by userID. An
// issue already resolved is answered as it stands, with nothing written. A
// draft's curp issue is refused (domain.ErrDraftNotPlaceable): only saving a
// valid CURP closes it; a withdrawn pupil's issues are domain.ErrWithdrawnReadOnly.
// Another pupil's issue — or an id that is not one — is
// KindNotFound, so the answer never confirms an issue exists elsewhere.
func (s *ImportIssueService) AcceptImportIssue(ctx context.Context, studentID, issueID, userID string) (domain.ImportIssue, error) {
	if uuid.Validate(userID) != nil {
		return domain.ImportIssue{}, glerrors.New(glerrors.KindInvalid, "the caller is not a user id")
	}
	if uuid.Validate(issueID) != nil {
		return domain.ImportIssue{}, glerrors.ErrNotFound
	}
	student, err := s.students.FindByID(ctx, studentID)
	if err != nil {
		return domain.ImportIssue{}, err
	}
	// A withdrawn pupil's file is read-only (T14), its issues included.
	if err := student.RequireEditable(); err != nil {
		return domain.ImportIssue{}, err
	}
	issue, err := s.issues.ImportIssueByID(ctx, studentID, issueID)
	if err != nil {
		return domain.ImportIssue{}, err
	}
	if !issue.IsOpen() {
		return issue, nil
	}
	if issue.ClosesOnlyByCorrection() && student.IsDraft() {
		return domain.ImportIssue{}, domain.ErrDraftNotPlaceable
	}
	return s.issues.ResolveImportIssue(ctx, issue.Accept(userID, s.now()))
}
