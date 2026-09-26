package middleware

import (
	"net/http"
	"strings"
)

// preflightMaxAge caps how long a browser may cache a preflight result (seconds).
const preflightMaxAge = "600"

// exposedHeaders are the non-safelisted response headers the web must read:
// X-Confirm-Required marks a 409 the user can resolve by confirming (teacher
// access reusing an existing account); X-Scan-Result names an attendance
// scan's verdict (T4), since two of its refusals share 422.
const exposedHeaders = "X-Confirm-Required, X-Scan-Result"

// CORS returns middleware that enables the browser web client to call the
// gateway cross-origin. The web fetches with credentials (auth cookies), so the
// response MUST carry Access-Control-Allow-Credentials:true and reflect the exact
// request Origin — a wildcard origin is illegal alongside credentials. Only
// origins in allowedOrigins are reflected; a preflight OPTIONS from an allowed
// origin is answered 204 before routing (the mux otherwise 405s OPTIONS).
//
// The gateway does not itself block a disallowed origin: it simply omits the CORS
// headers, and the browser enforces the block. Server-side authz (auth, module
// guards, permissions) remains the real security boundary.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok && origin != "" {
				h := w.Header()
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Expose-Headers", exposedHeaders)
				if r.Method == http.MethodOptions {
					reqHeaders := r.Header.Get("Access-Control-Request-Headers")
					if reqHeaders == "" {
						reqHeaders = "Content-Type"
					}
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", reqHeaders)
					h.Set("Access-Control-Max-Age", preflightMaxAge)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
