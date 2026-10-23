package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ADR 0041 (T3) through the real mux: every families route needs
// module_students AND module_families (404 before auth when either is off).
// Reading is students:read (the adults: guardians:read); renaming and
// dismissing a suggestion are students:update; building the family — create,
// add, remove, dissolve — and its adults' access are guardians:write, because
// a family hands its accounts to every pupil in it (security gate H1).

const (
	familyRoute  = "/api/v1/families/0f1c0e2a-0000-4000-8000-0000000000f1"
	familyPupil  = "0f1c0e2a-0000-4000-8000-0000000000a1"
	familyPupils = `{"studentIds":["` + familyPupil + `"]}`
)

type familyCall struct{ method, path, body string }

var (
	familyReads = []familyCall{
		{http.MethodGet, "/api/v1/families", ""},
		{http.MethodGet, "/api/v1/families/suggestions", ""},
		{http.MethodGet, familyRoute, ""},
	}
	familyAdults  = familyCall{http.MethodGet, familyRoute + "/contacts", ""}
	familyUpdates = []familyCall{
		{http.MethodPatch, familyRoute, `{"name":"GARCÍA"}`},
		{http.MethodPost, "/api/v1/families/suggestions/dismiss", `{"studentIds":["` + familyPupil + `","0f1c0e2a-0000-4000-8000-0000000000b1"]}`},
	}
	familyMembership = []familyCall{
		{http.MethodPost, "/api/v1/families", familyPupils},
		{http.MethodPost, familyRoute + "/students", familyPupils},
		{http.MethodDelete, familyRoute + "/students/" + familyPupil, ""},
		{http.MethodDelete, familyRoute, ""},
		{http.MethodPost, familyRoute + "/contacts", `{"relationship":"mother","fullName":"ANA"}`},
		{http.MethodPost, familyRoute + "/access/revoke", `{"guardianUserId":"0f1c0e2a-0000-4000-8000-0000000000e1"}`},
	}
)

func allFamilyCalls() []familyCall {
	calls := append([]familyCall{}, familyReads...)
	calls = append(calls, familyAdults)
	calls = append(calls, familyUpdates...)
	return append(calls, familyMembership...)
}

func familiesFlags(off string) map[string]bool {
	flags := map[string]bool{"module_students": true, "module_families": true}
	delete(flags, off)
	return flags
}

func TestFamilyRoutesHideWhenAnyOfTheirModulesIsOff(t *testing.T) {
	for _, off := range []string{"module_students", "module_families"} {
		gw := newTestGateway(t, familiesFlags(off))
		t.Run(off, func(t *testing.T) {
			for _, c := range allFamilyCalls() {
				// Even the holder of every permission, and even without a session:
				// the module is invisible before auth runs.
				for _, token := range []string{"guardian-student-admin", "bogus"} {
					res := doGateway(t, gw, c.method, c.path, token, c.body)
					assert.Equal(t, http.StatusNotFound, res.StatusCode, "%s %s as %s", c.method, c.path, token)
				}
			}
		})
	}
}

// H1 regression: students:update can rename a family and dismiss a
// suggestion, but cannot build one — that hands out app access.
func TestFamilyMembershipIsGuardiansWriteNotStudentsUpdate(t *testing.T) {
	gw := newTestGateway(t, familiesFlags(""))
	for _, c := range familyMembership {
		res := doGateway(t, gw, c.method, c.path, "student-updater", c.body)
		assert.Equal(t, http.StatusForbidden, res.StatusCode, "%s %s with students:update", c.method, c.path)
	}
	for _, c := range familyUpdates {
		res := doGateway(t, gw, c.method, c.path, "student-reader", c.body)
		assert.Equal(t, http.StatusForbidden, res.StatusCode, "%s %s with students:read", c.method, c.path)
	}
	for _, c := range familyReads {
		res := doGateway(t, gw, c.method, c.path, "guardian-writer", c.body)
		assert.Equal(t, http.StatusForbidden, res.StatusCode, "%s %s without students:read", c.method, c.path)
	}
	res := doGateway(t, gw, familyAdults.method, familyAdults.path, "student-updater", "")
	assert.Equal(t, http.StatusForbidden, res.StatusCode, "the family's adults need guardians:read")
}

// The holder reaches the handler, which asks students (the stub has not
// implemented the RPCs): neither the mux's 404 nor the guard's 401/403.
func TestFamilyRoutesReachTheHandler(t *testing.T) {
	gw := newTestGateway(t, familiesFlags(""))
	reach := func(token string, calls ...familyCall) {
		for _, c := range calls {
			res := doGateway(t, gw, c.method, c.path, token, c.body)
			assert.NotContains(t, []int{http.StatusNotFound, http.StatusForbidden, http.StatusUnauthorized}, res.StatusCode,
				"%s %s as %s", c.method, c.path, token)
		}
	}
	reach("student-reader", familyReads...)
	reach("guardian-reader", familyAdults)
	reach("student-updater", familyUpdates...)
	reach("guardian-writer", familyMembership...)
}
