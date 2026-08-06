package usecase

import "context"

// HealthStatus is the result of a liveness/readiness probe of the config service.
type HealthStatus struct {
	Service string
	Status  string
}

// statusServing is reported when the service is up.
const statusServing = "SERVING"

// HealthService answers Ping/health checks. It is a trivial liveness reporter
// today; once config owns real dependencies (DB) readiness will check them here.
type HealthService struct {
	serviceName string
}

// NewHealthService builds a HealthService that identifies itself as name.
func NewHealthService(name string) *HealthService {
	return &HealthService{serviceName: name}
}

// Ping reports the service health. It never errors today; the signature returns
// an error so readiness checks (DB pings, etc.) can be added without breaking
// callers.
func (h *HealthService) Ping(_ context.Context) (HealthStatus, error) {
	return HealthStatus{Service: h.serviceName, Status: statusServing}, nil
}
