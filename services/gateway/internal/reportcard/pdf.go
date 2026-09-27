// Package reportcard renders a report card (boleta, T7) as a PDF: the school's
// internal format, not the SEP one. The report card itself is the frozen
// snapshot the grades service issued when the period closed; this package only
// lays it out, with the school's details from config (ADR 0020). It mirrors
// internal/receipt: gopdf, the embedded Poppins faces, a letter page.
package reportcard

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/signintech/gopdf"
	"golang.org/x/text/unicode/norm"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/pdffont"
)

// Document is what a report card PDF prints: the snapshot and the school.
type Document struct {
	School client.SchoolProfile
	Card   client.ReportCard
}

const (
	pageMargin = 48.0
	// letterShort and letterLong are the sides of a US Letter page, in points.
	letterShort  = 612.0
	letterLong   = 792.0
	monthWidth   = 58.0
	averageWidth = 64.0
	// minSubjectWidth is the narrowest the subject column may get: the month
	// columns give way before a subject name is squeezed into one-word lines.
	minSubjectWidth = 160.0
	// maxPortraitMonths is the longest term a portrait page holds; a longer
	// one (config caps a term at six months) prints landscape.
	maxPortraitMonths = 4
	rowHeight         = 18.0
	bodySize          = 10.0
	smallSize         = 9.0
	titleSize         = 14.0
	schoolSize        = 16.0
	lineGap           = 6.0
	noGrade           = "—"
	hundredths        = 100
)

// spanishMonths names the months as the school writes them.
var (
	monthShort = [...]string{"Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"}
	monthLong  = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
)

// layout is the page geometry of one report card, decided by how many months
// its term has.
type layout struct {
	landscape    bool
	pageWidth    float64
	pageHeight   float64
	contentWidth float64
	bottom       float64
	monthWidth   float64
	subjectWidth float64
}

// layoutFor turns the page landscape past maxPortraitMonths and keeps the
// subject column at least minSubjectWidth wide: when the months would crowd
// it, they share what is left between the subject and the average columns.
func layoutFor(months int) layout {
	l := layout{pageWidth: letterShort, pageHeight: letterLong}
	if months > maxPortraitMonths {
		l.landscape, l.pageWidth, l.pageHeight = true, letterLong, letterShort
	}
	l.contentWidth = l.pageWidth - 2*pageMargin
	l.bottom = l.pageHeight - pageMargin
	l.monthWidth = monthWidth
	l.subjectWidth = l.contentWidth - float64(months)*monthWidth - averageWidth
	if l.subjectWidth < minSubjectWidth {
		l.subjectWidth = minSubjectWidth
		l.monthWidth = (l.contentWidth - minSubjectWidth - averageWidth) / float64(months)
	}
	return l
}

// PDF renders the report card on a letter page, landscape for a term longer
// than four months (more pages when its subjects do not fit one).
func PDF(d Document) ([]byte, error) {
	l := layoutFor(len(d.Card.Months))
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: l.pageWidth, H: l.pageHeight}})
	pdf.AddPage()
	if err := pdffont.Register(pdf); err != nil {
		return nil, err
	}
	w := &writer{pdf: pdf, l: l, y: pageMargin}
	w.header(d)
	w.student(d.Card)
	w.table(d.Card)
	w.foot(d.Card)
	if w.err != nil {
		return nil, fmt.Errorf("rendering report card %s: %w", d.Card.ID, w.err)
	}
	var out bytes.Buffer
	if _, err := pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("writing report card %s: %w", d.Card.ID, err)
	}
	return out.Bytes(), nil
}

// Filename is the download name: boleta-<matrícula>-<period>-v<version>.pdf,
// ASCII only (accents folded), so every browser and file system keeps it.
func Filename(c client.ReportCard) string {
	who := c.EnrollmentNumber
	if who == "" {
		who = c.StudentID
	}
	return fmt.Sprintf("boleta-%s-%s-v%d.pdf", slug(who), slug(c.TermName), c.Version)
}

// slug folds accents, lower-cases and joins the letters and digits with dashes.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
	}
	return b.String()
}

