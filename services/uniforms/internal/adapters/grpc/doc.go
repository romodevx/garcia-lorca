// Package grpcadapter adapts the uniform use cases to the generated
// garcialorca.uniforms.v1 Connect service. It only translates: proto messages to
// use-case calls, domain entities back to proto, and domain error kinds to
// Connect codes. Nothing decides anything here.
package grpcadapter
