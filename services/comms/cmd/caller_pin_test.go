package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
	"github.com/garcia-lorca/pkg/logger"
)

const schedulerSA = "birthday-scheduler@proj.iam.gserviceaccount.com"

// tokenFor is an unsigned OIDC-shaped token carrying email (Cloud Run has
// already verified the signature by the time a request reaches comms).
func tokenFor(email string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return "Bearer " + enc([]byte(`{"alg":"RS256"}`)) + "." + enc([]byte(`{"email":"`+email+`"}`)) + ".sig"
}

// Review: run.invoker on comms is held by the shared runtime SA too, so the
// birthday run is pinned to the scheduler's own identity. Every other
// procedure is untouched.
func TestPinProcedureToCaller(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true; w.WriteHeader(http.StatusOK) })
	run := commsv1connect.CommsServiceRunBirthdaysProcedure
	announce := commsv1connect.CommsServiceAnnounceProcedure

	cases := []struct {
		name, expected, path, auth string
		want                       int
		wantReached                bool
	}{
		{"the scheduler runs it", schedulerSA, run, tokenFor(schedulerSA), http.StatusOK, true},
		{"the runtime SA cannot", schedulerSA, run, tokenFor("runtime@proj.iam.gserviceaccount.com"), http.StatusForbidden, false},
		{"no token cannot", schedulerSA, run, "", http.StatusForbidden, false},
		{"other procedures are not pinned", schedulerSA, announce, tokenFor("runtime@proj.iam.gserviceaccount.com"), http.StatusOK, true},
		{"unset locally: not checked", "", run, "", http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reached = false
			h := pinProcedureToCaller(next, run, tc.expected, logger.Default())
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want || reached != tc.wantReached {
				t.Errorf("status = %d reached = %v, want %d %v", rec.Code, reached, tc.want, tc.wantReached)
			}
		})
	}
}
