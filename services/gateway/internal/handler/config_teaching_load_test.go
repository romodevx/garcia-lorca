package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// --- fakeConfig: the teaching-load surface ------------------------------------

func (f *fakeConfig) AssignTeachingLoad(_ context.Context, in client.TeachingLoadInput) (client.TeachingLoadResult, error) {
	f.load = &in
	if f.err != nil {
		return client.TeachingLoadResult{}, f.err
	}
	return f.loadResult, nil
}

func (f *fakeConfig) DeleteTeachingAssignment(_ context.Context, id string) error {
	f.deletedAssignment = id
	return f.err
}

func (f *fakeConfig) ReassignTeachingAssignment(_ context.Context, id, teacherID string) (client.TeachingAssignment, error) {
	f.reassignID, f.reassignTo = id, teacherID
	if f.err != nil {
		return client.TeachingAssignment{}, f.err
	}
	return client.TeachingAssignment{ID: id, CycleID: "cyc-1", GroupID: grpA, SubjectID: subA, TeacherID: teacherID}, nil
}

const (
	tchA = "00000000-0000-4000-8000-000000000001"
	tchB = "00000000-0000-4000-8000-000000000002"
	grpA = "00000000-0000-4000-8000-000000000010"
	grpB = "00000000-0000-4000-8000-000000000011"
	subA = "00000000-0000-4000-8000-000000000020"
	subB = "00000000-0000-4000-8000-000000000021"
	asgA = "00000000-0000-4000-8000-000000000030"
)

// doPathReq drives a handler whose route carries {id}: httptest.NewRequest does
// not populate r.PathValue, so the test sets it as the mux would.
func doPathReq(t *testing.T, hf http.HandlerFunc, method, target, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	hf(rec, r)
	return rec
}

// --- POST /api/v1/teaching-assignments/bulk -----------------------------------

func TestAssignTeachingLoadForwardsAndAnswersTheFourLists(t *testing.T) {
	svc := &fakeConfig{}
	svc.loadResult = client.TeachingLoadResult{
		Created:    []client.TeachingAssignment{{ID: asgA, CycleID: "cyc-1", GroupID: grpA, SubjectID: subA, TeacherID: tchA}},
		Reassigned: []client.TeachingAssignment{},
		Skipped:    []client.TeachingLoadItem{{GroupID: grpB, SubjectID: subA}},
		Conflicts: []client.TeachingLoadConflict{{
			GroupID: grpB, SubjectID: subB, AssignmentID: "asg-x", TeacherID: tchB, TeacherName: "Grace Hopper",
		}},
	}
	h := handler.NewConfigHandler(svc)
	body := fmt.Sprintf(`{"teacherId":%q,"items":[{"groupId":%q,"subjectId":%q},{"groupId":%q,"subjectId":%q}],"replaceConflicts":true}`,
		tchA, grpA, subA, grpB, subB)

	rec := doReq(t, h.AssignTeachingLoad, http.MethodPost, "/api/v1/teaching-assignments/bulk", body)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := svc.load
	require.NotNil(t, got)
	assert.Equal(t, client.TeachingLoadInput{
		TeacherID:        tchA,
		Items:            []client.TeachingLoadItem{{GroupID: grpA, SubjectID: subA}, {GroupID: grpB, SubjectID: subB}},
		ReplaceConflicts: true,
	}, *got)
	assert.JSONEq(t, fmt.Sprintf(`{
		"created":[{"id":%q,"cycleId":"cyc-1","groupId":%q,"subjectId":%q,"teacherId":%q}],
		"reassigned":[],
		"skipped":[{"groupId":%q,"subjectId":%q}],
		"conflicts":[{"groupId":%q,"subjectId":%q,"assignmentId":"asg-x","teacherId":%q,"teacherName":"Grace Hopper"}]
	}`, asgA, grpA, subA, tchA, grpB, subA, grpB, subB, tchB), rec.Body.String())
}

