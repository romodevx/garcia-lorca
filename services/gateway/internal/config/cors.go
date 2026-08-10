package config

import "strings"

// defaultAllowedOrigin is the browser origin the web dev server runs on when
// WEB_ORIGIN is unset. Shared environments MUST set WEB_ORIGIN explicitly — the
// web and the gateway live on different hosts there, so this default never fits.
const defaultAllowedOrigin = "http://localhost:3000"

// ParseAllowedOrigins turns a comma-separated WEB_ORIGIN list into the exact
// origins the CORS middleware may reflect. Entries are trimmed and blanks
// dropped; an empty result falls back to the local dev default so `make run`
// works out of the box.
//
//	""                                   → ["http://localhost:3000"]
//	"http://localhost:3002, https://x"   → ["http://localhost:3002", "https://x"]
func ParseAllowedOrigins(csv string) []string {
	origins := make([]string, 0, 1)
	for _, raw := range strings.Split(csv, ",") {
		if o := strings.TrimSpace(raw); o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		return []string{defaultAllowedOrigin}
	}
	return origins
}
