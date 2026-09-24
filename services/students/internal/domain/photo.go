package domain

import (
	"bytes"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxPhotoBytes is the largest photo a file keeps. The browser shrinks a
// capture to a few hundred KB before sending it; two megabytes is room for a
// phone photo sent as is, and a ceiling on what one upload can cost.
const MaxPhotoBytes = 2 << 20

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
)

// Photo is a pupil's photo: a JPEG or PNG image, identified by its bytes.
type Photo struct {
	contentType string
	data        []byte
}

// NewPhoto accepts JPEG and PNG only, judged by the file's own leading bytes
// (its "magic number") — never by a name or a client-supplied content type, so
// a script or a PDF renamed to .jpg is refused — and at most MaxPhotoBytes.
func NewPhoto(data []byte) (Photo, error) {
	if len(data) == 0 {
		return Photo{}, glerrors.New(glerrors.KindInvalid, "the photo is empty")
	}
	if len(data) > MaxPhotoBytes {
		return Photo{}, glerrors.New(glerrors.KindInvalid, "the photo is larger than 2 MB")
	}
	switch {
	case bytes.HasPrefix(data, jpegMagic):
		return Photo{contentType: "image/jpeg", data: data}, nil
	case bytes.HasPrefix(data, pngMagic):
		return Photo{contentType: "image/png", data: data}, nil
	default:
		return Photo{}, glerrors.New(glerrors.KindInvalid, "the photo must be a JPG or PNG image")
	}
}

// ContentType is image/jpeg or image/png, as the bytes say.
func (p Photo) ContentType() string { return p.contentType }

// Data is the image itself.
func (p Photo) Data() []byte { return p.data }
