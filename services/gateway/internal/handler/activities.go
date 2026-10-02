package handler

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Edge bounds of the Actividades module (T17), mirroring finance's domain.
const (
	maxActivityName    = 80
	maxActivitySlots   = 21
	maxActivityOptions = 8
	maxEnrollBatch     = 200
	maxWeekday         = 7
	activityDayLayout  = "2006-01-02"
)

var (
	activityPeriodicities = map[string]bool{"monthly": true, "weekly": true, "per_cycle": true}
	activityClock         = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

// ActivityService is the finance surface the Actividades module needs
// (consumer-defined; the Connect FinanceClient implements it).
type ActivityService interface {
	ListActivities(ctx context.Context, includeInactive bool) ([]client.Activity, error)
	GetActivity(ctx context.Context, id string) (client.Activity, error)
	CreateActivity(ctx context.Context, in client.ActivityInput) (client.Activity, error)
	UpdateActivity(ctx context.Context, id string, in client.ActivityInput) (client.Activity, error)
	ListActivityEnrollments(ctx context.Context, activityID string) ([]client.ActivityEnrollment, error)
	EnrollInActivity(ctx context.Context, activityID, startsOn string, items []client.ActivityEnrollItem) ([]client.ActivityEnrollment, int32, error)
	UnenrollFromActivity(ctx context.Context, activityID string, studentIDs []string, endsOn string) (int32, error)
	ChangeActivityOption(ctx context.Context, activityID, studentID, optionID string) (client.ActivityEnrollment, error)
}

// ActivitiesHandler serves the Actividades module (T17): the activities, their
// price options and who is enrolled. Every route runs behind module_finance
// AND module_activities plus activities:read / activities:write (cmd/main.go).
// The charges it causes are finance's; collecting them is Registrar pago.
type ActivitiesHandler struct {
	svc ActivityService
}

// NewActivitiesHandler wires the handler.
func NewActivitiesHandler(svc ActivityService) *ActivitiesHandler {
	return &ActivitiesHandler{svc: svc}
}

// activityIDOf is the path's id when it has an id's shape; otherwise 404.
func activityIDOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "activity not found", http.StatusNotFound)
		return "", false
	}
	return id, true
}

// activityRequest is the body of create and update. Active is a pointer so a
// body that leaves it out means "active", as the contract's default says.
type activityRequest struct {
	Name            string                       `json:"name"`
	Schedule        []client.ActivitySlot        `json:"schedule"`
	Periodicity     string                       `json:"periodicity"`
	BillingStartsOn string                       `json:"billingStartsOn"`
	FirstDueOn      string                       `json:"firstDueOn"`
	LateFeeCents    int64                        `json:"lateFeeCents"`
	Active          *bool                        `json:"active"`
	Options         []client.ActivityPriceOption `json:"options"`
}

// invalid is the edge check against the contract; finance re-validates the
// rules that need the whole activity (overlaps, the due date inside the period).
func (req activityRequest) invalid() string {
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "":
		return "name is required"
	case tooLong(name, maxActivityName):
		return "name is at most 80 characters"
	case len(req.Schedule) == 0 || len(req.Schedule) > maxActivitySlots:
		return "schedule needs 1 to 21 slots"
	case !activityPeriodicities[req.Periodicity]:
		return "periodicity must be monthly, weekly or per_cycle"
	case !isDay(req.BillingStartsOn) || !isDay(req.FirstDueOn):
		return "dates must be YYYY-MM-DD"
	case req.LateFeeCents < 0:
		return "lateFeeCents cannot be negative"
	case len(req.Options) == 0 || len(req.Options) > maxActivityOptions:
		return "options needs 1 to 8 entries"
	}
	for _, s := range req.Schedule {
		if s.Weekday < 1 || s.Weekday > maxWeekday || !activityClock.MatchString(s.Start) || !activityClock.MatchString(s.End) {
			return "each slot needs a weekday 1–7 and HH:MM times"
		}
	}
	for _, o := range req.Options {
		if o.ID != "" && !uuidShape.MatchString(o.ID) {
			return "option id must be a uuid"
		}
		if o.AmountCents <= 0 || (o.DaysPerWeek != nil && (*o.DaysPerWeek < 1 || *o.DaysPerWeek > maxWeekday)) {
			return "each option needs daysPerWeek 1–7 (or null) and an amount above zero"
		}
	}
	return ""
}

