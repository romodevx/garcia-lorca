package reportcard

import (
	"strings"
	"testing"

	"github.com/signintech/gopdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/pdffont"
)

// Item 6: the subject column never shrinks under its minimum; the month
// columns give way instead, and more than four months turn the page.
func TestLayoutKeepsTheSubjectColumn(t *testing.T) {
	for n := 0; n <= 12; n++ {
		l := layoutFor(n)
		assert.Equal(t, n > maxPortraitMonths, l.landscape, "%d months", n)
		assert.GreaterOrEqual(t, l.subjectWidth, minSubjectWidth, "%d months", n)
		assert.InDelta(t, l.contentWidth, l.subjectWidth+float64(n)*l.monthWidth+averageWidth, 0.001, "%d months fill the width", n)
		assert.Positive(t, l.monthWidth)
	}
	assert.Greater(t, layoutFor(6).pageWidth, layoutFor(6).pageHeight)
}

// Item 7: a row is measured before it is placed; one that would cross the
// bottom margin starts a new page, under a repeated header, instead.
func TestARowThatDoesNotFitStartsANewPage(t *testing.T) {
	pdf := &gopdf.GoPdf{}
	l := layoutFor(3)
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: l.pageWidth, H: l.pageHeight}})
	pdf.AddPage()
	require.NoError(t, pdffont.Register(pdf))
	w := &writer{pdf: pdf, l: l, y: l.bottom - 2.5*rowHeight}
	w.tableHeader = func() { w.row(pdffont.Bold, "Materia", []string{"a", "b", "c"}, "Promedio") }
	long := strings.Repeat("Formación Cívica y Ética ", 6)
	w.bodyRow(pdffont.Regular, long, []string{"9", "9", "9"}, "9.0")
	require.NoError(t, w.err)
	assert.Equal(t, 2, pdf.GetNumberOfPages(), "the row moved to a new page")
	assert.LessOrEqual(t, w.y, l.bottom, "nothing printed past the bottom margin")
	assert.Greater(t, w.y, pageMargin+2*rowHeight, "header and the row sit on the new page")
}