// writer keeps the first error, so the layout reads as a list of steps.
type writer struct {
	pdf *gopdf.GoPdf
	l   layout
	y   float64
	err error
	// tableHeader reprints the table's header row; set while the table's
	// body is printing, so a page break repeats it on the new page.
	tableHeader func()
}

func (w *writer) header(d Document) {
	s := d.School
	w.text(pdffont.Bold, schoolSize, s.Name)
	w.text(pdffont.Regular, smallSize, s.LegalName)
	w.text(pdffont.Regular, smallSize, joinNonEmpty(" · ", prefixed("RFC ", s.RFC), prefixed("CCT ", s.CCT)))
	w.text(pdffont.Regular, smallSize, joinNonEmpty(" · ", s.Address, prefixed("Tel. ", s.Phone)))
	w.rule()
	w.text(pdffont.Bold, titleSize, joinNonEmpty(" — ", "Boleta de calificaciones", d.Card.TermName))
	if d.Card.Version > 1 {
		w.text(pdffont.Bold, bodySize, fmt.Sprintf("Versión %d — reemplaza a la anterior", d.Card.Version))
	}
}

func (w *writer) student(c client.ReportCard) {
	w.text(pdffont.Regular, bodySize, prefixed("Alumno: ", c.StudentName))
	w.text(pdffont.Regular, bodySize, prefixed("Matrícula: ", c.EnrollmentNumber))
	w.text(pdffont.Regular, bodySize, prefixed("Ciclo escolar: ", c.CycleName))
	w.text(pdffont.Regular, bodySize, prefixed("Nivel y grupo: ", joinNonEmpty(" · ", c.LevelName, c.GroupLabel)))
	w.rule()
}

// table prints subject × month and the trimester grade, then the general
// average. A page break inside the body repeats the header row on top.
func (w *writer) table(c client.ReportCard) {
	labels := monthLabels(c.Months)
	w.tableHeader = func() { w.row(pdffont.Bold, "Materia", labels, "Promedio") }
	w.breakPage(2 * rowHeight)
	w.tableHeader()
	for _, l := range c.Lines {
		values := make([]string, 0, len(c.Months))
		for _, m := range c.Months {
			values = append(values, orDash(l.MonthValues[m]))
		}
		w.bodyRow(pdffont.Regular, l.SubjectName, values, orDash(l.TermAverage))
	}
	w.tableHeader = nil
	w.rule()
	averageX := pageMargin + w.l.contentWidth - averageWidth
	w.breakPage(titleSize + lineGap)
	w.cell(pdffont.Bold, titleSize-1, "Promedio general", pageMargin, w.l.contentWidth-averageWidth, gopdf.Left)
	w.cell(pdffont.Bold, titleSize-1, orDash(c.GeneralAverage), averageX, averageWidth, gopdf.Right)
	w.y += titleSize + lineGap
}

// bodyRow measures the row's wrapped subject name first; a row that would
// cross the bottom margin starts a new page, under the repeated header.
func (w *writer) bodyRow(family, subject string, values []string, average string) {
	need := w.rowLines(family, subject) * rowHeight
	if w.err == nil && w.y+need > w.l.bottom {
		w.newPage()
		if w.tableHeader != nil {
			w.tableHeader()
		}
	}
	w.row(family, subject, values, average)
}

// rowLines is how many lines the subject name wraps to in its column (≥ 1).
func (w *writer) rowLines(family, subject string) float64 {
	if w.err != nil || subject == "" {
		return 1
	}
	if err := w.pdf.SetFont(family, "", bodySize); err != nil {
		w.err = err
		return 1
	}
	lines, err := w.pdf.SplitTextWithWordWrap(subject, w.l.subjectWidth-lineGap)
	if err != nil {
		w.err = err
		return 1
	}
	return max(1, float64(len(lines)))
}

