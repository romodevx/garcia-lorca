package repo

import (
	"context"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the repository serves the group usage (T8, ADR 0036).
var _ service.GroupUsageRepo = (*Repo)(nil)

type groupUsageRow struct {
	Sessions int64
	Marks    int64
}

// GroupUsage counts, tenant-bound, the pass lists taken in the group and the
// marks stamped with it.
func (r *Repo) GroupUsage(ctx context.Context, groupID string) (service.GroupUsage, error) {
	var u groupUsageRow
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT
				(SELECT count(*) FROM attendance_sessions WHERE group_id = @g) AS sessions,
				(SELECT count(*) FROM attendance WHERE group_id = @g)          AS marks`,
			map[string]any{"g": groupID}).Scan(&u).Error
	})
	if err != nil {
		return service.GroupUsage{}, glerrors.Wrap(glerrors.KindInternal, "counting the group's attendance", err)
	}
	return service.GroupUsage{Sessions: u.Sessions, Marks: u.Marks}, nil
}
