package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// --- fakeConfig: subject rename/delete (T13) ------------------------------------

func (f *fakeConfig) RenameSubject(_ context.Context, id, name string) (client.SubjectResult, error) {
	f.renamedSubject, f.renamedTo = id, name
	if f.err != nil {
		return client.SubjectResult{}, f.err
	}
	return client.SubjectResult{ID: id, GradeLevelID: "grade-1", Name: name}, nil
}

func (f *fakeConfig) DeleteSubject(_ context.Context, id string) error {
	f.deletedSubject = id
	return f.err
}

// --- PATCH /api/v1/config/subjects/{id} -----------------------------------------

func TestRenameSubjectForwardsAndAnswersTheSubject(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doPathReq(t, h.RenameSubject, http.MethodPatch, "/api/v1/config/subjects/"+subA, subA, `{"name":"Lengua"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, subA, svc.renamedSubject)
	assert.Equal(t, "Lengua", svc.renamedTo)
	var got map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, map[string]string{"id": subA, "gradeLevelId": "grade-1", "name": "Lengua"}, got)
}

func TestRenameSubjectRefusesBadShapesBeforeCallingConfig(t *testing.T) {
	cases := map[string]struct{ id, body string }{
		"malformed id":   {"not-a-uuid", `{"name":"Lengua"}`},
		"malformed body": {subA, `{"name":`},
		"no name":        {subA, `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeConfig{}
			h := handler.NewConfigHandler(svc)
			rec := doPathReq(t, h.RenameSubject, http.MethodPatch, "/api/v1/config/subjects/"+tc.id, tc.id, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, svc.renamedSubject, "config must not be called")
		})
	}
}

func TestRenameSubjectToANameTheGradeHasIsConflict(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewConfigHandler(svc)
	rec := doPathReq(t, h.RenameSubject, http.MethodPatch, "/api/v1/config/subjects/"+subA, subA, `{"name":"Inglés"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// --- DELETE /api/v1/config/subjects/{id} ----------------------------------------

func TestDeleteSubjectAnswersNoContent(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doPathReq(t, h.DeleteSubject, http.MethodDelete, "/api/v1/config/subjects/"+subA, subA, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, subA, svc.deletedSubject)
}

func TestDeleteSubjectRefusals(t *testing.T) {
	t.Run("malformed id never reaches config", func(t *testing.T) {
		svc := &fakeConfig{}
		h := handler.NewConfigHandler(svc)
		rec := doPathReq(t, h.DeleteSubject, http.MethodDelete, "/api/v1/config/subjects/x", "x", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Empty(t, svc.deletedSubject)
	})
	t.Run("an assigned subject is a conflict", func(t *testing.T) {
		svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, nil)}
		h := handler.NewConfigHandler(svc)
		rec := doPathReq(t, h.DeleteSubject, http.MethodDelete, "/api/v1/config/subjects/"+subA, subA, "")
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
	t.Run("unknown subject is not found", func(t *testing.T) {
		svc := &fakeConfig{err: connect.NewError(connect.CodeNotFound, nil)}
		h := handler.NewConfigHandler(svc)
		rec := doPathReq(t, h.DeleteSubject, http.MethodDelete, "/api/v1/config/subjects/"+subA, subA, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
