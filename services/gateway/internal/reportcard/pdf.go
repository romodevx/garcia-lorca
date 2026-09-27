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
	pageMargin   = 48.0
	pageWidth    = 612.0 // US Letter, in points
	pageHeight   = 792.0
	contentWidth = pageWidth - 2*pageMargin
	monthWidth   = 58.0
	averageWidth = 64.0
	rowHeight    = 18.0
	bodySize     = 10.0
	smallSize    = 9.0
	titleSize    = 14.0
	schoolSize   = 16.0
	lineGap      = 6.0
	bottomLimit  = pageHeight - pageMargin
	noGrade      = "—"
	hundredths   = 100
)

// spanishMonths names the months as the school writes them.
var (
	monthShort = [...]string{"Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"}
	monthLong  = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
)

// PDF renders the report card on a letter page (more pages when its subjects
// do not fit one).
func PDF(d Document) ([]byte, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeLetter})
	pdf.AddPage()
	if err := pdffont.Register(pdf); err != nil {
		return nil, err
	}
	w := &writer{pdf: pdf, y: pageMargin}
	w.header(d)
	w.student(d.Card)
	w.table(d.Card)
	w.foot(d)
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
	y   float64
	err error
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
// average. The subject column takes what the months and the average leave.
func (w *writer) table(c client.ReportCard) {
	subjectWidth := contentWidth - float64(len(c.Months))*monthWidth - averageWidth
	w.row(pdffont.Bold, "Materia", monthLabels(c.Months), "Promedio", subjectWidth)
	for _, l := range c.Lines {
		values := make([]string, 0, len(c.Months))
		for _, m := range c.Months {
			values = append(values, orDash(l.MonthValues[m]))
		}
		w.row(pdffont.Regular, l.SubjectName, values, orDash(l.TermAverage), subjectWidth)
	}
	w.rule()
	w.cell(pdffont.Bold, titleSize-1, "Promedio general", pageMargin, contentWidth-averageWidth, gopdf.Left)
	w.cell(pdffont.Bold, titleSize-1, orDash(c.GeneralAverage), pageMargin+contentWidth-averageWidth, averageWidth, gopdf.Right)
	w.y += titleSize + lineGap
}

// row prints one table row; the subject name wraps inside its column.
func (w *writer) row(family, subject string, values []string, average string, subjectWidth float64) {
	w.breakPage(rowHeight * 2)
	x := pageMargin + subjectWidth
	for _, v := range values {
		w.cell(family, bodySize, v, x, monthWidth, gopdf.Center)
		x += monthWidth
	}
	w.cell(family, bodySize, average, x, averageWidth, gopdf.Right)
	taken := w.wrapped(family, bodySize, subject, pageMargin, subjectWidth-lineGap)
	w.y += max(1, taken) * rowHeight
}

func (w *writer) foot(d Document) {
	w.y += lineGap
	s := d.Card.Scale
	w.text(pdffont.Regular, smallSize, fmt.Sprintf("Calificación mínima aprobatoria: %s · Escala de %s a %s",
		decimal(s.PassingHundredths), decimal(s.MinHundredths), decimal(s.MaxHundredths)))
	w.text(pdffont.Regular, smallSize, prefixed("Emitida el ", spanishDate(d.Card.IssuedAt)))
	w.y += lineGap
	w.text(pdffont.Regular, smallSize, d.School.Legend)
}

// --- primitives ----------------------------------------------------------------

func (w *writer) breakPage(need float64) {
	if w.err != nil || w.y+need <= bottomLimit {
		return
	}
	w.pdf.AddPage()
	w.y = pageMargin
}

func (w *writer) text(family string, size float64, s string) {
	if s == "" {
		return
	}
	w.breakPage(size + lineGap)
	w.y += w.wrapped(family, size, s, pageMargin, contentWidth) * (size + lineGap)
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
	w.pdf.Line(pageMargin, w.y, pageMargin+contentWidth, w.y)
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
