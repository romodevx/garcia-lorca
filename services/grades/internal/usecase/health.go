package usecase

import "context"

// HealthStatus is the result of a liveness/readiness probe of the grades service.
type HealthStatus struct {
	Service string
	Status  string
}

// statusServing is reported when the service is up.
const statusServing = "SERVING"

// HealthService answers health/liveness checks. It is a trivial liveness
// reporter today; once readiness needs to check dependencies (DB, config,
// students) the logic lives here without changing the handler seam.
type HealthService struct {
	serviceName string
}

// NewHealthService builds a HealthService that identifies itself as name.
func NewHealthService(name string) *HealthService {
	return &HealthService{serviceName: name}
}

// Ping reports the service health. It never errors today; the error return lets
// readiness checks be added later without breaking callers.
func (h *HealthService) Ping(_ context.Context) (HealthStatus, error) {
	return HealthStatus{Service: h.serviceName, Status: statusServing}, nil
}
