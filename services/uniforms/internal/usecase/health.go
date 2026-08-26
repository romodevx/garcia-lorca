package usecase

import "context"

// HealthStatus is the result of a liveness probe of the uniforms service.
type HealthStatus struct {
	Service string
	Status  string
}

// statusServing is reported when the service is up.
const statusServing = "SERVING"

// HealthService answers liveness checks. Trivial today; when readiness has to
// check the database, finance and students, that logic lands here without moving
// the handler seam.
type HealthService struct {
	serviceName string
}

// NewHealthService builds a HealthService that identifies itself as name.
func NewHealthService(name string) *HealthService { return &HealthService{serviceName: name} }

// Ping reports the service health. The error return exists so readiness can be
// added later without breaking callers.
func (h *HealthService) Ping(_ context.Context) (HealthStatus, error) {
	return HealthStatus{Service: h.serviceName, Status: statusServing}, nil
}
