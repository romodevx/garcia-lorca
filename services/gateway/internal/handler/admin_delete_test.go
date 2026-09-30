package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T2 — DELETE /api/v1/users/{id}. The handler forwards the caller's OWN token:
// iam decides who is asking from it (the self-deletion guard and the
// permission check), never from anything else in the request.

// newCookieReq is a request carrying an access token the way the web sends it:
// in the gl_access cookie.
func newCookieReq(method, target, token string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.AddCookie(&http.Cookie{Name: "gl_access", Value: token})
	return r
}

func TestDeleteUserForwardsTheCallersTokenAndAnswers204(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).DeleteUser)

	r := newCookieReq(http.MethodDelete, "/api/v1/users/usr-9", "caller-access-token")
	r.SetPathValue("id", "usr-9")
	w := serve(h, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "caller-access-token", f.deleteToken)
	assert.Equal(t, "usr-9", f.deleteUserID)
}

func TestDeleteUserForwardsIamRefusals(t *testing.T) {
	cases := []struct {
		name string
		code connect.Code
		msg  string
		want int
	}{
		{"self deletion", connect.CodePermissionDenied, "no puedes eliminar tu propia cuenta", http.StatusForbidden},
		{"last owner", connect.CodeAlreadyExists, "no se puede eliminar al último director activo", http.StatusConflict},
		{"unknown user", connect.CodeNotFound, "not found", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeAdmin{deleteErr: connect.NewError(tc.code, errString(tc.msg))}
			h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).DeleteUser)

			r := newCookieReq(http.MethodDelete, "/api/v1/users/usr-9", "tok")
			r.SetPathValue("id", "usr-9")
			w := serve(h, r)

			assert.Equal(t, tc.want, w.Code)
			assert.Equal(t, tc.msg, strings.TrimSpace(w.Body.String()), "iam's own sentence reaches the director")
		})
	}
}

func TestUserDeletionContractDocumentsPlainTextErrors(t *testing.T) {
	doc := loadOpenAPI(t)
	node, ok := doc.Paths["/api/v1/users/{id}"]["delete"]
	require.True(t, ok, "DELETE /api/v1/users/{id} is documented")
	var operation openAPIOperation
	require.NoError(t, node.Decode(&operation))
	for _, status := range []string{"204", "401", "403", "404", "409"} {
		assert.Contains(t, operation.Responses, status)
	}
	for status, response := range operation.Responses {
		if !strings.HasPrefix(status, "4") {
			continue
		}
		content := doc.resolve(t, response).Content
		assert.Contains(t, content, "text/plain", status)
		assert.NotContains(t, content, "application/json", status)
	}
}
