package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// ScholarshipRepository keeps the scholarships of T18 (ADR 0022) and the
// pupils who hold them. What a scholarship takes off a charge is not stored
// here: it is worked out when the charge is read or paid, and written on the
// charge only when the charge is settled under it (FreezeScholarships, and the
// deposit's DepositWrite.Scholarships).
type ScholarshipRepository interface {
	// CreateScholarship stores a new scholarship. A name already used
	// (case-insensitive) is KindConflict.
	CreateScholarship(ctx context.Context, s domain.Scholarship) (domain.Scholarship, error)
	// UpdateScholarship replaces its fields; KindNotFound when absent, a taken
	// name is KindConflict.
	UpdateScholarship(ctx context.Context, s domain.Scholarship) (domain.Scholarship, error)
	// GetScholarship reads one, or KindNotFound.
	GetScholarship(ctx context.Context, id string) (domain.Scholarship, error)
	// ListScholarships lists them by name; inactive ones only when asked.
	ListScholarships(ctx context.Context, includeInactive bool) ([]domain.Scholarship, error)
	// CountHolders is, per scholarship, how many pupils hold it now.
	CountHolders(ctx context.Context) (map[string]int, error)
	// ListHolders lists the open holdings of one scholarship, oldest first.
	ListHolders(ctx context.Context, scholarshipID string) ([]domain.ScholarshipHolding, error)
	// HeldBy is, per pupil, the scholarships they hold now (inactive ones
	// included: the rule ignores them). nil studentIDs means every pupil.
	HeldBy(ctx context.Context, studentIDs []string) (map[string][]domain.Scholarship, error)
	// Assign stores new open holdings and returns those written; a pupil who
	// already holds the scholarship is skipped, never duplicated.
	Assign(ctx context.Context, hs []domain.ScholarshipHolding) ([]domain.ScholarshipHolding, error)
	// Unassign ends the pupils' open holdings of a scholarship on `on` and
	// reports how many ended.
	Unassign(ctx context.Context, scholarshipID string, studentIDs []string, on time.Time) (int, error)
	// FreezeScholarships writes the scholarship part these charges carry
	// (settled under it), only on a charge not cancelled and with no part
	// written yet, re-deriving its status; it reports how many it wrote.
	FreezeScholarships(ctx context.Context, charges []domain.Charge) (int, error)
}