func (req activityRequest) input() client.ActivityInput {
	active := req.Active == nil || *req.Active
	return client.ActivityInput{
		Name: strings.TrimSpace(req.Name), Schedule: req.Schedule, Periodicity: req.Periodicity,
		BillingStartsOn: req.BillingStartsOn, FirstDueOn: req.FirstDueOn,
		LateFeeCents: req.LateFeeCents, Active: active, Options: req.Options,
	}
}

func isDay(s string) bool {
	_, err := time.Parse(activityDayLayout, s)
	return err == nil
}

// decodeActivity reads and checks a create/update body, answering 400 itself.
func decodeActivity(w http.ResponseWriter, r *http.Request) (client.ActivityInput, bool) {
	var req activityRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return client.ActivityInput{}, false
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return client.ActivityInput{}, false
	}
	return req.input(), true
}

// List handles GET /api/v1/activities[?includeInactive=true].
func (h *ActivitiesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListActivities(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.Activity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"activities": rows})
}

// Get handles GET /api/v1/activities/{id}.
func (h *ActivitiesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	a, err := h.svc.GetActivity(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// Create handles POST /api/v1/activities.
func (h *ActivitiesHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeActivity(w, r)
	if !ok {
		return
	}
	a, err := h.svc.CreateActivity(r.Context(), in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// Update handles PUT /api/v1/activities/{id}.
func (h *ActivitiesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	in, ok := decodeActivity(w, r)
	if !ok {
		return
	}
	a, err := h.svc.UpdateActivity(r.Context(), id, in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ListEnrollments handles GET /api/v1/activities/{id}/enrollments.
func (h *ActivitiesHandler) ListEnrollments(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.ListActivityEnrollments(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.ActivityEnrollment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enrollments": rows})
}

type activityEnrollRequest struct {
	StartsOn    string                      `json:"startsOn"`
	Enrollments []client.ActivityEnrollItem `json:"enrollments"`
}

func (req activityEnrollRequest) invalid() string {
	if req.StartsOn != "" && !isDay(req.StartsOn) {
		return "startsOn must be YYYY-MM-DD"
	}
	if len(req.Enrollments) == 0 || len(req.Enrollments) > maxEnrollBatch {
		return "enrollments needs 1 to 200 pupils"
	}
	for _, e := range req.Enrollments {
		if !uuidShape.MatchString(e.StudentID) || !uuidShape.MatchString(e.OptionID) {
			return "studentId and optionId must be uuids"
		}
	}
	return ""
}

// Enroll handles POST /api/v1/activities/{id}/enrollments.
func (h *ActivitiesHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	var req activityEnrollRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	rows, charged, err := h.svc.EnrollInActivity(r.Context(), id, req.StartsOn, req.Enrollments)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if rows == nil {
		rows = []client.ActivityEnrollment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enrollments": rows, "chargesCreated": charged})
}

type unactivityEnrollRequest struct {
	EndsOn     string   `json:"endsOn"`
	StudentIDs []string `json:"studentIds"`
}

// Unenroll handles POST /api/v1/activities/{id}/enrollments/end.
func (h *ActivitiesHandler) Unenroll(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	var req unactivityEnrollRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.EndsOn != "" && !isDay(req.EndsOn) {
		http.Error(w, "endsOn must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if len(req.StudentIDs) == 0 || len(req.StudentIDs) > maxEnrollBatch {
		http.Error(w, "studentIds needs 1 to 200 pupils", http.StatusBadRequest)
		return
	}
	for _, s := range req.StudentIDs {
		if !uuidShape.MatchString(s) {
			http.Error(w, "studentIds must be uuids", http.StatusBadRequest)
			return
		}
	}
	n, err := h.svc.UnenrollFromActivity(r.Context(), id, req.StudentIDs, req.EndsOn)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ended": n})
}

// ChangeOption handles PUT /api/v1/activities/{id}/enrollments/{studentId}.
func (h *ActivitiesHandler) ChangeOption(w http.ResponseWriter, r *http.Request) {
	id, ok := activityIDOf(w, r)
	if !ok {
		return
	}
	studentID := r.PathValue("studentId")
	if !uuidShape.MatchString(studentID) {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	var req struct {
		OptionID string `json:"optionId"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !uuidShape.MatchString(req.OptionID) {
		http.Error(w, "optionId must be a uuid", http.StatusBadRequest)
		return
	}
	e, err := h.svc.ChangeActivityOption(r.Context(), id, studentID, req.OptionID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}
