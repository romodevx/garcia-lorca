package postgres

import (
	"context"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the repository serves the group usage (T8, ADR 0036).
var _ ports.GroupUsageRepository = (*Repository)(nil)

// CountGroupReportCards counts, tenant-bound, every report card issued for
// the group, whatever its status.
func (r *Repository) CountGroupReportCards(ctx context.Context, groupID string) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM report_cards WHERE group_id = ?", groupID).Scan(&n).Error
	})
	if err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "counting the group's report cards", err)
	}
	return n, nil
}
