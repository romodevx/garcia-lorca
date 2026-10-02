package attendancereport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/attendancereport"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

func day(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

var names = map[string]string{"stu-1": "GARCÍA LÓPEZ ANA", "stu-2": "PÉREZ RUIZ BETO"}

func sample() attendancereport.Report {
	sum := client.AttendanceSummary{ListsTaken: 20, Counts: []client.AttendanceCount{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Absent: 1, Late: 3, Excused: 1},
		{EnrollmentID: "enr-2", StudentID: "stu-2"},
	}}
	group := attendancereport.Group{ID: "grp-1", Label: "1° SECUNDARIA · A"}
	return attendancereport.Build(group, day("2026-09-01"), day("2026-09-30"), sum, func(id string) string { return names[id] })
}

// Present is what the exceptions leave of the lists taken (default-present).
func TestBuildDerivesPresentAndTotals(t *testing.T) {
	rep := sample()

	assert.Equal(t, "grp-1", rep.GroupID)
	assert.Equal(t, "1° SECUNDARIA · A", rep.GroupLabel)
	assert.Equal(t, "2026-09-01", rep.From)
	assert.Equal(t, "2026-09-30", rep.To)
	assert.Equal(t, 20, rep.ListsTaken)
	require.Len(t, rep.Rows, 2)
	assert.Equal(t, attendancereport.Row{EnrollmentID: "enr-1", StudentID: "stu-1", Name: "GARCÍA LÓPEZ ANA",
		Counts: attendancereport.Counts{Present: 15, Absent: 1, Late: 3, Excused: 1}}, rep.Rows[0])
	assert.Equal(t, attendancereport.Counts{Present: 20}, rep.Rows[1].Counts)
	assert.Equal(t, attendancereport.Counts{Present: 35, Absent: 1, Late: 3, Excused: 1}, rep.Totals)
}

// A legacy mark on a day the group took no list can leave more exceptions
// than lists: present never goes below zero.
func TestBuildNeverCountsNegativePresence(t *testing.T) {
	sum := client.AttendanceSummary{ListsTaken: 1, Counts: []client.AttendanceCount{{EnrollmentID: "e", StudentID: "s", Absent: 2}}}
	rep := attendancereport.Build(attendancereport.Group{ID: "g"}, day("2026-09-01"), day("2026-09-30"), sum, func(string) string { return "" })
	assert.Equal(t, 0, rep.Rows[0].Present)
}

func TestDocumentLaysOutTheReport(t *testing.T) {
	doc := attendancereport.Document(sample(), []string{"Colegio García Lorca"})

	assert.Equal(t, "Reporte de asistencia", doc.Title)
	assert.Equal(t, "1° SECUNDARIA · A · Del 1 de septiembre de 2026 al 30 de septiembre de 2026", doc.Subtitle)
	assert.Equal(t, []string{"Colegio García Lorca"}, doc.Header)
	require.Len(t, doc.Tables, 2)

	summary := doc.Tables[0]
	assert.Equal(t, []report.Cell{report.TextCell("Listas tomadas"), report.CountCell(20)}, summary.Rows[0])

	pupils := doc.Tables[1]
	headers := make([]string, 0, len(pupils.Columns))
	for _, c := range pupils.Columns {
		headers = append(headers, c.Header)
	}
	assert.Equal(t, []string{"Alumno", "Presentes", "Ausentes", "Retardos", "Justificadas"}, headers)
	assert.Equal(t, []report.Cell{report.TextCell("GARCÍA LÓPEZ ANA"), report.CountCell(15), report.CountCell(1), report.CountCell(3), report.CountCell(1)}, pupils.Rows[0])
	assert.Equal(t, []report.Cell{report.TextCell("Total"), report.CountCell(35), report.CountCell(1), report.CountCell(3), report.CountCell(1)}, pupils.Total)
}

// The engine renders it: a real PDF and a real workbook come out.
func TestDocumentRenders(t *testing.T) {
	doc := attendancereport.Document(sample(), []string{"Colegio García Lorca"})
	pdf, err := report.PDF(doc)
	require.NoError(t, err)
	assert.Equal(t, "%PDF", string(pdf[:4]))
	xlsx, err := report.XLSX(doc)
	require.NoError(t, err)
	assert.Equal(t, "PK", string(xlsx[:2]))
}
