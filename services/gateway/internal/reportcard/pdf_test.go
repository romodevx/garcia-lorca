package reportcard_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/reportcard"
)

func textOf(t *testing.T, doc []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	require.NoError(t, err, "the PDF does not open")
	plain, err := r.GetPlainText()
	require.NoError(t, err)
	var b bytes.Buffer
	_, err = io.Copy(&b, plain)
	require.NoError(t, err)
	return b.String()
}

func sampleCard() client.ReportCard {
	return client.ReportCard{
		ID: "rc-1", Version: 1, Status: "published", IssuedAt: time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC),
		StudentName: "Camila Zamora Sánchez", EnrollmentNumber: "2026-00042", GroupLabel: "1° PRIMARIA · A",
		LevelName: "Primaria", CycleName: "2026-2027", TermName: "1.er trimestre",
		Months: []string{"2026-09", "2026-10", "2026-11"}, GeneralAverage: "8.4",
		Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		Lines: []client.ReportCardLine{
			{SubjectName: "Español", Position: 1, MonthValues: map[string]string{"2026-09": "8", "2026-11": "8.5"}, TermAverage: "8.3"},
			{SubjectName: "Matemáticas", Position: 2, MonthValues: map[string]string{"2026-09": "8.4", "2026-10": "8.5"}, TermAverage: "8.5"},
			{SubjectName: "Educación Artística", Position: 3, MonthValues: map[string]string{}, TermAverage: ""},
		},
	}
}

var school = client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", RFC: "EGL150101AB1", CCT: "09PPR0001A", Legend: "Gracias por su pago."}

// T7: the report card says who, when, which period, each subject's grade per
// month and its average, the general average and the passing mark — accents
// included.
func TestThePDFSaysWhatTheReportCardSays(t *testing.T) {
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: sampleCard()})
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(doc, []byte("%PDF-")))
	text := textOf(t, doc)
	for _, want := range []string{
		"COLEGIO GARCÍA LORCA", "Boleta de calificaciones — 1.er trimestre", "Camila Zamora Sánchez", "2026-00042",
		"2026-2027", "Primaria · 1° PRIMARIA · A", "Español", "Matemáticas", "Educación Artística",
		"Sep 2026", "Oct 2026", "Nov 2026", "Promedio", "8.4", "8.5", "8.3", "Promedio general",
		"Calificación mínima aprobatoria: 6", "1 de diciembre de 2026",
	} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "reemplaza", "a first version replaces nothing")
	// QA ISSUE-003: the school's legend is the RECEIPT footer; a report card is no receipt.
	assert.NotContains(t, text, "Gracias por su pago", "the receipt footer never prints on a report card")
}

func TestASecondVersionSaysItReplacesTheFirst(t *testing.T) {
	card := sampleCard()
	card.Version = 2
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: card})
	require.NoError(t, err)
	assert.Contains(t, textOf(t, doc), "Versión 2 — reemplaza a la anterior")
}

// Many months and long subject names still fit the page: every word is there.
func TestManyMonthsAndLongNamesFit(t *testing.T) {
	card := sampleCard()
	card.Months = []string{"2026-12", "2027-01", "2027-02", "2027-03"}
	card.Lines = []client.ReportCardLine{{SubjectName: strings.Repeat("Formación Cívica y Ética ", 4) + "FIN", Position: 1,
		MonthValues: map[string]string{"2027-03": "10"}, TermAverage: "10.0"}}
	doc, err := reportcard.PDF(reportcard.Document{Card: card})
	require.NoError(t, err)
	text := textOf(t, doc)
	assert.Contains(t, text, "FIN")
	assert.Contains(t, text, "Mar 2027")
	assert.Contains(t, text, "10.0")
}

