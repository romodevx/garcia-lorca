package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// requireUUIDQuery reads a required uuid query parameter. A missing, blank or
// malformed one is the caller's mistake: it answers 400 in the shared error
// envelope naming the parameter and reports false (CC-304). Forwarded, the
// empty string reached Postgres as invalid uuid syntax and came back a 500.
func requireUUIDQuery(w http.ResponseWriter, q url.Values, name string) (string, bool) {
	value := strings.TrimSpace(q.Get(name))
	if value == "" {
		writeInvalidQuery(w, name, "Falta el parámetro "+name)
		return "", false
	}
	if !uuidShape.MatchString(value) {
		writeInvalidQuery(w, name, name+" debe ser un identificador válido (uuid)")
		return "", false
	}
	return value, true
}

// optionalUUIDQuery is requireUUIDQuery for a parameter that may be absent: an
// absent one is "" and true.
func optionalUUIDQuery(w http.ResponseWriter, q url.Values, name string) (string, bool) {
	if strings.TrimSpace(q.Get(name)) == "" {
		return "", true
	}
	return requireUUIDQuery(w, q, name)
}

// writeInvalidQuery answers 400 in the envelope with the offending field.
func writeInvalidQuery(w http.ResponseWriter, field, message string) {
	middleware.WriteError(w, http.StatusBadRequest, "invalid", message, map[string]string{"field": field})
}
