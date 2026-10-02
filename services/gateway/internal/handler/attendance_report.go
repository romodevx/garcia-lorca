package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/attendancereport"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

// maxReportDays bounds an attendance report's range: a school year, leap day
// included. The attendance service enforces the same bound (its
// maxSummaryRangeDays); checking it here too answers the person in Spanish.
const maxReportDays = 366

// AttendanceSummarizer is the attendance surface the report reads. The Connect
// AttendanceClient implements it.
type AttendanceSummarizer interface {
	SummarizeAttendance(ctx context.Context, groupID, from, to string) (client.AttendanceSummary, error)
}

// AttendanceReportHandler serves the attendance report (T12): a group's lists
// over a date range with each pupil's present, absent, late and excused counts,
// as JSON and exported to PDF and Excel through the T10 report engine. The
// routes run behind module_attendance, auth and attendance:read — the full
// report is the director's (teachers' own-group report is deferred).
type AttendanceReportHandler struct {
	attendance AttendanceSummarizer
	groups     AttendanceGroupReader
	students   StudentNamesReader
	profiles   SchoolProfileReader
}

// NewAttendanceReportHandler wires the handler to attendance (the counts),
// config (the group's label and the school's heading) and students (names).
func NewAttendanceReportHandler(attendance AttendanceSummarizer, groups AttendanceGroupReader, students StudentNamesReader, profiles SchoolProfileReader) *AttendanceReportHandler {
	return &AttendanceReportHandler{attendance: attendance, groups: groups, students: students, profiles: profiles}
}

// reportRange parses ?from&to: two ISO days, in order, at most maxReportDays.
func reportRange(r *http.Request) (time.Time, time.Time, string) {
	from, okFrom := parseListDate(r.URL.Query().Get("from"))
	to, okTo := parseListDate(r.URL.Query().Get("to"))
	switch {
	case !okFrom || !okTo:
		return time.Time{}, time.Time{}, "Indica las fechas del reporte (desde y hasta)."
	case to.Before(from):
		return time.Time{}, time.Time{}, "La fecha final es anterior a la inicial."
	case to.Sub(from) >= maxReportDays*24*time.Hour:
		return time.Time{}, time.Time{}, fmt.Sprintf("El reporte abarca como máximo %d días.", maxReportDays)
	}
	return from, to, ""
}

// reportOf validates the query, reads the group, the counts and the names, and
// builds the report; on failure it has already answered.
func (h *AttendanceReportHandler) reportOf(w http.ResponseWriter, r *http.Request) (attendancereport.Report, bool) {
	groupID := r.URL.Query().Get("groupId")
	if groupID == "" {
		writeBadRequest(w, "Elige un grupo.")
		return attendancereport.Report{}, false
	}
	from, to, refusal := reportRange(r)
	if refusal != "" {
		writeBadRequest(w, refusal)
		return attendancereport.Report{}, false
	}
	ctx := r.Context()
	group, found, err := resolveListGroup(ctx, h.groups, groupID)
	if err != nil {
		writeConnectError(w, err)
		return attendancereport.Report{}, false
	}
	if !found {
		http.NotFound(w, r)
		return attendancereport.Report{}, false
	}
	sum, err := h.attendance.SummarizeAttendance(ctx, groupID, from.Format(attendanceDateLayout), to.Format(attendanceDateLayout))
	if err != nil {
		writeConnectError(w, err)
		return attendancereport.Report{}, false
	}
	ids := make([]string, 0, len(sum.Counts))
	for _, c := range sum.Counts {
		ids = append(ids, c.StudentID)
	}
	names := studentsByIDs(ctx, h.students, ids)
	rep := attendancereport.Build(attendancereport.Group{ID: groupID, Label: group.Label}, from, to, sum, names.name)
	sortBySurname(names, rep.Rows, func(row attendancereport.Row) string { return row.StudentID })
	return rep, true
}

// Report handles GET /api/v1/attendance/reports: the report as JSON.
func (h *AttendanceReportHandler) Report(w http.ResponseWriter, r *http.Request) {
	rep, ok := h.reportOf(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rep)
}

// ReportPDF handles GET /api/v1/attendance/reports/pdf.
func (h *AttendanceReportHandler) ReportPDF(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, "pdf", "application/pdf", report.PDF)
}

// ReportXLSX handles GET /api/v1/attendance/reports/xlsx.
func (h *AttendanceReportHandler) ReportXLSX(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, "xlsx", xlsxContentType, report.XLSX)
}

func (h *AttendanceReportHandler) serve(w http.ResponseWriter, r *http.Request, extension, contentType string, render func(report.Document) ([]byte, error)) {
	// The school's heading does not depend on the report: read it alongside.
	heading := make(chan schoolHeading, 1)
	go func() { heading <- h.readHeading(r.Context()) }()
	rep, ok := h.reportOf(w, r)
	head := <-heading
	if !ok {
		return
	}
	if head.err != nil {
		writeConnectError(w, head.err)
		return
	}
	body, err := render(attendancereport.Document(rep, head.lines))
	if err != nil {
		http.Error(w, "the report could not be rendered", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("reporte-asistencia-%s-%s.%s", rep.From, rep.To, extension)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body) // the status is sent; a client that hung up is not an error to report
}

// schoolHeading is the printed heading's lines, or why they could not be read.
type schoolHeading struct {
	lines []string
	err   error
}

// readHeading is the school's lines, or a generic title before anybody filled
// in the school's details.
func (h *AttendanceReportHandler) readHeading(ctx context.Context) schoolHeading {
	school, found, err := h.profiles.GetSchoolProfile(ctx)
	if err != nil {
		return schoolHeading{err: err}
	}
	if !found {
		return schoolHeading{lines: []string{"Reporte de asistencia"}}
	}
	return schoolHeading{lines: report.SchoolHeading(school.Name, school.LegalName, school.RFC)}
}
