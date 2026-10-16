package handler_test

import (
	"context"
	"encoding/json"
	"errors"
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

// --- fakeConfig: catalog corrections (T8) -----------------------------------------

func (f *fakeConfig) ReorderLevels(_ context.Context, ids []string) ([]client.LevelResult, error) {
	f.reorderedLevels = ids
	if f.err != nil {
		return nil, f.err
	}
	out := make([]client.LevelResult, 0, len(ids))
	for i, id := range ids {
		out = append(out, client.LevelResult{ID: id, Name: "L" + id[:1], SortOrder: i + 1})
	}
	return out, nil
}

func (f *fakeConfig) RenameGradeLevel(_ context.Context, id, name string) (client.GradeLevelResult, error) {
	f.renamedGrade, f.renamedGradeTo = id, name
	if f.err != nil {
		return client.GradeLevelResult{}, f.err
	}
	return client.GradeLevelResult{ID: id, LevelID: "lvl-1", Name: name, Ordinal: 3}, nil
}

func (f *fakeConfig) DeleteGradeLevel(_ context.Context, id string) error {
	f.deletedGrade = id
	return f.err
}

const (
	lvlA = "a0000000-0000-4000-8000-000000000001"
	lvlB = "b0000000-0000-4000-8000-000000000002"
	grdA = "c0000000-0000-4000-8000-000000000003"
)

// --- levels: school order ---------------------------------------------------------

func TestListLevelsAnswersTheSortOrder(t *testing.T) {
	svc := &fakeConfig{levels: []client.LevelResult{{ID: lvlA, Name: "Primaria", SortOrder: 3}}}
	rec := httptest.NewRecorder()

	handler.NewConfigHandler(svc).ListLevels(rec, httptest.NewRequest(http.MethodGet, "/api/v1/config/levels", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Levels []map[string]any `json:"levels"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, float64(3), got.Levels[0]["sortOrder"])
}

func TestReorderLevelsForwardsTheWholeOrder(t *testing.T) {
	svc := &fakeConfig{}
	rec := httptest.NewRecorder()
	body := `{"levelIds":["` + lvlB + `","` + lvlA + `"]}`

	handler.NewConfigHandler(svc).ReorderLevels(rec, httptest.NewRequest(http.MethodPut, "/api/v1/config/levels/order", strings.NewReader(body)))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{lvlB, lvlA}, svc.reorderedLevels)
	var got struct {
		Levels []struct {
			ID        string `json:"id"`
			SortOrder int    `json:"sortOrder"`
		} `json:"levels"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Levels, 2)
	assert.Equal(t, lvlB, got.Levels[0].ID)
	assert.Equal(t, 1, got.Levels[0].SortOrder)
}

func TestReorderLevelsRefusesBadShapesBeforeCallingConfig(t *testing.T) {
	cases := map[string]string{
		"malformed body": `{"levelIds":`,
		"no list":        `{}`,
		"empty list":     `{"levelIds":[]}`,
		"not an id":      `{"levelIds":["primaria"]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := httptest.NewRecorder()
			handler.NewConfigHandler(svc).ReorderLevels(rec, httptest.NewRequest(http.MethodPut, "/api/v1/config/levels/order", strings.NewReader(body)))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Nil(t, svc.reorderedLevels, "config must not be called")
		})
	}
}

func TestReorderLevelsForwardsConfigsMismatch(t *testing.T) {
	const msg = "La lista de niveles no coincide con los niveles del colegio. Recarga la página e inténtalo de nuevo."
	svc := &fakeConfig{err: refusal(connect.CodeInvalidArgument, "LEVEL_ORDER_MISMATCH", msg)}
	rec := httptest.NewRecorder()

	handler.NewConfigHandler(svc).ReorderLevels(rec, httptest.NewRequest(http.MethodPut, "/api/v1/config/levels/order",
		strings.NewReader(`{"levelIds":["`+lvlA+`"]}`)))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "LEVEL_ORDER_MISMATCH", rec.Header().Get("X-Error-Code"))
	assert.Equal(t, msg, strings.TrimSpace(rec.Body.String()))
}

// --- grades -------------------------------------------------------------------------

func TestRenameGradeLevelForwardsAndAnswersTheGrade(t *testing.T) {
	svc := &fakeConfig{}

	rec := doPathReq(t, handler.NewConfigHandler(svc).RenameGradeLevel, http.MethodPatch, "/api/v1/config/grade-levels/"+grdA, grdA, `{"name":"3°"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{grdA, "3°"}, []string{svc.renamedGrade, svc.renamedGradeTo})
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, map[string]any{"id": grdA, "levelId": "lvl-1", "name": "3°", "ordinal": float64(3)}, got)
}

func TestRenameGradeLevelRefusesBadShapesBeforeCallingConfig(t *testing.T) {
	cases := map[string]struct{ id, body string }{
		"malformed id":   {"not-a-uuid", `{"name":"3°"}`},
		"malformed body": {grdA, `{"name":`},
		"blank name":     {grdA, `{"name":"  "}`},
		"too long":       {grdA, `{"name":"` + strings.Repeat("x", 81) + `"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := doPathReq(t, handler.NewConfigHandler(svc).RenameGradeLevel, http.MethodPatch, "/api/v1/config/grade-levels/"+tc.id, tc.id, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, svc.renamedGrade, "config must not be called")
		})
	}
}

func TestRenameGradeLevelForwardsTheNameConflict(t *testing.T) {
	svc := &fakeConfig{err: refusal(connect.CodeAlreadyExists, "GRADE_NAME_TAKEN", "Ya existe un grado con ese nombre en este nivel.")}

	rec := doPathReq(t, handler.NewConfigHandler(svc).RenameGradeLevel, http.MethodPatch, "/api/v1/config/grade-levels/"+grdA, grdA, `{"name":"2°"}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "GRADE_NAME_TAKEN", rec.Header().Get("X-Error-Code"))
}

func TestDeleteGradeLevel(t *testing.T) {
	svc := &fakeConfig{}
	rec := doPathReq(t, handler.NewConfigHandler(svc).DeleteGradeLevel, http.MethodDelete, "/api/v1/config/grade-levels/"+grdA, grdA, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, grdA, svc.deletedGrade)

	bad := &fakeConfig{}
	rec = doPathReq(t, handler.NewConfigHandler(bad).DeleteGradeLevel, http.MethodDelete, "/api/v1/config/grade-levels/x", "x", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, bad.deletedGrade)

	const msg = "No se puede borrar el grado 3°: tiene 2 grupos y 8 materias. Bórralos primero."
	busy := &fakeConfig{err: refusal(connect.CodeAlreadyExists, "GRADE_LEVEL_IN_USE", msg)}
	rec = doPathReq(t, handler.NewConfigHandler(busy).DeleteGradeLevel, http.MethodDelete, "/api/v1/config/grade-levels/"+grdA, grdA, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "GRADE_LEVEL_IN_USE", rec.Header().Get("X-Error-Code"))
	assert.Equal(t, msg, strings.TrimSpace(rec.Body.String()))
}

// --- groups: rename + delete across services --------------------------------------

const grpT8 = "d0000000-0000-4000-8000-000000000004"

type fakeGroupConfig struct {
	group     client.GroupResult
	getErr    error
	renameErr error
	usage     client.ConfigGroupUsage
	deleteErr error
	renamed   string
	deleted   string
}

func (f *fakeGroupConfig) GetGroup(_ context.Context, id string) (client.GroupResult, error) {
	if f.getErr != nil {
		return client.GroupResult{}, f.getErr
	}
	g := f.group
	g.ID = id
	return g, nil
}

func (f *fakeGroupConfig) RenameGroup(_ context.Context, id, name string) (client.GroupResult, error) {
	f.renamed = name
	if f.renameErr != nil {
		return client.GroupResult{}, f.renameErr
	}
	return client.GroupResult{ID: id, CycleID: "cyc", GradeLevelID: grdA, Name: strings.TrimSpace(name), Capacity: 25}, nil
}

func (f *fakeGroupConfig) GroupUsage(context.Context, string) (client.ConfigGroupUsage, error) {
	return f.usage, nil
}

func (f *fakeGroupConfig) DeleteGroup(_ context.Context, id string) error {
	f.deleted = id
	return f.deleteErr
}

type fakeGroupStudents struct {
	usage      client.StudentsGroupUsage
	usageErr   error
	refreshErr error
	relabelled string
}

func (f *fakeGroupStudents) GroupUsage(context.Context, string) (client.StudentsGroupUsage, error) {
	return f.usage, f.usageErr
}

func (f *fakeGroupStudents) RefreshGroupName(_ context.Context, groupID, name string) (int64, error) {
	f.relabelled = groupID + "=" + name
	return 1, f.refreshErr
}

type fakeGroupAttendance struct{ usage client.AttendanceGroupUsage }

func (f fakeGroupAttendance) GroupUsage(context.Context, string) (client.AttendanceGroupUsage, error) {
	return f.usage, nil
}

type fakeGroupGrades struct{ cards int64 }

func (f fakeGroupGrades) GroupReportCards(context.Context, string) (int64, error) {
	return f.cards, nil
}

type groupFixture struct {
	config     *fakeGroupConfig
	students   *fakeGroupStudents
	attendance fakeGroupAttendance
	grades     fakeGroupGrades
}

func newGroupFixture() *groupFixture {
	return &groupFixture{config: &fakeGroupConfig{group: client.GroupResult{Name: "A"}}, students: &fakeGroupStudents{}}
}

func (g *groupFixture) handler() *handler.GroupEditHandler {
	return handler.NewGroupEditHandler(handler.GroupEditDeps{
		Config: g.config, Students: g.students, Attendance: g.attendance, Grades: g.grades,
	})
}

func TestRenameGroupRenamesInConfigThenRelabelsThePupilsFiles(t *testing.T) {
	g := newGroupFixture()

	rec := doPathReq(t, g.handler().RenameGroup, http.MethodPatch, "/api/v1/config/groups/"+grpT8, grpT8, `{"name":" B "}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, " B ", g.config.renamed)
	assert.Equal(t, grpT8+"=B", g.students.relabelled, "the files get the name config stored")
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, map[string]any{"id": grpT8, "cycleId": "cyc", "gradeLevelId": grdA, "name": "B", "capacity": float64(25)}, got)
}

func TestRenameGroupRefusals(t *testing.T) {
	t.Run("bad shapes never reach a service", func(t *testing.T) {
		for name, tc := range map[string]struct{ id, body string }{
			"malformed id": {"x", `{"name":"B"}`},
			"blank name":   {grpT8, `{"name":""}`},
			"too long":     {grpT8, `{"name":"` + strings.Repeat("x", 81) + `"}`},
		} {
			g := newGroupFixture()
			rec := doPathReq(t, g.handler().RenameGroup, http.MethodPatch, "/api/v1/config/groups/"+tc.id, tc.id, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, name)
			assert.Empty(t, g.config.renamed, name)
		}
	})
	t.Run("a name taken in config relabels nothing", func(t *testing.T) {
		g := newGroupFixture()
		g.config.renameErr = refusal(connect.CodeAlreadyExists, "GROUP_NAME_TAKEN", "Ya existe un grupo con ese nombre en este grado y ciclo.")
		rec := doPathReq(t, g.handler().RenameGroup, http.MethodPatch, "/api/v1/config/groups/"+grpT8, grpT8, `{"name":"B"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, "GROUP_NAME_TAKEN", rec.Header().Get("X-Error-Code"))
		assert.Empty(t, g.students.relabelled)
	})
	t.Run("a relabel failure is reported, so the retry finishes it", func(t *testing.T) {
		g := newGroupFixture()
		g.students.refreshErr = connect.NewError(connect.CodeUnavailable, errors.New("down"))
		rec := doPathReq(t, g.handler().RenameGroup, http.MethodPatch, "/api/v1/config/groups/"+grpT8, grpT8, `{"name":"B"}`)
		// Review of T8: config already holds the new name, so a generic
		// "could not rename" would be false. It says what happened and how to
		// finish it.
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "GROUP_RELABEL_PENDING", rec.Header().Get("X-Error-Code"))
		assert.Contains(t, rec.Body.String(), "El grupo ya se llama B")
		assert.Contains(t, rec.Body.String(), "Guarda otra vez")
	})
}

func TestDeleteGroupThatNothingKeepsIsDeleted(t *testing.T) {
	g := newGroupFixture()

	rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")

	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, grpT8, g.config.deleted)
}

