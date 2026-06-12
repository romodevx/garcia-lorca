// Package pdf defines the document-rendering boundary for García Lorca.
//
// The docs service (and others producing receipts, report cards, etc.) renders
// PDFs through the Renderer interface so the rendering engine stays a
// replaceable detail behind a stable contract.
//
// STATUS: interface stub. A concrete renderer is added when the docs module is
// implemented, test-first.
package pdf

import "context"

// Document is the input to a render: a named template and its data bindings.
type Document struct {
	Template string
	Data     map[string]any
}

// Renderer produces PDF bytes from a Document.
type Renderer interface {
	Render(ctx context.Context, doc Document) ([]byte, error)
}

// TODO(docs): provide a concrete Renderer and a golden-file render test when the
// docs module begins.
