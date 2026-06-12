package grpcadapter

import (
	"encoding/json"
	"net/http"

	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// PingResponse is the wire shape of a health Ping.
//
// FASE 0 NOTE: this is a hand-written JSON DTO so the gateway<->iam pipeline can
// be validated before `buf generate` exists. When the iam proto contract is
// generated (garcialorca.iam.v1), this handler is replaced by the generated
// Connect HealthService handler and this DTO goes away. The use-case seam
// (usecase.HealthService) stays unchanged.
type PingResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// HealthHandler adapts usecase.HealthService to HTTP/JSON.
type HealthHandler struct {
	svc *usecase.HealthService
}

// NewHealthHandler wires the handler to a HealthService.
func NewHealthHandler(svc *usecase.HealthService) *HealthHandler {
	return &HealthHandler{svc: svc}
}

// PingPath is the route the gateway client calls.
const PingPath = "/garcialorca.iam.v1.HealthService/Ping"

// Register mounts the iam health routes on mux.
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc(PingPath, h.ping)
}

func (h *HealthHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *HealthHandler) ping(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Ping(r.Context())
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PingResponse{Service: status.Service, Status: status.Status})
}
