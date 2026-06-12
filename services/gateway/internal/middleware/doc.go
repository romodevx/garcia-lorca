// Package middleware holds the gateway's HTTP/Connect middleware. The feature-flag middleware lives here: it is the server-side security boundary that turns a disabled module's routes into 404s (default-deny via pkg/featureflag).
package middleware
