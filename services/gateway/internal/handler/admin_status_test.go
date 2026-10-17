package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
)

// ISSUE-CC-117 — POST /api/v1/users/{id}/status forwards the caller's OWN
// token, as DELETE does: iam decides who is asking (the self-suspension guard)
// from it, never from anything else in the request.

// statusReq is a status change carrying the access token the way the web
// sends it: in the gl_access cookie.
func statusReq(token, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/usr-9/status", strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "gl_access", Value: token})
	return r
}

func TestSetUserStatusForwardsTheCallersToken(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserStatus)

	r := statusReq("caller-access-token", `{"status":"suspended"}`)
	r.SetPathValue("id", "usr-9")
	w := serve(h, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "caller-access-token", f.statusToken)
	assert.Equal(t, "usr-9", f.statusUserID)
	assert.Equal(t, "suspended", f.statusValue)
}

func TestSetUserStatusForwardsIamRefusals(t *testing.T) {
	cases := []struct {
		name string
		code connect.Code
		msg  string
		want int
	}{
		{"self suspension", connect.CodePermissionDenied, "no puedes suspender tu propia cuenta", http.StatusForbidden},
		{"last owner", connect.CodeAlreadyExists, "no se puede suspender al último director activo", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeAdmin{err: connect.NewError(tc.code, errString(tc.msg))}
			h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserStatus)

			r := statusReq("tok", `{"status":"suspended"}`)
			r.SetPathValue("id", "usr-9")
			w := serve(h, r)

			assert.Equal(t, tc.want, w.Code)
			assert.Equal(t, tc.msg, strings.TrimSpace(w.Body.String()), "iam's own sentence reaches the director")
		})
	}
}