func TestAssignTeachingLoadRejectsBadBodiesAtTheEdge(t *testing.T) {
	item := fmt.Sprintf(`{"groupId":%q,"subjectId":%q}`, grpA, subA)
	tooMany := strings.TrimSuffix(strings.Repeat(item+",", handler.MaxTeachingLoadItems+1), ",")
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"not json", `nope`, "invalid request body"},
		{"missing teacher", fmt.Sprintf(`{"items":[%s]}`, item), "teacherId"},
		{"malformed teacher", fmt.Sprintf(`{"teacherId":"tch-1","items":[%s]}`, item), "teacherId"},
		{"no items", fmt.Sprintf(`{"teacherId":%q,"items":[]}`, tchA), "items"},
		{"too many items", fmt.Sprintf(`{"teacherId":%q,"items":[%s]}`, tchA, tooMany), "items"},
		{"malformed group names the item", fmt.Sprintf(`{"teacherId":%q,"items":[%s,{"groupId":"g","subjectId":%q}]}`, tchA, item, subA), "items[1].groupId"},
		{"malformed subject names the item", fmt.Sprintf(`{"teacherId":%q,"items":[{"groupId":%q,"subjectId":""}]}`, tchA, grpA), "items[0].subjectId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			h := handler.NewConfigHandler(svc)

			rec := doReq(t, h.AssignTeachingLoad, http.MethodPost, "/api/v1/teaching-assignments/bulk", tt.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantMsg)
			assert.Nil(t, svc.load, "a bad body never reaches config")
		})
	}
}

// upstreamCases are the config refusals every teaching-load route maps.
var upstreamCases = []struct {
	name string
	code connect.Code
	want int
}{
	{"invalid argument", connect.CodeInvalidArgument, http.StatusBadRequest},
	{"not found", connect.CodeNotFound, http.StatusNotFound},
	{"already exists", connect.CodeAlreadyExists, http.StatusConflict},
}

func TestAssignTeachingLoadMapsUpstreamErrors(t *testing.T) {
	body := fmt.Sprintf(`{"teacherId":%q,"items":[{"groupId":%q,"subjectId":%q}]}`, tchA, grpA, subA)
	for _, tt := range upstreamCases {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.NewConfigHandler(&fakeConfig{err: connect.NewError(tt.code, errString("items[0]: group not found"))})
			rec := doReq(t, h.AssignTeachingLoad, http.MethodPost, "/api/v1/teaching-assignments/bulk", body)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

// --- DELETE /api/v1/teaching-assignments/{id} ---------------------------------

func TestDeleteTeachingAssignmentIs204(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doPathReq(t, h.DeleteTeachingAssignment, http.MethodDelete, "/api/v1/teaching-assignments/"+asgA, asgA, "")

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, asgA, svc.deletedAssignment)
}

func TestDeleteTeachingAssignmentRefusals(t *testing.T) {
	t.Run("malformed id", func(t *testing.T) {
		svc := &fakeConfig{}
		rec := doPathReq(t, handler.NewConfigHandler(svc).DeleteTeachingAssignment, http.MethodDelete, "/api/v1/teaching-assignments/x", "x", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Empty(t, svc.deletedAssignment)
	})
	for _, tt := range upstreamCases {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.NewConfigHandler(&fakeConfig{err: connect.NewError(tt.code, nil)})
			rec := doPathReq(t, h.DeleteTeachingAssignment, http.MethodDelete, "/api/v1/teaching-assignments/"+asgA, asgA, "")
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

// --- PATCH /api/v1/teaching-assignments/{id} ----------------------------------

func TestReassignTeachingAssignmentIs200(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doPathReq(t, h.ReassignTeachingAssignment, http.MethodPatch, "/api/v1/teaching-assignments/"+asgA, asgA,
		fmt.Sprintf(`{"teacherId":%q}`, tchB))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, asgA, svc.reassignID)
	assert.Equal(t, tchB, svc.reassignTo)
	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, asgA, out["id"])
	assert.Equal(t, tchB, out["teacherId"])
	assert.Equal(t, "cyc-1", out["cycleId"])
}

func TestReassignTeachingAssignmentRefusals(t *testing.T) {
	edge := []struct {
		name string
		id   string
		body string
	}{
		{"malformed id", "x", fmt.Sprintf(`{"teacherId":%q}`, tchB)},
		{"malformed teacher", asgA, `{"teacherId":"tch-2"}`},
		{"missing teacher", asgA, `{}`},
		{"not json", asgA, `nope`},
	}
	for _, tt := range edge {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := doPathReq(t, handler.NewConfigHandler(svc).ReassignTeachingAssignment, http.MethodPatch, "/api/v1/teaching-assignments/"+tt.id, tt.id, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, svc.reassignID, "a bad request never reaches config")
		})
	}
	for _, tt := range upstreamCases {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.NewConfigHandler(&fakeConfig{err: connect.NewError(tt.code, nil)})
			rec := doPathReq(t, h.ReassignTeachingAssignment, http.MethodPatch, "/api/v1/teaching-assignments/"+asgA, asgA, fmt.Sprintf(`{"teacherId":%q}`, tchB))
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
