package handler

import (
	"context"
	"log/slog"
	"net/http"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// --- families from the import (PLAN-familias-importacion, ADR 0041) -----------
//
// After a commit writes its pupils, the gateway asks students to build the
// batch's very likely families. It is best effort: a failure never fails the
// import, it only leaves those groups as suggestions. Building a family hands
// its app accounts to every pupil in it, so it runs only for a caller holding
// guardians:write, like every other membership write (ADR 0041 Permisos).

// ImportFamilies is students' side of an import's families.
type ImportFamilies interface {
	BuildFamiliesFromImport(ctx context.Context, batchID string) (client.FamilyImportBuild, error)
	UndoFamiliesFromImport(ctx context.Context, batchID string) (client.FamilyImportUndo, error)
	SuggestFamilies(ctx context.Context) ([]client.FamilySuggestion, error)
}

const (
	permGuardiansWrite = "guardians:write"
	permStudentsRead   = "students:read"
)

// importFamilies is the commit's `families` object.
type importFamilies struct {
	// Built is false when students failed partway: Created and Joined then
	// hold what was done before the failure.
	Built   bool           `json:"built"`
	Created []importFamily `json:"created"`
	Joined  []importFamily `json:"joined"`
	Skipped int            `json:"skipped"`
	// ToReview is how many suggestions the office still has to look at;
	// absent when they could not be read.
	ToReview *int `json:"toReview,omitempty"`
}

// importFamily names a family and its pupils only for students:read.
type importFamily struct {
	ID         string   `json:"id"`
	Name       string   `json:"name,omitempty"`
	StudentIDs []string `json:"studentIds,omitempty"`
}

type importFamiliesUndo struct {
	Removed        int `json:"removed"`
	Dissolved      int `json:"dissolved"`
	KeptWithAccess int `json:"keptWithAccess"`
}

// buildFamilies builds the batch's families for who, or answers nil when the
// module is off or who may not write family membership. ctx is the commit's
// detached context.
func (h *StudentImportHandler) buildFamilies(ctx context.Context, who middleware.Principal, batch string) *importFamilies {
	if !h.familiesOn(ctx) || !middleware.HasPermission(who, permGuardiansWrite) {
		return nil
	}
	built, err := h.d.Families.BuildFamiliesFromImport(ctx, batch)
	switch {
	case err != nil:
		// The batch and the code only: students' sentence may name a pupil.
		slog.WarnContext(ctx, "student import: families not built", "batch", batch, "code", connect.CodeOf(err).String())
	case built.Incomplete:
		slog.WarnContext(ctx, "student import: families built partway", "batch", batch)
	}
	readsPupils := middleware.HasPermission(who, permStudentsRead)
	out := &importFamilies{
		Built:   err == nil && !built.Incomplete,
		Created: toImportFamilies(built.Created, readsPupils),
		Joined:  toImportFamilies(built.Joined, readsPupils),
		Skipped: built.Skipped,
	}
	if suggestions, err := h.d.Families.SuggestFamilies(ctx); err == nil {
		n := len(suggestions)
		out.ToReview = &n
	} else {
		slog.WarnContext(ctx, "student import: families to review not counted", "batch", batch, "code", connect.CodeOf(err).String())
	}
	return out
}

func (h *StudentImportHandler) familiesOn(ctx context.Context) bool {
	return h.d.Families != nil && h.d.FamiliesEnabled != nil && h.d.FamiliesEnabled(ctx)
}

func toImportFamilies(families []client.Family, readsPupils bool) []importFamily {
	out := make([]importFamily, 0, len(families))
	for _, f := range families {
		fam := importFamily{ID: f.ID}
		if readsPupils {
			fam.Name, fam.StudentIDs = f.Name, f.StudentIDs
		}
		out = append(out, fam)
	}
	return out
}

// UndoFamilies: POST /api/v1/imports/students/{batchId}/families/undo — takes
// back the families the import built. Idempotent. The route sits behind
// module_students and module_families, students:import and guardians:write.
func (h *StudentImportHandler) UndoFamilies(w http.ResponseWriter, r *http.Request) {
	who, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	batch, ok := pathUUID(w, r, "batchId")
	if !ok {
		return
	}
	undo, err := h.d.Families.UndoFamiliesFromImport(r.Context(), batch)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	slog.InfoContext(r.Context(), "student import families undone", "batch", batch, "by", who.UserID,
		"removed", undo.Removed, "dissolved", undo.Dissolved, "kept_with_access", undo.KeptWithAccess)
	writeJSON(w, http.StatusOK, importFamiliesUndo{
		Removed: undo.Removed, Dissolved: undo.Dissolved, KeptWithAccess: undo.KeptWithAccess,
	})
}