func TestFilename(t *testing.T) {
	assert.Equal(t, "boleta-2026-00042-1-er-trimestre-v1.pdf", reportcard.Filename(sampleCard()))
	card := sampleCard()
	card.EnrollmentNumber, card.StudentID, card.Version, card.TermName = "", "stu-9", 2, "Trimestre Único ñ"
	assert.Equal(t, "boleta-stu-9-trimestre-unico-n-v2.pdf", reportcard.Filename(card))
}

// pages opens the PDF and returns each page's plain text and whether it is
// landscape (MediaBox wider than tall).
func pages(t *testing.T, doc []byte) (texts []string, landscape []bool) {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	require.NoError(t, err, "the PDF does not open")
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		text, err := p.GetPlainText(nil)
		require.NoError(t, err)
		texts = append(texts, text)
		box := p.V.Key("MediaBox")
		if box.Len() == 0 {
			box = p.V.Key("Parent").Key("MediaBox")
		}
		require.Equal(t, 4, box.Len(), "page %d has a MediaBox", i)
		landscape = append(landscape, box.Index(2).Float64()-box.Index(0).Float64() > box.Index(3).Float64()-box.Index(1).Float64())
	}
	return texts, landscape
}

func sixMonthCard() client.ReportCard {
	card := sampleCard()
	card.Months = []string{"2027-01", "2027-02", "2027-03", "2027-04", "2027-05", "2027-06"}
	card.Lines = []client.ReportCardLine{
		{SubjectName: "Formación Cívica y Ética y Proyecto de Vida Comunitaria FIN", Position: 1,
			MonthValues: map[string]string{"2027-01": "9", "2027-06": "10"}, TermAverage: "9.5"},
		{SubjectName: "Matemáticas", Position: 2, MonthValues: map[string]string{"2027-03": "8.5"}, TermAverage: "8.5"},
	}
	return card
}

// A term of up to four months prints on a portrait letter page.
func TestAShortTermPrintsPortrait(t *testing.T) {
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: sampleCard()})
	require.NoError(t, err)
	_, landscape := pages(t, doc)
	assert.Equal(t, []bool{false}, landscape)
}

// Item 6: a six-month term turns the page landscape; every month label, every
// grade and the whole long subject name are there.
func TestASixMonthTermPrintsLandscape(t *testing.T) {
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: sixMonthCard()})
	require.NoError(t, err)
	texts, landscape := pages(t, doc)
	require.Len(t, texts, 1)
	assert.Equal(t, []bool{true}, landscape)
	for _, want := range []string{"Ene 2027", "Feb 2027", "Mar 2027", "Abr 2027", "May 2027", "Jun 2027",
		"FIN", "Matemáticas", "9.5", "10", "Promedio general"} {
		assert.Contains(t, texts[0], want)
	}
}

// Item 7: sixteen subjects with long names run onto a second page, and the
// table header is printed again on top of it; no subject is lost.
func TestManySubjectsBreakPagesAndRepeatTheHeader(t *testing.T) {
	card := sampleCard()
	card.Lines = nil
	for i := 1; i <= 16; i++ {
		card.Lines = append(card.Lines, client.ReportCardLine{
			SubjectName: fmt.Sprintf("Asignatura %02d de Formación Integral, Lenguajes y Saberes Científicos Comunitarios", i),
			Position:    i, MonthValues: map[string]string{"2026-09": "9"}, TermAverage: "9.0",
		})
	}
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: card})
	require.NoError(t, err)
	texts, _ := pages(t, doc)
	require.Greater(t, len(texts), 1, "sixteen long subjects do not fit one page")
	all := strings.Join(texts, "\n")
	for i := 1; i <= 16; i++ {
		assert.Contains(t, all, fmt.Sprintf("Asignatura %02d", i))
	}
	for i, text := range texts {
		if !strings.Contains(text, "Asignatura") {
			continue // a trailing page with only the footer
		}
		assert.Contains(t, text, "Materia", "page %d repeats the table header", i+1)
		assert.Contains(t, text, "Sep 2026", "page %d repeats the month labels", i+1)
	}
}
