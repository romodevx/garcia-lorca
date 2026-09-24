package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// LevelLister lists the school's levels from config.
type LevelLister interface {
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
}

// UniformLevelsHandler serves the school's levels to the inventory module.
//
// Garments are scoped by config level (ADR 0014), so the inventory's level
// chips need the school's list. Reading it from /api/v1/config/levels would
// demand config:read and module_config of a storeroom role that has no other
// business in Configuración; this route sits behind the uniforms guards
// instead, and answers only what a chip shows — an id and a name.
type UniformLevelsHandler struct {
	levels LevelLister
}

// NewUniformLevelsHandler wires the handler to config's level catalog.
func NewUniformLevelsHandler(levels LevelLister) *UniformLevelsHandler {
	return &UniformLevelsHandler{levels: levels}
}

type uniformLevelResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// List: GET /api/v1/uniforms/levels
func (h *UniformLevelsHandler) List(w http.ResponseWriter, r *http.Request) {
	levels, err := h.levels.ListLevels(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]uniformLevelResponse, 0, len(levels))
	for _, l := range levels {
		out = append(out, uniformLevelResponse{ID: l.ID, Name: l.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"levels": out})
}
