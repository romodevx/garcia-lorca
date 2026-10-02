package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/config/internal/domain"
)

// ScheduleCatalog is everything the timetable of a cycle is drawn and named
// with: the cycle, the catalog (levels, grades, the cycle's groups, their
// grades' subjects, every teacher, the cycle's assignments) and each level's
// SAVED grid (a level with none uses domain.DefaultScheduleGrid).
type ScheduleCatalog struct {
	Cycle       domain.Cycle
	Levels      []domain.Level
	Grades      []domain.GradeLevel
	Groups      []domain.Group
	Subjects    []domain.Subject
	Teachers    []domain.Teacher
	Assignments []domain.TeachingAssignment
	Grids       []domain.ScheduleGrid
}

// ScheduleRepository loads and persists the school timetable (T16). Every
// method is tenant-scoped (RLS) like CatalogRepository.
//
//   - GetScheduleBlock / GetScheduleOverride: KindNotFound when absent.
//   - LoadScheduleCatalog: KindNotFound when the cycle does not exist.
//   - ListScheduleBlocks / ListScheduleOverrides read outside any lock (the
//     screen's reads); ListScheduleOverrides returns the cycle's changes with
//     from <= date <= to.
//   - RunScheduleTx runs fn in ONE transaction holding the cycle's timetable
//     lock (a transaction-scoped advisory lock): every timetable write of the
//     cycle is serialised, so the checks fn makes on the effective timetable
//     of a date cannot race another writer. An EXCLUDE refusal or a unique key
//     the database trips anyway is domain.ScheduleChangedMeanwhile.
type ScheduleRepository interface {
	GetCycle(ctx context.Context, id string) (domain.Cycle, error)
	ListCycles(ctx context.Context) ([]domain.Cycle, error)
	GetLevel(ctx context.Context, id string) (domain.Level, error)
	GetTeachingAssignment(ctx context.Context, id string) (domain.TeachingAssignment, error)
	GetScheduleBlock(ctx context.Context, id string) (domain.ScheduleBlock, error)
	GetScheduleOverride(ctx context.Context, id string) (domain.ScheduleOverride, error)
	LoadScheduleCatalog(ctx context.Context, cycleID string) (ScheduleCatalog, error)
	ListScheduleBlocks(ctx context.Context, cycleID string) ([]domain.ScheduleBlock, error)
	ListScheduleOverrides(ctx context.Context, cycleID string, from, to time.Time) ([]domain.ScheduleOverride, error)
	ListScheduleEvents(ctx context.Context, cycleID string, limit int) ([]domain.ScheduleEvent, error)
	SaveScheduleGrid(ctx context.Context, g domain.ScheduleGrid) error
	RunScheduleTx(ctx context.Context, cycleID string, fn func(ScheduleTx) error) error
}

// ScheduleTx is the timetable seen from inside RunScheduleTx.
//
//   - GetBlock / GetOverride: KindNotFound when absent.
//   - OverridesFrom returns the cycle's changes dated on or after from.
//   - DeleteBlock removes the class and (FK cascade) its changes.
type ScheduleTx interface {
	Blocks(cycleID string) ([]domain.ScheduleBlock, error)
	OverridesFrom(cycleID string, from time.Time) ([]domain.ScheduleOverride, error)
	GetBlock(id string) (domain.ScheduleBlock, error)
	GetOverride(id string) (domain.ScheduleOverride, error)
	InsertBlock(b domain.ScheduleBlock) error
	UpdateBlock(b domain.ScheduleBlock) error
	DeleteBlock(id string) error
	InsertOverride(o domain.ScheduleOverride) error
	DeleteOverride(id string) error
	AppendEvent(e domain.ScheduleEvent) error
}
