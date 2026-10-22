package domain_test

import (
	"bytes"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestNewReceiptFile(t *testing.T) {
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{1}, 10)...)
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}, 1, 2)
	pdf := []byte("%PDF-1.7\n%âãÏÓ\n")
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{"jpeg", jpeg, "image/jpeg", false},
		{"png", png, "image/png", false},
		{"pdf", pdf, "application/pdf", false},
		{"empty", nil, "", true},
		{"html renamed", []byte("<html><script>"), "", true},
		{"too large", append(append([]byte{}, pdf...), make([]byte, domain.MaxReceiptBytes)...), "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := domain.NewReceiptFile(tc.data)
			if tc.wantErr {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("want KindInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if f.ContentType() != tc.want || len(f.Data()) != len(tc.data) {
				t.Fatalf("got %q (%d bytes)", f.ContentType(), len(f.Data()))
			}
		})
	}
}

// A payment voucher is deduplicated by its file's digest: a retry or a double
// tap sending the same bytes must be recognised as the same voucher.
func TestReceiptFileSHA256(t *testing.T) {
	pdf := []byte("%PDF-1.7\nsame bytes")
	a, err := domain.NewReceiptFile(pdf)
	if err != nil {
		t.Fatal(err)
	}
	b, err := domain.NewReceiptFile(append([]byte{}, pdf...))
	if err != nil {
		t.Fatal(err)
	}
	other, err := domain.NewReceiptFile([]byte("%PDF-1.7\nother bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.SHA256()) != 32 {
		t.Fatalf("digest is %d bytes, want 32", len(a.SHA256()))
	}
	if !bytes.Equal(a.SHA256(), b.SHA256()) {
		t.Fatal("the same bytes must have the same digest")
	}
	if bytes.Equal(a.SHA256(), other.SHA256()) {
		t.Fatal("different bytes must not share a digest")
	}
}
