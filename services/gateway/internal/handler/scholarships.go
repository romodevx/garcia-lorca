package handler

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Edge bounds of the Becas module (T18), mirroring finance's domain.
const (
	maxScholarshipName    = 120
	maxScholarshipTargets = 50
	maxScholarshipBatch   = 200
	fullScholarshipBP     = 10_000
	maxScholarshipCents   = 1_000_000_000_000_000
)

// scholarshipTargetShape is one of the four targets the contract allows.
var scholarshipTargetShape = regexp.MustCompile(
	`^(enrollment|tuition|(concept|activity):[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// ScholarshipService is the finance surface the Becas module needs
// (consumer-defined; the Connect FinanceClient implements it).
type ScholarshipService interface {
	ListScholarships(ctx context.Context, includeInactive bool) ([]client.Scholarship, error)
	GetScholarship(ctx context.Context, id string) (client.Scholarship, error)
	CreateScholarship(ctx context.Context, in client.ScholarshipInput) (client.Scholarship, error)
	UpdateScholarship(ctx context.Context, id string, in client.ScholarshipInput) (client.Scholarship, error)
	ListScholarshipTargets(ctx context.Context) ([]client.ScholarshipTarget, error)
	ListScholarshipHolders(ctx context.Context, id string) ([]client.ScholarshipHolder, error)
	AssignScholarship(ctx context.Context, id string, studentIDs []string) ([]client.ScholarshipHolder, error)
	UnassignScholarship(ctx context.Context, id string, studentIDs []string) (int32, error)
}

// ScholarshipsHandler serves the Becas module (T18): the scholarships, what
// they can apply to and who holds them. Every route runs behind module_finance
// AND module_scholarships plus scholarships:read / scholarships:write
// (cmd/main.go). What a scholarship takes off a charge is finance's to work
// out, on every read and payment.
type ScholarshipsHandler struct {
	svc ScholarshipService
}

// NewScholarshipsHandler wires the handler.
func NewScholarshipsHandler(svc ScholarshipService) *ScholarshipsHandler {
	return &ScholarshipsHandler{svc: svc}
}

// scholarshipIDOf is the path's id when it has an id's shape; otherwise 404.
func scholarshipIDOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "scholarship not found", http.StatusNotFound)
		return "", false
	}
	return id, true
}

// scholarshipRequest is the body of create and update. Active is a pointer so
// a body that leaves it out means "active".
type scholarshipRequest struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Value   int64    `json:"value"`
	Targets []string `json:"targets"`
	Active  *bool    `json:"active"`
}

// invalid is the edge check against the contract; finance re-validates and
// checks that every concept and activity named exists.
func (req scholarshipRequest) invalid() string {
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "":
		return "name is required"
	case tooLong(name, maxScholarshipName):
		return "name is at most 120 characters"
	case len(req.Targets) == 0 || len(req.Targets) > maxScholarshipTargets:
		return "targets needs 1 to 50 entries"
	}
	if msg := invalidScholarshipValue(req.Kind, req.Value); msg != "" {
		return msg
	}
	for i, t := range req.Targets {
		if !scholarshipTargetShape.MatchString(t) {
			return "each target is enrollment, tuition, concept:<uuid> or activity:<uuid>"
		}
		if slices.Contains(req.Targets[:i], t) {
			return "a target appears twice"
		}
	}
	return ""
}

// invalidScholarshipValue checks a value against its kind.
func invalidScholarshipValue(kind string, value int64) string {
	switch kind {
	case "percent":
		if value < 1 || value > fullScholarshipBP {
			return "a percent is 1 to 10000 basis points"
		}
	case "fixed_off":
		if value < 1 || value > maxScholarshipCents {
			return "an amount off is at least 1 cent"
		}
	case "fixed_price":
		if value < 0 || value > maxScholarshipCents {
			return "an amount to pay cannot be negative"
		}
	default:
		return "kind must be percent, fixed_off or fixed_price"
	}
	return ""
}

func (req scholarshipRequest) input() client.ScholarshipInput {
	return client.ScholarshipInput{
		Name: strings.TrimSpace(req.Name), Kind: req.Kind, Value: req.Value, Targets: req.Targets,
		Active: req.Active == nil || *req.Active,
	}
}

// decodeScholarship reads and checks a create/update body, answering 400 itself.
func decodeScholarship(w http.ResponseWriter, r *http.Request) (client.ScholarshipInput, bool) {
	var req scholarshipRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return client.ScholarshipInput{}, false
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return client.ScholarshipInput{}, false
	}
	return req.input(), true
}

// decodeStudentBatch reads {studentIds} (1–200 uuids), answering 400 itself.
func decodeStudentBatch(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var req struct {
		StudentIDs []string `json:"studentIds"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return nil, false
	}
	if len(req.StudentIDs) == 0 || len(req.StudentIDs) > maxScholarshipBatch {
		http.Error(w, "studentIds needs 1 to 200 pupils", http.StatusBadRequest)
		return nil, false
	}
	for _, s := range req.StudentIDs {
		if !uuidShape.MatchString(s) {
			http.Error(w, "studentIds must be uuids", http.StatusBadRequest)
			return nil, false
		}
	}
	return req.StudentIDs, true
}

// List handles GET /api/v1/scholarships[?includeInactive=true].
func (h *ScholarshipsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListScholarships(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.Scholarship{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scholarships": rows})
}

// Get handles GET /api/v1/scholarships/{id}.
func (h *ScholarshipsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := scholarshipIDOf(w, r)
	if !ok {
		return
	}
	s, err := h.svc.GetScholarship(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// Create handles POST /api/v1/scholarships.
func (h *ScholarshipsHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeScholarship(w, r)
	if !ok {
		return
	}
	s, err := h.svc.CreateScholarship(r.Context(), in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// Update handles PUT /api/v1/scholarships/{id}.
func (h *ScholarshipsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := scholarshipIDOf(w, r)
	if !ok {
		return
	}
	in, ok := decodeScholarship(w, r)
	if !ok {
		return
	}
	s, err := h.svc.UpdateScholarship(r.Context(), id, in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// Targets handles GET /api/v1/scholarships/targets.
func (h *ScholarshipsHandler) Targets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListScholarshipTargets(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.ScholarshipTarget{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": rows})
}

// Holders handles GET /api/v1/scholarships/{id}/students.
func (h *ScholarshipsHandler) Holders(w http.ResponseWriter, r *http.Request) {
	id, ok := scholarshipIDOf(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.ListScholarshipHolders(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.ScholarshipHolder{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"students": rows})
}

// Assign handles POST /api/v1/scholarships/{id}/students.
func (h *ScholarshipsHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := scholarshipIDOf(w, r)
	if !ok {
		return
	}
	students, ok := decodeStudentBatch(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.AssignScholarship(r.Context(), id, students)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.ScholarshipHolder{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"students": rows})
}

// Unassign handles POST /api/v1/scholarships/{id}/students/remove.
func (h *ScholarshipsHandler) Unassign(w http.ResponseWriter, r *http.Request) {
	id, ok := scholarshipIDOf(w, r)
	if !ok {
		return
	}
	students, ok := decodeStudentBatch(w, r)
	if !ok {
		return
	}
	n, err := h.svc.UnassignScholarship(r.Context(), id, students)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}