// row prints one table row where it stands; the subject name wraps inside
// its column. Callers decide the page (bodyRow, table).
func (w *writer) row(family, subject string, values []string, average string) {
	x := pageMargin + w.l.subjectWidth
	for _, v := range values {
		w.cell(family, bodySize, v, x, w.l.monthWidth, gopdf.Center)
		x += w.l.monthWidth
	}
	w.cell(family, bodySize, average, x, averageWidth, gopdf.Right)
	taken := w.wrapped(family, bodySize, subject, pageMargin, w.l.subjectWidth-lineGap)
	w.y += max(1, taken) * rowHeight
}

// foot prints the scale and the issue date. The school's legend is the
// receipt's footer ("Gracias por su pago…"), so it never prints here.
func (w *writer) foot(c client.ReportCard) {
	w.y += lineGap
	s := c.Scale
	w.text(pdffont.Regular, smallSize, fmt.Sprintf("Calificación mínima aprobatoria: %s · Escala de %s a %s",
		decimal(s.PassingHundredths), decimal(s.MinHundredths), decimal(s.MaxHundredths)))
	w.text(pdffont.Regular, smallSize, prefixed("Emitida el ", spanishDate(c.IssuedAt)))
}

// --- primitives ----------------------------------------------------------------

func (w *writer) breakPage(need float64) {
	if w.err != nil || w.y+need <= w.l.bottom {
		return
	}
	w.newPage()
}

func (w *writer) newPage() {
	w.pdf.AddPage()
	w.y = pageMargin
}

func (w *writer) text(family string, size float64, s string) {
	if s == "" {
		return
	}
	w.breakPage(size + lineGap)
	w.y += w.wrapped(family, size, s, pageMargin, w.l.contentWidth) * (size + lineGap)
}

// wrapped writes s word-wrapped in a box, starting at the current height, and
// returns how many lines it took (0 for nothing), leaving y where it was.
func (w *writer) wrapped(family string, size float64, s string, x, width float64) float64 {
	if w.err != nil || s == "" {
		return 0
	}
	if err := w.pdf.SetFont(family, "", size); err != nil {
		w.err = err
		return 0
	}
	lines, err := w.pdf.SplitTextWithWordWrap(s, width)
	if err != nil {
		w.err = err
		return 0
	}
	top := w.y
	for i, line := range lines {
		w.y = top + float64(i)*(size+lineGap)
		w.cell(family, size, line, x, width, gopdf.Left)
	}
	w.y = top
	return float64(len(lines))
}

func (w *writer) cell(family string, size float64, s string, x, width float64, align int) {
	if w.err != nil || s == "" {
		return
	}
	if err := w.pdf.SetFont(family, "", size); err != nil {
		w.err = err
		return
	}
	w.pdf.SetXY(x, w.y)
	w.err = w.pdf.CellWithOption(&gopdf.Rect{W: width, H: size + 2}, s, gopdf.CellOption{Align: align | gopdf.Top})
}

func (w *writer) rule() {
	if w.err != nil {
		return
	}
	w.y += 4
	w.pdf.SetLineWidth(0.6)
	w.pdf.Line(pageMargin, w.y, pageMargin+w.l.contentWidth, w.y)
	w.y += 10
}

// --- text ----------------------------------------------------------------------

// monthLabels names "2026-09" as "Sep 2026"; a malformed month prints as it came.
func monthLabels(months []string) []string {
	out := make([]string, 0, len(months))
	for _, m := range months {
		t, err := time.Parse("2006-01", m)
		if err != nil {
			out = append(out, m)
			continue
		}
		out = append(out, monthShort[t.Month()-1]+" "+strconv.Itoa(t.Year()))
	}
	return out
}

// spanishDate is "1 de diciembre de 2026" (the UTC day, the platform's school day).
func spanishDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	u := t.UTC()
	return fmt.Sprintf("%d de %s de %d", u.Day(), monthLong[u.Month()-1], u.Year())
}

// decimal prints hundredths as the shortest exact decimal (600 → "6", 950 → "9.5").
func decimal(h int) string {
	whole, frac := h/hundredths, h%hundredths
	if frac == 0 {
		return strconv.Itoa(whole)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%02d", whole, frac), "0")
}

func orDash(s string) string {
	if s == "" {
		return noGrade
	}
	return s
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
