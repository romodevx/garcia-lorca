package postgres

import (
	"context"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter serves the group records (T8, ADR 0036).
var _ ports.GroupRecordRepository = (*Repository)(nil)

type groupUsageRow struct {
	Enrollments int64
	Transfers   int64
}

// GroupUsage counts, tenant-bound, every enrollment ever in the group (any
// status) and every transfer out of or into it. Both columns are indexed per
// (tenant, group): 0002's idx_enrollments_group, 0015's to_group index, and
// the from side is the transfer's own enrollment's group history.
func (r *Repository) GroupUsage(ctx context.Context, groupID string) (ports.GroupUsage, error) {
	var u groupUsageRow
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT
				(SELECT count(*) FROM enrollments WHERE group_id = @g) AS enrollments,
				(SELECT count(*) FROM enrollment_transfers WHERE from_group_id = @g OR to_group_id = @g) AS transfers`,
			map[string]any{"g": groupID}).Scan(&u).Error
	})
	if err != nil {
		return ports.GroupUsage{}, glerrors.Wrap(glerrors.KindInternal, "counting the group's records", err)
	}
	return ports.GroupUsage{Enrollments: u.Enrollments, Transfers: u.Transfers}, nil
}

// RefreshGroupName writes name on the file of every pupil actively enrolled in
// the group — the label receipts, the card and the family app print — and
// answers how many files changed. A pupil who left keeps the label they left
// with; a file that already says name is not touched (a repeat changes 0).
func (r *Repository) RefreshGroupName(ctx context.Context, groupID, name string) (int64, error) {
	var changed int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE students SET grade_group = @name, updated_at = now()
			WHERE id IN (SELECT student_id FROM enrollments WHERE group_id = @g AND status = @active)
			  AND grade_group IS DISTINCT FROM @name`,
			map[string]any{"name": name, "g": groupID, "active": domain.EnrollmentStatusActive.String()})
		changed = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "relabelling the group on the files", err)
	}
	return changed, nil
}
