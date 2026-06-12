package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// iamPinger is the slice of the iam client the gateway needs. Defined on the
// consumer side (small interface) so the handler is testable with a fake.
type iamPinger interface {
	Ping(ctx context.Context) (client.IAMHealth, error)
}

// HealthHandler serves the gateway's own liveness plus an aggregate readiness
// probe that confirms the gateway can reach iam end to end. This aggregate is
// the Sprint-1 pipeline-validation artifact.
type HealthHandler struct {
	iam iamPinger
}

// NewHealthHandler wires the handler to an iam pinger.
func NewHealthHandler(iam iamPinger) *HealthHandler {
	return &HealthHandler{iam: iam}
}

// readyResponse is the aggregate readiness payload.
type readyResponse struct {
	Gateway string           `json:"gateway"`
	IAM     client.IAMHealth `json:"iam"`
}

// Register mounts the gateway health routes on mux.
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/readyz", h.readyz)
}

func (h *HealthHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readyz calls iam and reports the combined status, proving the gateway<->iam
// path works.
func (h *HealthHandler) readyz(w http.ResponseWriter, r *http.Request) {
	iamHealth, err := h.iam.Ping(r.Context())
	if err != nil {
		http.Error(w, "iam unreachable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(readyResponse{Gateway: "SERVING", IAM: iamHealth})
}
