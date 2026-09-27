// Package pdffont embeds the TrueType faces every PDF the gateway renders
// uses (receipts, report cards): Poppins, the web's display face, under the SIL
// Open Font License (fonts/OFL.txt). gopdf has no built-in faces, and a PDF
// must print "García" and "ñ" exactly (ADR 0017).
package pdffont

import (
	_ "embed"
	"fmt"

	"github.com/signintech/gopdf"
)

// The family names a document selects the faces by.
const (
	Regular = "poppins"
	Bold    = "poppins-bold"
)

var (
	//go:embed fonts/Poppins-Regular.ttf
	regular []byte
	//go:embed fonts/Poppins-SemiBold.ttf
	semiBold []byte
)

// Register adds both faces to pdf under Regular and Bold.
func Register(pdf *gopdf.GoPdf) error {
	if err := pdf.AddTTFFontData(Regular, regular); err != nil {
		return fmt.Errorf("loading the regular face: %w", err)
	}
	if err := pdf.AddTTFFontData(Bold, semiBold); err != nil {
		return fmt.Errorf("loading the bold face: %w", err)
	}
	return nil
}
