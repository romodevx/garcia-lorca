package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR 0033 through the real mux: the import needs module_students,
// module_config AND module_student_import, and students:import (owner-only).

var studentImportRoutes = []string{"/api/v1/imports/students/preview", "/api/v1/imports/students/commit"}

func importFlags(off string) map[string]bool {
	flags := map[string]bool{"module_students": true, "module_config": true, "module_student_import": true}
	delete(flags, off)
	return flags
}

func TestStudentImportRoutesHideWhenAnyOfTheirModulesIsOff(t *testing.T) {
	for _, off := range []string{"module_students", "module_config", "module_student_import"} {
		gw := newTestGateway(t, importFlags(off))
		for _, path := range studentImportRoutes {
			t.Run(off+" "+path, func(t *testing.T) {
				res := doGateway(t, gw, http.MethodPost, path, "importer", "{}")
				assert.Equal(t, http.StatusNotFound, res.StatusCode)
			})
		}
	}
}

func TestStudentImportRoutesAreTheOwnersAlone(t *testing.T) {
	gw := newTestGateway(t, importFlags(""))
	for _, token := range []string{"config-writer", "with-perm", "student-enroller"} {
		for _, path := range studentImportRoutes {
			t.Run(token+" "+path, func(t *testing.T) {
				res := doGateway(t, gw, http.MethodPost, path, token, "{}")
				assert.Equal(t, http.StatusForbidden, res.StatusCode)
			})
		}
	}
}

// The holder reaches the handler: an empty file is the handler's 400, not the
// mux's 404 or the guard's 403.
func TestStudentImportRoutesReachTheHandler(t *testing.T) {
	gw := newTestGateway(t, importFlags(""))
	for _, path := range studentImportRoutes {
		res := doGateway(t, gw, http.MethodPost, path, "importer", `{"fileBase64":""}`)
		assert.Equal(t, http.StatusBadRequest, res.StatusCode, path)
	}
}

// PLAN-familias-importacion I2: undoing an import's families is a family
// membership write, so it needs module_students AND module_families (404
// before auth) and students:import AND guardians:write.
const undoImportFamilies = "/api/v1/imports/students/0f1c0e2a-0000-4000-8000-0000000000d1/families/undo"

func TestUndoImportFamiliesHidesWhenAnyOfItsModulesIsOff(t *testing.T) {
	for _, off := range []string{"module_students", "module_families"} {
		flags := map[string]bool{"module_students": true, "module_families": true, "module_config": true, "module_student_import": true}
		delete(flags, off)
		gw := newTestGateway(t, flags)
		for _, token := range []string{"family-importer", "bogus"} {
			res := doGateway(t, gw, http.MethodPost, undoImportFamilies, token, "")
			assert.Equal(t, http.StatusNotFound, res.StatusCode, "%s off, as %s", off, token)
		}
	}
}

func TestUndoImportFamiliesNeedsImportAndGuardiansWrite(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true, "module_families": true})
	res := doGateway(t, gw, http.MethodPost, undoImportFamilies, "", "")
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "no session")
	for _, token := range []string{"importer", "guardian-writer", "guardian-student-admin"} {
		res := doGateway(t, gw, http.MethodPost, undoImportFamilies, token, "")
		assert.Equal(t, http.StatusForbidden, res.StatusCode, token)
	}
}

func TestUndoImportFamiliesValidatesTheBatch(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true, "module_families": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/imports/students/batch-1/families/undo", "family-importer", "")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// The holder reaches students (the stub undoes a fixed result).
func TestUndoImportFamiliesReachesStudents(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true, "module_families": true})
	res := doGateway(t, gw, http.MethodPost, undoImportFamilies, "family-importer", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body map[string]int
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, map[string]int{"removed": 3, "dissolved": 1, "keptWithAccess": 0}, body)
}
