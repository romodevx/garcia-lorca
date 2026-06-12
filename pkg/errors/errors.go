// Package errors defines the shared error conventions for García Lorca services.
//
// The domain layer of each service declares its own sentinel/typed errors; this
// package provides the small, framework-free vocabulary those errors are built
// from so that adapters (gRPC/Connect, HTTP, persistence) can map consistently
// to the wire without importing each other.
//
// Conventions enforced across the codebase:
//   - Wrap with fmt.Errorf("context: %w", err) so errors.Is / errors.As keep working.
//   - Compare with errors.Is against the sentinels below (or service-local ones).
//   - Never swallow an error; if it is intentionally ignored, document why.
package errors

import (
	"errors"
	"fmt"
)

// Kind classifies an error independently of any transport. Adapters translate a
// Kind into the appropriate wire status (e.g. Connect code, HTTP status).
type Kind uint8

const (
	// KindUnknown is the zero value; treat as an internal error.
	KindUnknown Kind = iota
	// KindNotFound: the requested resource does not exist (or is hidden by RLS).
	KindNotFound
	// KindInvalid: input failed validation at the boundary.
	KindInvalid
	// KindUnauthorized: the caller is not authenticated.
	KindUnauthorized
	// KindForbidden: the caller is authenticated but not permitted.
	KindForbidden
	// KindConflict: the operation conflicts with current state (e.g. duplicate).
	KindConflict
	// KindInternal: an unexpected server-side failure.
	KindInternal
)

// Sentinel errors shared by all services. Service-specific sentinels live in
// each service's domain package; these cover cross-cutting cases.
var (
	// ErrNotFound is returned when a resource cannot be found.
	ErrNotFound = &Error{Kind: KindNotFound, Msg: "not found"}
	// ErrUnauthorized is returned when authentication is missing or invalid.
	ErrUnauthorized = &Error{Kind: KindUnauthorized, Msg: "unauthorized"}
	// ErrForbidden is returned when the caller lacks permission.
	ErrForbidden = &Error{Kind: KindForbidden, Msg: "forbidden"}
	// ErrConflict is returned when an operation conflicts with current state.
	ErrConflict = &Error{Kind: KindConflict, Msg: "conflict"}
)

// Error is the canonical typed error. It carries a Kind for transport mapping
// and may wrap an underlying cause for errors.Is / errors.As traversal.
type Error struct {
	Kind Kind
	Msg  string
	Err  error // wrapped cause; may be nil
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	return e.Msg
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Err }

// New builds a typed Error with the given kind and message.
func New(kind Kind, msg string) *Error {
	return &Error{Kind: kind, Msg: msg}
}

// Wrap annotates cause with a message and kind, preserving the chain via %w
// semantics (the returned Error unwraps to cause).
func Wrap(kind Kind, msg string, cause error) *Error {
	return &Error{Kind: kind, Msg: msg, Err: cause}
}

// KindOf reports the Kind of err by walking the chain. Errors that are not a
// *Error (or do not wrap one) are classified as KindInternal.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	if err == nil {
		return KindUnknown
	}
	return KindInternal
}
