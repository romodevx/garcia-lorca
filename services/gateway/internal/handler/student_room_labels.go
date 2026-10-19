package handler

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// RoomLabelSource names each pupil's classroom for the current cycle, by pupil
// id: "3° PRIMARIA · B" (CC-211, CC-313). A pupil it cannot place is absent.
type RoomLabelSource interface {
	RoomLabels(ctx context.Context) (map[string]string, error)
}

// RoomCatalogReader is the config catalog a classroom label needs: the cycles
// (to find the current one) and that cycle's groups with their grades and
// levels. The Connect ConfigClient satisfies it.
type RoomCatalogReader interface {
	ListCycles(ctx context.Context) ([]client.CycleResult, error)
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
}

// CycleEnrollmentReader lists one cycle's enrollments. The Connect
// StudentsClient satisfies it.
type CycleEnrollmentReader interface {
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
}

// namingStatuses are the enrollments that name a pupil's classroom, in order:
// the active one, else the one a withdrawn pupil left. A transfer's old
// enrollment never does — the pupil is in the new group now.
var namingStatuses = [...]string{"active", "withdrawn"}

// CycleRoomLabels resolves the classroom labels of a whole cycle in five reads
// (cycles, then groups, grades, levels and enrollments in parallel) and joins
// them in memory — O(groups + grades + enrollments), never one read per pupil.
//
// It exists so a role with students:read but without config:read (Cobranza)
// sees the same "3° PRIMARIA · B" as the director, without being handed the
// catalog itself: the gateway reads config as the edge, and only the label
// leaves it. It mirrors the web's roomLabelsOf, so both name a pupil alike.
type CycleRoomLabels struct {
	catalog     RoomCatalogReader
	enrollments CycleEnrollmentReader
}

// NewCycleRoomLabels wires the labels source.
func NewCycleRoomLabels(catalog RoomCatalogReader, enrollments CycleEnrollmentReader) CycleRoomLabels {
	return CycleRoomLabels{catalog: catalog, enrollments: enrollments}
}

// RoomLabels is every placed pupil's classroom label in the current cycle: the
// active one, or the first when none is active (as the web picks it). No cycle
// at all is no labels, not an error.
func (l CycleRoomLabels) RoomLabels(ctx context.Context) (map[string]string, error) {
	cycles, err := l.catalog.ListCycles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing the cycles: %w", err)
	}
	cycleID := currentCycleID(cycles)
	if cycleID == "" {
		return map[string]string{}, nil
	}

	var (
		groups      []client.GroupResult
		grades      []client.GradeLevelResult
		levels      []client.LevelResult
		enrollments []client.Enrollment
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		if groups, err = l.catalog.ListGroups(gctx, cycleID, ""); err != nil {
			return fmt.Errorf("listing the groups of cycle %s: %w", cycleID, err)
		}
		return nil
	})
	g.Go(func() (err error) {
		if grades, err = l.catalog.ListGradeLevels(gctx, ""); err != nil {
			return fmt.Errorf("listing the grades: %w", err)
		}
		return nil
	})
	g.Go(func() (err error) {
		if levels, err = l.catalog.ListLevels(gctx); err != nil {
			return fmt.Errorf("listing the levels: %w", err)
		}
		return nil
	})
	g.Go(func() (err error) {
		if enrollments, err = l.enrollments.ListEnrollments(gctx, "", cycleID, ""); err != nil {
			return fmt.Errorf("listing the enrollments of cycle %s: %w", cycleID, err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return labelsByStudent(enrollments, groupLabels(groups, grades, levels)), nil
}

// currentCycleID is the active cycle, else the first listed; "" when none.
func currentCycleID(cycles []client.CycleResult) string {
	for _, c := range cycles {
		if c.Status == "active" {
			return c.ID
		}
	}
	if len(cycles) == 0 {
		return ""
	}
	return cycles[0].ID
}

// groupLabels names each group whose grade the catalog still holds, the way
// the pass lists and receipts do (gradeWithLevel + groupLabel).
func groupLabels(groups []client.GroupResult, grades []client.GradeLevelResult, levels []client.LevelResult) map[string]string {
	levelName := make(map[string]string, len(levels))
	for _, lv := range levels {
		levelName[lv.ID] = lv.Name
	}
	gradeName := make(map[string]string, len(grades))
	for _, gr := range grades {
		gradeName[gr.ID] = gradeWithLevel(gr.Name, levelName[gr.LevelID])
	}
	out := make(map[string]string, len(groups))
	for _, gp := range groups {
		if name, ok := gradeName[gp.GradeLevelID]; ok && name != "" {
			out[gp.ID] = groupLabel(name, gp.Name)
		}
	}
	return out
}

// labelsByStudent picks, per pupil, the label of the first naming status that
// places them in a known group.
func labelsByStudent(enrollments []client.Enrollment, labelOf map[string]string) map[string]string {
	out := make(map[string]string)
	for _, status := range namingStatuses {
		for _, e := range enrollments {
			if e.Status != status {
				continue
			}
			if _, placed := out[e.StudentID]; placed {
				continue
			}
			if label, ok := labelOf[e.GroupID]; ok {
				out[e.StudentID] = label
			}
		}
	}
	return out
}
