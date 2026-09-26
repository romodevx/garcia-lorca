package handler

import (
	"fmt"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// MaxTeachingLoadItems bounds one bulk request. It is a TECHNICAL guard on the
// size of a single config transaction, not a business rule: a teacher may hold
// any number of classes, sent in as many requests as needed. It matches the
// config service's own bound.
const MaxTeachingLoadItems = 500

type teachingLoadItemJSON struct {
	GroupID   string `json:"groupId"`
	SubjectID string `json:"subjectId"`
}

type teachingLoadRequest struct {
	TeacherID        string                 `json:"teacherId"`
	Items            []teachingLoadItemJSON `json:"items"`
	ReplaceConflicts bool                   `json:"replaceConflicts"`
}

type teachingLoadConflictJSON struct {
	GroupID      string `json:"groupId"`
	SubjectID    string `json:"subjectId"`
	AssignmentID string `json:"assignmentId"`
	TeacherID    string `json:"teacherId"`
	TeacherName  string `json:"teacherName"`
}

type teachingLoadResponse struct {
	Created    []teachingAssignmentResponse `json:"created"`
	Reassigned []teachingAssignmentResponse `json:"reassigned"`
	Skipped    []teachingLoadItemJSON       `json:"skipped"`
	Conflicts  []teachingLoadConflictJSON   `json:"conflicts"`
}

type reassignTeachingAssignmentRequest struct {
	TeacherID string `json:"teacherId"`
}

// AssignTeachingLoad handles POST /api/v1/teaching-assignments/bulk: one
// teacher, a batch of (group, subject) slots, applied by config in ONE
// transaction. Always 200 when config accepts it — the body says per item what
// was created, reassigned, skipped (already this teacher's) or left as a
// conflict (another teacher's, replaceConflicts=false).
func (h *ConfigHandler) AssignTeachingLoad(w http.ResponseWriter, r *http.Request) {
	var req teachingLoadRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := invalidTeachingLoad(req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	res, err := h.svc.AssignTeachingLoad(r.Context(), toTeachingLoadInput(req))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeachingLoadResponse(res))
}

// DeleteTeachingAssignment handles DELETE /api/v1/teaching-assignments/{id}.
func (h *ConfigHandler) DeleteTeachingAssignment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a teaching assignment id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteTeachingAssignment(r.Context(), id); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReassignTeachingAssignment handles PATCH /api/v1/teaching-assignments/{id}
// with {teacherId}: the same slot, handed to another teacher.
func (h *ConfigHandler) ReassignTeachingAssignment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a teaching assignment id", http.StatusBadRequest)
		return
	}
	var req reassignTeachingAssignmentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !uuidShape.MatchString(req.TeacherID) {
		http.Error(w, "teacherId must be a teacher id", http.StatusBadRequest)
		return
	}
	a, err := h.svc.ReassignTeachingAssignment(r.Context(), id, req.TeacherID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeachingAssignmentResponse(a))
}

// invalidTeachingLoad checks the body against the contract. "" means valid.
// Duplicates and existence are config's to judge (it names the item too).
func invalidTeachingLoad(req teachingLoadRequest) string {
	if !uuidShape.MatchString(req.TeacherID) {
		return "teacherId must be a teacher id"
	}
	if len(req.Items) == 0 {
		return "items must not be empty"
	}
	if len(req.Items) > MaxTeachingLoadItems {
		return fmt.Sprintf("items must not exceed %d per request", MaxTeachingLoadItems)
	}
	for i, it := range req.Items {
		if !uuidShape.MatchString(it.GroupID) {
			return fmt.Sprintf("items[%d].groupId must be a group id", i)
		}
		if !uuidShape.MatchString(it.SubjectID) {
			return fmt.Sprintf("items[%d].subjectId must be a subject id", i)
		}
	}
	return ""
}

func toTeachingLoadInput(req teachingLoadRequest) client.TeachingLoadInput {
	items := make([]client.TeachingLoadItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, client.TeachingLoadItem{GroupID: it.GroupID, SubjectID: it.SubjectID})
	}
	return client.TeachingLoadInput{TeacherID: req.TeacherID, Items: items, ReplaceConflicts: req.ReplaceConflicts}
}

// toTeachingLoadResponse renders every list, empty ones as [] (never null), so
// the front can iterate without guards.
func toTeachingLoadResponse(res client.TeachingLoadResult) teachingLoadResponse {
	out := teachingLoadResponse{
		Created:    make([]teachingAssignmentResponse, 0, len(res.Created)),
		Reassigned: make([]teachingAssignmentResponse, 0, len(res.Reassigned)),
		Skipped:    make([]teachingLoadItemJSON, 0, len(res.Skipped)),
		Conflicts:  make([]teachingLoadConflictJSON, 0, len(res.Conflicts)),
	}
	for _, a := range res.Created {
		out.Created = append(out.Created, toTeachingAssignmentResponse(a))
	}
	for _, a := range res.Reassigned {
		out.Reassigned = append(out.Reassigned, toTeachingAssignmentResponse(a))
	}
	for _, s := range res.Skipped {
		out.Skipped = append(out.Skipped, teachingLoadItemJSON{GroupID: s.GroupID, SubjectID: s.SubjectID})
	}
	for _, c := range res.Conflicts {
		out.Conflicts = append(out.Conflicts, teachingLoadConflictJSON(c))
	}
	return out
}
