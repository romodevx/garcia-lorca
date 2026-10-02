// Package attendancereport assembles the attendance report (T12): a group's
// lists over a date range and, per pupil, how many found them present, absent,
// late (retardo) or excused — as JSON for the screen and as a report.Document
// the PDF and Excel renderers print. Attendance counts the exceptions; this
// package only derives presence from them and arranges the result.
package attendancereport

import (
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

const dayLayout = "2006-01-02"

// Counts is how many lists found a pupil (or the group) in each state.
type Counts struct {
	Present int `json:"present"`
	Absent  int `json:"absent"`
	Late    int `json:"late"`
	Excused int `json:"excused"`
}

func (c Counts) plus(o Counts) Counts {
	return Counts{Present: c.Present + o.Present, Absent: c.Absent + o.Absent, Late: c.Late + o.Late, Excused: c.Excused + o.Excused}
}

// Row is one pupil's line.
type Row struct {
	EnrollmentID string `json:"enrollmentId"`
	StudentID    string `json:"studentId"`
	Name         string `json:"name"`
	Counts
}

// Group is the group a report is about, as the screen names it.
type Group struct {
	ID    string
	Label string
}

// Report is a group's attendance over a range, as the screen shows it.
type Report struct {
	GroupID    string `json:"groupId"`
	GroupLabel string `json:"groupLabel"`
	From       string `json:"from"`
	To         string `json:"to"`
	ListsTaken int    `json:"listsTaken"`
	Rows       []Row  `json:"rows"`
	Totals     Counts `json:"totals"`

	period string // "Del … al …", for the printed heading
}

// Build derives each pupil's presence from the lists taken and their
// exceptions (default-present model), keeping the summary's order; nameOf names
// a pupil by student id ("" when unknown).
func Build(group Group, from, to time.Time, sum client.AttendanceSummary, nameOf func(studentID string) string) Report {
	rep := Report{
		GroupID: group.ID, GroupLabel: group.Label,
		From: from.Format(dayLayout), To: to.Format(dayLayout),
		ListsTaken: sum.ListsTaken, Rows: make([]Row, 0, len(sum.Counts)),
		period: report.RangeLabel(from, to),
	}
	for _, c := range sum.Counts {
		counts := Counts{Absent: c.Absent, Late: c.Late, Excused: c.Excused}
		// A legacy mark on a day the group took no list can outnumber the lists.
		counts.Present = max(0, sum.ListsTaken-c.Absent-c.Late-c.Excused)
		rep.Rows = append(rep.Rows, Row{EnrollmentID: c.EnrollmentID, StudentID: c.StudentID, Name: nameOf(c.StudentID), Counts: counts})
		rep.Totals = rep.Totals.plus(counts)
	}
	return rep
}

// Document lays the report out for the PDF and the workbook, under heading
// (the school's lines).
func Document(rep Report, heading []string) report.Document {
	subtitle := rep.period
	if rep.GroupLabel != "" {
		subtitle = rep.GroupLabel + " · " + rep.period
	}
	return report.Document{
		Title:    "Reporte de asistencia",
		Subtitle: subtitle,
		Header:   heading,
		Tables:   []report.Table{summaryTable(rep), pupilsTable(rep)},
	}
}

func summaryTable(rep Report) report.Table {
	return report.Table{
		Title:   "Resumen",
		Columns: []report.Column{{Header: "Concepto", Kind: report.Text, Weight: 3}, {Header: "Total", Kind: report.Count, Weight: 1}},
		Rows: [][]report.Cell{
			{report.TextCell("Listas tomadas"), report.CountCell(int64(rep.ListsTaken))},
			{report.TextCell("Alumnos"), report.CountCell(int64(len(rep.Rows)))},
		},
	}
}

func countCells(label string, c Counts) []report.Cell {
	return []report.Cell{
		report.TextCell(label), report.CountCell(int64(c.Present)), report.CountCell(int64(c.Absent)),
		report.CountCell(int64(c.Late)), report.CountCell(int64(c.Excused)),
	}
}

func pupilsTable(rep Report) report.Table {
	t := report.Table{
		Title: "Por alumno",
		Columns: []report.Column{
			{Header: "Alumno", Kind: report.Text, Weight: 3},
			{Header: "Presentes", Kind: report.Count, Weight: 1}, {Header: "Ausentes", Kind: report.Count, Weight: 1},
			{Header: "Retardos", Kind: report.Count, Weight: 1}, {Header: "Justificadas", Kind: report.Count, Weight: 1},
		},
		Rows:  make([][]report.Cell, 0, len(rep.Rows)),
		Total: countCells("Total", rep.Totals),
	}
	for _, r := range rep.Rows {
		t.Rows = append(t.Rows, countCells(r.Name, r.Counts))
	}
	return t
}