func TestDeleteGroupWithHistoryExplainsWhyAndDeletesNothing(t *testing.T) {
	g := newGroupFixture()
	g.students.usage = client.StudentsGroupUsage{Enrollments: 25}
	g.attendance.usage = client.AttendanceGroupUsage{Sessions: 40}

	rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "GROUP_IN_USE", rec.Header().Get("X-Error-Code"))
	assert.Equal(t, "No se puede borrar el grupo A: tiene 25 inscripciones y 40 listas de asistencia. Ese historial se conserva; puedes cambiarle el nombre.",
		strings.TrimSpace(rec.Body.String()))
	assert.Empty(t, g.config.deleted)
}

func TestDeleteGroupNamesEveryBlockerInOrder(t *testing.T) {
	g := newGroupFixture()
	g.config.usage = client.ConfigGroupUsage{TeachingAssignments: 1, ScheduleBlocks: 2}
	g.students.usage = client.StudentsGroupUsage{Enrollments: 1, Transfers: 1}
	g.attendance.usage = client.AttendanceGroupUsage{Sessions: 1, Marks: 3}
	g.grades.cards = 1

	rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")

	assert.Equal(t, "No se puede borrar el grupo A: tiene 1 clase asignada, 2 bloques de horario, 1 inscripción, 1 cambio de grupo, "+
		"1 lista de asistencia, 3 registros de asistencia y 1 boleta. Ese historial se conserva; puedes cambiarle el nombre.",
		strings.TrimSpace(rec.Body.String()))
}

