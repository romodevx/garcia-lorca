package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// ActivityRepository keeps the activities of T17, their price options and the
// pupils enrolled in them. Their charges are ordinary charges written through
// FinanceRepository; this seam never touches the ledger.
type ActivityRepository interface {
	// CreateActivity stores a new activity with its options. A name already
	// used (case-insensitive) is KindConflict.
	CreateActivity(ctx context.Context, a domain.Activity) (domain.Activity, error)
	// UpdateActivity replaces the activity's fields and live options, in one
	// transaction: options present are inserted or re-priced, options absent
	// are retired. Retiring one an open enrolment still uses is KindConflict;
	// a taken name is KindConflict; no activity is KindNotFound.
	UpdateActivity(ctx context.Context, a domain.Activity) (domain.Activity, error)
	// GetActivity reads one with its live options, or KindNotFound.
	GetActivity(ctx context.Context, id string) (domain.Activity, error)
	// ListActivities lists them by name; inactive ones only when asked.
	ListActivities(ctx context.Context, includeInactive bool) ([]domain.Activity, error)
	// CountActiveEnrollments is, per activity, how many pupils are enrolled
	// on that day.
	CountActiveEnrollments(ctx context.Context, on time.Time) (map[string]int, error)
	// ListActivityEnrollments lists one activity's enrolments, ended ones
	// included, oldest first.
	ListActivityEnrollments(ctx context.Context, activityID string) ([]domain.ActivityEnrollment, error)
	// ListBillableEnrollments lists every enrolment of every ACTIVE activity,
	// ended ones included (a period billed before the end may still be owed
	// when the run catches up).
	ListBillableEnrollments(ctx context.Context) ([]domain.ActivityEnrollment, error)
	// CreateEnrollments stores new open enrolments and returns those written;
	// a pupil who already has an open one is skipped, never duplicated.
	CreateEnrollments(ctx context.Context, es []domain.ActivityEnrollment) ([]domain.ActivityEnrollment, error)
	// EndEnrollments stops the pupils' enrolments in an activity on `on` (when
	// open, or ending later) and reports how many it ended.
	EndEnrollments(ctx context.Context, activityID string, studentIDs []string, on time.Time) (int, error)
	// ChangeEnrollmentOption moves a pupil's open enrolment to another option
	// of the same activity; no open enrolment is KindNotFound.
	ChangeEnrollmentOption(ctx context.Context, activityID, studentID, optionID string, on time.Time) (domain.ActivityEnrollment, error)
}
