package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

// Catalog corrections (T8, ADR 0036): a grade captured wrong is renamed or
// deleted without deleting its level, and the levels take a school order.
// Config judges every rule; the edge only refuses shapes that cannot be right.

// maxCatalogNameRunes bounds a grade or group name (the contract's maxLength).
const maxCatalogNameRunes = 80

// maxLevelsInOrder bounds a reorder (the contract's maxItems).
const maxLevelsInOrder = 50

// catalogName trims a grade or group name and reports whether its shape can
// reach config: not blank, at most maxCatalogNameRunes.
func catalogName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	return name, name != "" && utf8.RuneCountInString(name) <= maxCatalogNameRunes
}

type reorderLevelsRequest struct {
	LevelIDs []string `json:"levelIds"`
}

// ReorderLevels handles PUT /api/v1/config/levels/order with {levelIds}: the
// whole school order, first level first. A list that is not exactly the
// school's levels is config's to refuse (400 LEVEL_ORDER_MISMATCH).
func (h *ConfigHandler) ReorderLevels(w http.ResponseWriter, r *http.Request) {
	var req reorderLevelsRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.LevelIDs) == 0 || len(req.LevelIDs) > maxLevelsInOrder {
		http.Error(w, "levelIds must list the school's levels", http.StatusBadRequest)
		return
	}
	for _, id := range req.LevelIDs {
		if !uuidShape.MatchString(id) {
			http.Error(w, "levelIds must be level ids", http.StatusBadRequest)
			return
		}
	}
	res, err := h.svc.ReorderLevels(r.Context(), req.LevelIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]levelResponse, 0, len(res))
	for _, l := range res {
		out = append(out, toLevelResponse(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"levels": out})
}

type renameRequest struct {
	Name string `json:"name"`
}

// RenameGradeLevel handles PATCH /api/v1/config/grade-levels/{id} with {name}:
// the same grade (id, level, ordinal) under a new name. A name the level has
// is config's 409 GRADE_NAME_TAKEN.
func (h *ConfigHandler) RenameGradeLevel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a grade level id", http.StatusBadRequest)
		return
	}
	var req renameRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := catalogName(req.Name)
	if !ok {
		http.Error(w, "name is required (at most 80 characters)", http.StatusBadRequest)
		return
	}
	res, err := h.svc.RenameGradeLevel(r.Context(), id, name)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGradeLevelResponse(res))
}

// DeleteGradeLevel handles DELETE /api/v1/config/grade-levels/{id}. A grade
// that still holds groups, subjects or evaluation periods is config's 409
// GRADE_LEVEL_IN_USE, whose sentence names each with its count.
func (h *ConfigHandler) DeleteGradeLevel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a grade level id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteGradeLevel(r.Context(), id); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
