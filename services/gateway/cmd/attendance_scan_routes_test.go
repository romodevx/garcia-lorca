package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// T4 scan routes through the whole chain: both module flags, auth and the
// permission each surface reads with. The stub students service answers every
// code as stu-1's valid card; stu-1 is actively enrolled in the stub grp-1.
func TestAttendanceScanRoutesSecurityChain(t *testing.T) {
	// The list on screen is today's (the teacher's window allows it).
	body := `{"date":"` + time.Now().UTC().Format(time.DateOnly) + `","groupId":"grp-1","code":"GLCARD:AAAAAAAAAAAAAAAAAAAAAA"}`
	both := map[string]bool{"module_attendance": true, "module_student_card": true}
	tests := []struct {
		name    string
		modules map[string]bool
		path    string
		token   string
		want    int
	}{
		{"teacher: own reader resolves the card", both, "/api/v1/teaching/attendance/scan", "att-own-reader", http.StatusOK},
		{"teacher: attendance module off", map[string]bool{"module_student_card": true}, "/api/v1/teaching/attendance/scan", "att-own-reader", http.StatusNotFound},
		{"teacher: student_card module off", map[string]bool{"module_attendance": true}, "/api/v1/teaching/attendance/scan", "att-own-reader", http.StatusNotFound},
		{"teacher: no token", both, "/api/v1/teaching/attendance/scan", "", http.StatusUnauthorized},
		{"teacher: attendance:read is not attendance_own:read", both, "/api/v1/teaching/attendance/scan", "att-reader", http.StatusForbidden},
		{"director: attendance reader resolves the card", both, "/api/v1/attendance/scan", "att-reader", http.StatusOK},
		{"director: attendance module off", map[string]bool{"module_student_card": true}, "/api/v1/attendance/scan", "att-reader", http.StatusNotFound},
		{"director: student_card module off", map[string]bool{"module_attendance": true}, "/api/v1/attendance/scan", "att-reader", http.StatusNotFound},
		{"director: no token", both, "/api/v1/attendance/scan", "", http.StatusUnauthorized},
		{"director: attendance_own:read is not attendance:read", both, "/api/v1/attendance/scan", "att-own-reader", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.modules)
			res := doGateway(t, gw, http.MethodPost, tt.path, tt.token, body)
			assert.Equal(t, tt.want, res.StatusCode)
			if tt.want == http.StatusOK {
				assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
				assert.Equal(t, "ok", res.Header.Get("X-Scan-Result"))
			}
		})
	}
}
