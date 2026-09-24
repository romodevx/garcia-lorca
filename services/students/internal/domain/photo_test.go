package domain_test

import (
	"bytes"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var (
	jpegHead = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	pngHead  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0, 0, 0, 0x0D}
)

func TestNewPhotoKnowsJPEGAndPNGByTheirBytes(t *testing.T) {
	for want, data := range map[string][]byte{"image/jpeg": jpegHead, "image/png": pngHead} {
		p, err := domain.NewPhoto(data)
		if err != nil {
			t.Fatalf("NewPhoto(%s): %v", want, err)
		}
		if p.ContentType() != want || !bytes.Equal(p.Data(), data) {
			t.Errorf("photo = %q (%d bytes), want %s", p.ContentType(), len(p.Data()), want)
		}
	}
}

// A file is what its bytes say, not what its name or header claims: a GIF, a
// PDF or a script renamed to .jpg is refused.
func TestNewPhotoRefusesEverythingElse(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":         nil,
		"gif":           []byte("GIF89a......"),
		"pdf":           []byte("%PDF-1.7...."),
		"html as image": []byte("<script>x</script>"),
	} {
		if _, err := domain.NewPhoto(data); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%s: want KindInvalid, got %v", name, err)
		}
	}
}

func TestNewPhotoRefusesMoreThanTwoMegabytes(t *testing.T) {
	big := append(append([]byte{}, jpegHead...), make([]byte, 2<<20)...)
	if _, err := domain.NewPhoto(big); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}
