package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// LevelTermsReader is the catalog a level-scoped terms read needs: the groups
// and grades that place a group in its level, and the terms of a cycle+level.
type LevelTermsReader interface {
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	ListTerms(ctx context.Context, cycleID, levelID string) ([]client.TermResult, error)
}

// cycleLevel is one (cycle, level) pair whose terms a picker may offer.
type cycleLevel struct{ cycleID, levelID string }

// levelTerms lists the terms of each DISTINCT (cycle, group level) the
// assignments span, deduped by term id. The group→level map is built from two
// list reads, never one read per assignment. Best-effort by design: a failed
// catalog read, an unresolvable group or a failed terms read contributes
// nothing — it never widens to an unfiltered (every level) read. The result is
// never nil.
func levelTerms(ctx context.Context, catalog LevelTermsReader, assignments []client.TeachingAssignment) []teachingTermResponse {
	out := make([]teachingTermResponse, 0, len(assignments))
	if len(assignments) == 0 {
		return out
	}
	levelOf := groupLevels(ctx, catalog)
	seenPair := map[cycleLevel]bool{}
	seenTerm := map[string]bool{}
	for _, a := range assignments {
		pair := cycleLevel{cycleID: a.CycleID, levelID: levelOf[a.GroupID]}
		if pair.cycleID == "" || pair.levelID == "" || seenPair[pair] {
			continue
		}
		seenPair[pair] = true
		terms, err := catalog.ListTerms(ctx, pair.cycleID, pair.levelID)
		if err != nil {
			continue
		}
		for _, t := range terms {
			// Defense in depth: keep only the pair's own level even if the
			// catalog ignored the filter.
			if t.LevelID != pair.levelID || seenTerm[t.ID] {
				continue
			}
			seenTerm[t.ID] = true
			out = append(out, teachingTermResponse{ID: t.ID, Name: t.Name, CycleID: t.CycleID, LevelID: t.LevelID})
		}
	}
	return out
}

// groupLevels maps group id → level id from the tenant's groups and grades.
// A failed read yields an empty (never nil) map.
func groupLevels(ctx context.Context, catalog LevelTermsReader) map[string]string {
	out := map[string]string{}
	groups, err := catalog.ListGroups(ctx, "", "")
	if err != nil {
		return out
	}
	grades, err := catalog.ListGradeLevels(ctx, "")
	if err != nil {
		return out
	}
	levelOfGrade := make(map[string]string, len(grades))
	for _, g := range grades {
		levelOfGrade[g.ID] = g.LevelID
	}
	for _, g := range groups {
		out[g.ID] = levelOfGrade[g.GradeLevelID]
	}
	return out
}

// AssignmentTermsReader is what the director's terms read consumes: one
// assignment by id plus the level-scoped catalog. The Connect ConfigClient
// satisfies it.
type AssignmentTermsReader interface {
	GetTeachingAssignment(ctx context.Context, id string) (client.TeachingAssignment, error)
	LevelTermsReader
}

// GradesTermsHandler serves the director's term picker for the legacy capture.
type GradesTermsHandler struct {
	config AssignmentTermsReader
}

// NewGradesTermsHandler wires the handler to the config catalog.
func NewGradesTermsHandler(config AssignmentTermsReader) *GradesTermsHandler {
	return &GradesTermsHandler{config: config}
}

// Terms handles GET /api/v1/grades/terms?assignmentId= — the terms of the
// assignment's cycle and of its group's level, so the director's picker can
// only offer terms the legacy capture accepts. assignmentId is required (400);
// an unknown one is 404.
//
// It runs behind RequireAuth + rotation, the module_grades + module_config
// guards and RequirePermission("grades:read").
func (h *GradesTermsHandler) Terms(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("assignmentId")
	if id == "" {
		writeBadRequest(w, "assignmentId es obligatorio")
		return
	}
	a, err := h.config.GetTeachingAssignment(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": levelTerms(r.Context(), h.config, []client.TeachingAssignment{a})})
}
