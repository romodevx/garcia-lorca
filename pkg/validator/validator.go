// Package validator defines the boundary-validation contract for García Lorca.
//
// All inbound data is validated at the edge against the contract schemas
// (OpenAPI at the gateway, protovalidate on Connect RPCs). This package gives
// the rest of the codebase a transport-agnostic Validator interface so use cases
// can declare a dependency on "something that validates" without binding to a
// concrete library.
//
// STATUS: interface stub. The concrete implementation (likely wrapping
// protovalidate / go-playground validator) is added when the first module needs
// it, behind a failing test as always.
package validator

import "context"

// Validator checks a value against its schema/rules and returns a ValidationError
// describing every failure, or nil when the value is valid.
type Validator interface {
	Validate(ctx context.Context, v any) error
}

// FieldError is a single field-level failure.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError aggregates field-level failures from one Validate call. It
// maps to errors.KindInvalid at the transport boundary.
type ValidationError struct {
	Fields []FieldError
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return "validation failed"
}

// TODO(M01): provide a concrete protovalidate-backed Validator and a
// boundary-validation test once the first contract is frozen.