func TestDeleteGroupWithOnlyClassesSaysToFreeThem(t *testing.T) {
	g := newGroupFixture()
	g.config.usage = client.ConfigGroupUsage{TeachingAssignments: 4, ScheduleBlocks: 6}

	rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "No se puede borrar el grupo A: tiene 4 clases asignadas y 6 bloques de horario. Quita primero sus clases asignadas.",
		strings.TrimSpace(rec.Body.String()))
	assert.Empty(t, g.config.deleted)
}

func TestDeleteGroupFailsClosed(t *testing.T) {
	t.Run("unknown group is 404 and asks nobody else", func(t *testing.T) {
		g := newGroupFixture()
		g.config.getErr = connect.NewError(connect.CodeNotFound, errors.New("not found"))
		rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Empty(t, g.config.deleted)
	})
	t.Run("a service that cannot answer never lets the delete through", func(t *testing.T) {
		g := newGroupFixture()
		g.students.usageErr = connect.NewError(connect.CodeUnavailable, errors.New("down"))
		rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Empty(t, g.config.deleted)
	})
	t.Run("config's own refusal (a class assigned meanwhile) is forwarded", func(t *testing.T) {
		g := newGroupFixture()
		g.config.deleteErr = refusal(connect.CodeAlreadyExists, "GROUP_IN_USE", "No se puede borrar el grupo A: tiene 1 clase asignada. Quita primero sus clases asignadas.")
		rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/"+grpT8, grpT8, "")
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, "GROUP_IN_USE", rec.Header().Get("X-Error-Code"))
	})
	t.Run("malformed id is 400", func(t *testing.T) {
		g := newGroupFixture()
		rec := doPathReq(t, g.handler().DeleteGroup, http.MethodDelete, "/api/v1/config/groups/x", "x", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- contract -----------------------------------------------------------------------

// Every new route answers its failures as text/plain (http.Error, ModuleGuard,
// RequirePermission, WriteConnectError), so the contract must say so.
func TestCatalogCorrectionsContractDocumentsPlainTextErrors(t *testing.T) {
	doc := loadOpenAPI(t)
	operations := []struct{ path, method string }{
		{"/api/v1/config/levels/order", "put"},
		{"/api/v1/config/grade-levels/{id}", "patch"},
		{"/api/v1/config/grade-levels/{id}", "delete"},
		{"/api/v1/config/groups/{id}", "patch"},
		{"/api/v1/config/groups/{id}", "delete"},
	}
	for _, op := range operations {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for _, status := range []string{"400", "401", "403", "404"} {
			response, ok := operation.Responses[status]
			require.True(t, ok, "%s %s documents %s", op.method, op.path, status)
			assert.Contains(t, doc.resolve(t, response).Content, "text/plain", "%s %s %s", op.method, op.path, status)
		}
	}
}
