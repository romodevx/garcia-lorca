package domain

import (
	"bytes"
	"crypto/sha256"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxReceiptBytes is the largest receipt (comprobante) finance keeps: a phone
// photo of a ticket, a bank app's screenshot or a supplier's PDF invoice fits
// with room.
const MaxReceiptBytes = 5 << 20

// ReceiptFile is a receipt (comprobante): a JPEG, PNG or PDF, judged by its
// bytes, at most MaxReceiptBytes. An expense's receipt and a family's payment
// voucher are both one.
type ReceiptFile struct {
	contentType string
	data        []byte
}

var (
	receiptJPEG = []byte{0xFF, 0xD8, 0xFF}
	receiptPNG  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	receiptPDF  = []byte("%PDF-")
)

// NewReceiptFile accepts a JPEG, PNG or PDF by the file's own leading bytes —
// never by a name or a client-supplied content type, so a page or a script
// renamed to .pdf is refused — and at most MaxReceiptBytes.
func NewReceiptFile(data []byte) (ReceiptFile, error) {
	if len(data) == 0 {
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante está vacío")
	}
	if len(data) > MaxReceiptBytes {
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante pesa más de 5 MB")
	}
	switch {
	case bytes.HasPrefix(data, receiptJPEG):
		return ReceiptFile{contentType: "image/jpeg", data: data}, nil
	case bytes.HasPrefix(data, receiptPNG):
		return ReceiptFile{contentType: "image/png", data: data}, nil
	case bytes.HasPrefix(data, receiptPDF):
		return ReceiptFile{contentType: "application/pdf", data: data}, nil
	default:
		return ReceiptFile{}, glerrors.New(glerrors.KindInvalid, "el comprobante debe ser una imagen JPG o PNG, o un PDF")
	}
}

// ContentType is image/jpeg, image/png or application/pdf, as the bytes say.
func (f ReceiptFile) ContentType() string { return f.contentType }

// Data is the file itself.
func (f ReceiptFile) Data() []byte { return f.data }

// SHA256 is the digest of the file's bytes: what recognises the same file sent
// twice (a retry, a double tap) as one payment voucher.
func (f ReceiptFile) SHA256() []byte {
	sum := sha256.Sum256(f.data)
	return sum[:]
}
