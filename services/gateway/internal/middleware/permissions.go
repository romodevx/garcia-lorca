package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
)

// This file holds the two permission gates that RequirePermission cannot express,
// and the JSON error envelope they answer with.
//
// The envelope is new here and deliberately narrow. The gateway's older handlers
// answer text/plain; forking error handling across the whole edge for one module
// would be worse than the inconsistency, so the shared web client reads JSON and
// falls back to text, and the older handlers migrate as they are touched. What
// this envelope buys, and text cannot, is a MACHINE-READABLE reason: the panel
// has to disable an entry point and say which permission is missing, and it
// cannot branch on prose.

// errorBody is the single error envelope, matching docs/api/openapi.yaml.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	// Code is stable and machine-readable. The UI branches on this, never on the
	// message.
	Code string `json:"code"`
	// Message is human-readable, in Spanish, and safe to show: what happened and
	// what to do about it. Never a stack trace, never an internal id.
	Message string `json:"message"`
	// Details carries structured context, e.g. which permission is missing.
	Details map[string]string `json:"details,omitempty"`
}

// WriteError sends the shared error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{
		Code: code, Message: message, Details: details,
	}})
}

// RequireAllPermissions wraps next so only a principal holding EVERY listed
// permission passes, and names the first one it is missing.
//
// It exists for the counter sale, which needs uniforms:write AND finance:write
// because it moves goods and takes money in one act. Naming the missing
// permission is not a nicety: without it the panel cannot disable the entry point
// with a reason, so a cashier keys in six lines with a parent waiting and then
// eats a bare 403.
//
// It MUST run after RequireAuth. No principal is 401, not 403 — the difference
// matters to the client, since one means "log in" and the other means "ask your
// director for access".
func RequireAllPermissions(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			for _, permission := range permissions {
				if hasPermission(p, permission) {
					continue
				}
				WriteError(w, http.StatusForbidden, "forbidden",
					missingPermissionMessage(permission),
					map[string]string{"missingPermission": permission})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission wraps next so a principal holding ANY of the listed
// permissions passes. It is for a surface with two legitimate doors — the orders
// list, reachable by a director through uniforms:orders and by a family through
// its own scoped read — where requiring both would lock out everyone who rightly
// holds one.
func RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			for _, permission := range permissions {
				if hasPermission(p, permission) {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteError(w, http.StatusForbidden, "forbidden",
				"Tu cuenta no tiene acceso a esta sección", nil)
		})
	}
}

// missingPermissionMessage turns a permission into something a person can act
// on. The two the counter sale needs get their own wording because they are the
// ones a cashier will actually hit; anything else falls back to a truthful
// generic rather than leaking the permission string into the UI.
func missingPermissionMessage(permission string) string {
	switch permission {
	case "finance:write":
		return "Necesitas permiso de cobros para vender en mostrador"
	case "uniforms:write":
		return "Necesitas permiso de almacén para esta acción"
	default:
		return "Tu cuenta no tiene permiso para esta acción"
	}
}

// RequireStudentScopeUnless lets a request through only when it names one
// pupil (?studentId=), unless the caller holds `wide`. The counter (task 9)
// reads one family at a time; a whole cycle's ledger is finance:read's.
func RequireStudentScopeUnless(wide string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			if hasPermission(p, wide) || strings.TrimSpace(r.URL.Query().Get("studentId")) != "" {
				next.ServeHTTP(w, r)
				return
			}
			WriteError(w, http.StatusForbidden, "forbidden",
				"Tu cuenta consulta un alumno a la vez", nil)
		})
	}
}
