package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the import-issue port.
var _ ports.ImportIssueRepository = (*Repository)(nil)

// openFirst orders a pupil's issues: open ones first, then by creation.
const openFirst = "resolved_at IS NOT NULL, created_at, id"

// ListImportIssues returns the pupil's import issues, open first (resolved
// ones only with includeResolved). RLS keeps another tenant's out.
func (r *Repository) ListImportIssues(ctx context.Context, studentID string, includeResolved bool) ([]domain.ImportIssue, error) {
	var out []domain.ImportIssue
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Where("student_id = ?", studentID).Order(openFirst)
		if !includeResolved {
			q = q.Where("resolved_at IS NULL")
		}
		var rows []importIssueModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing import issues", err)
		}
		out = make([]domain.ImportIssue, 0, len(rows))
		for _, m := range rows {
			issue, err := importIssueToDomain(m)
			if err != nil {
				return err
			}
			out = append(out, issue)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ImportIssueByID returns one issue of the pupil; another pupil's or tenant's
// is KindNotFound.
func (r *Repository) ImportIssueByID(ctx context.Context, studentID, issueID string) (domain.ImportIssue, error) {
	var out domain.ImportIssue
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = issueByID(tx, studentID, issueID)
		return err
	})
	return out, err
}

func issueByID(tx *gorm.DB, studentID, issueID string) (domain.ImportIssue, error) {
	var m importIssueModel
	err := tx.Where("id = ? AND student_id = ?", issueID, studentID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ImportIssue{}, glerrors.ErrNotFound
	}
	if err != nil {
		return domain.ImportIssue{}, glerrors.Wrap(glerrors.KindInternal, "reading an import issue", err)
	}
	return importIssueToDomain(m)
}

// ResolveImportIssue stores the issue's resolution only while it is still open
// (compare-and-set on resolved_at), then answers the issue as stored: when
// another writer resolved it first, theirs is the record.
func (r *Repository) ResolveImportIssue(ctx context.Context, issue domain.ImportIssue) (domain.ImportIssue, error) {
	var out domain.ImportIssue
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&importIssueModel{}).
			Where("id = ? AND student_id = ? AND resolved_at IS NULL", issue.ID(), issue.StudentID()).
			Updates(map[string]any{
				"resolved_at": issue.ResolvedAt(),
				"resolved_by": nullableID(issue.ResolvedBy()),
				"resolution":  string(issue.Resolution()),
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "resolving an import issue", res.Error)
		}
		var err error
		out, err = issueByID(tx, issue.StudentID(), issue.ID())
		return err
	})
	return out, err
}
