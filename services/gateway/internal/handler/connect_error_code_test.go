package handler_test

import (
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// WriteConnectError forwards a service's machine code (X-Error-Code error
// metadata) as a response header — only when it looks like a code.
func TestWriteConnectErrorForwardsTheErrorCode(t *testing.T) {
	for _, tt := range []struct{ meta, want string }{
		{"GRADE_DECIMALS", "GRADE_DECIMALS"},
		{"", ""},
		{"lower_case", ""},
		{"TOO_LONG_" + string(make([]byte, 80)), ""},
	} {
		err := connect.NewError(connect.CodeInvalidArgument, errString("Solo se permite 1 decimal"))
		if tt.meta != "" {
			err.Meta().Set("X-Error-Code", tt.meta)
		}
		rec := httptest.NewRecorder()
		handler.WriteConnectError(rec, err)
		assert.Equal(t, 400, rec.Code)
		assert.Equal(t, tt.want, rec.Header().Get("X-Error-Code"), "meta %q", tt.meta)
		assert.Contains(t, rec.Body.String(), "Solo se permite 1 decimal")
	}
}
