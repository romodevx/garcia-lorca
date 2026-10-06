package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
