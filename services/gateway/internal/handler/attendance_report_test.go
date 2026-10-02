package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// --- fakes ---------------------------------------------------------------------

type fakeSummarizer struct {
	sum               client.AttendanceSummary
	err               error
	gotGroup, gotFrom string
	gotTo             string
	calls             int
}

func (f *fakeSummarizer) SummarizeAttendance(_ context.Context, groupID, from, to string) (client.AttendanceSummary, error) {
	f.calls++
	f.gotGroup, f.gotFrom, f.gotTo = groupID, from, to
	return f.sum, f.err
}

type reportGroups struct{}

func (reportGroups) GetGroup(_ context.Context, id string) (client.GroupResult, error) {
	if id != "grp-1" {
		return client.GroupResult{}, connect.NewError(connect.CodeNotFound, nil)
	}
	return client.GroupResult{ID: "grp-1", GradeLevelID: "gl-1", Name: "A"}, nil
}

func (reportGroups) GetGradeLevel(context.Context, string) (client.GradeLevelResult, error) {
	return client.GradeLevelResult{ID: "gl-1", LevelID: "lvl-1", Name: "1°"}, nil
}

func (reportGroups) GetLevel(context.Context, string) (client.LevelResult, error) {
	return client.LevelResult{ID: "lvl-1", Name: "SECUNDARIA"}, nil
}

type reportStudents struct{ gotIDs []string }

func (r *reportStudents) StudentsByIDs(_ context.Context, ids []string) ([]client.StudentResult, error) {
	r.gotIDs = append([]string(nil), ids...)
	return []client.StudentResult{
		{ID: "stu-1", FirstName: "Zoe", LastName: "Zamora", MaternalLastName: "Ruiz"},
		{ID: "stu-2", FirstName: "Ana", LastName: "Álvarez", MaternalLastName: "Paz"},
	}, nil
}

type reportProfiles struct{}

func (reportProfiles) GetSchoolProfile(context.Context) (client.SchoolProfile, bool, error) {
	return client.SchoolProfile{Name: "Colegio García Lorca"}, true, nil
}

func newReportHandler(sum *fakeSummarizer) (*handler.AttendanceReportHandler, *reportStudents) {
	students := &reportStudents{}
	return handler.NewAttendanceReportHandler(sum, reportGroups{}, students, reportProfiles{}), students
}

func twoPupils() *fakeSummarizer {
	return &fakeSummarizer{sum: client.AttendanceSummary{ListsTaken: 20, Counts: []client.AttendanceCount{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Absent: 2, Late: 1},
		{EnrollmentID: "enr-2", StudentID: "stu-2", Late: 4, Excused: 1},
	}}}
}

const reportQuery = "?groupId=grp-1&from=2026-09-01&to=2026-09-30"

// --- tests -----------------------------------------------------------------------

// T12: the JSON report names the group, counts each pupil (surname first,
// sorted in Spanish) and totals them; tardies are their own column.
func TestAttendanceReportAnswersCountsBySurname(t *testing.T) {
	sum := twoPupils()
	h, students := newReportHandler(sum)
	rec := httptest.NewRecorder()
	h.Report(rec, httptest.NewRequest(http.MethodGet, "/api/v1/attendance/reports"+reportQuery, nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, [3]string{"grp-1", "2026-09-01", "2026-09-30"}, [3]string{sum.gotGroup, sum.gotFrom, sum.gotTo})
	assert.ElementsMatch(t, []string{"stu-1", "stu-2"}, students.gotIDs, "names read by id, in one read")

	var body struct {
		GroupLabel string `json:"groupLabel"`
		ListsTaken int    `json:"listsTaken"`
		Rows       []struct {
			Name                           string
			Present, Absent, Late, Excused int
		} `json:"rows"`
		Totals struct{ Present, Absent, Late, Excused int } `json:"totals"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "1° SECUNDARIA · A", body.GroupLabel)
	assert.Equal(t, 20, body.ListsTaken)
	require.Len(t, body.Rows, 2)
	assert.Equal(t, "Álvarez Paz Ana", body.Rows[0].Name, "Á sorts with A")
	assert.Equal(t, [4]int{15, 0, 4, 1}, [4]int{body.Rows[0].Present, body.Rows[0].Absent, body.Rows[0].Late, body.Rows[0].Excused})
	assert.Equal(t, [4]int{32, 2, 5, 1}, [4]int{body.Totals.Present, body.Totals.Absent, body.Totals.Late, body.Totals.Excused})
}

func TestAttendanceReportRefusesBadQueries(t *testing.T) {
	tests := []struct {
		name, query string
		want        int
	}{
		{"no group", "?from=2026-09-01&to=2026-09-30", http.StatusBadRequest},
		{"no from", "?groupId=grp-1&to=2026-09-30", http.StatusBadRequest},
		{"malformed to", "?groupId=grp-1&from=2026-09-01&to=30-09-2026", http.StatusBadRequest},
		{"reversed", "?groupId=grp-1&from=2026-09-30&to=2026-09-01", http.StatusBadRequest},
		{"longer than a year", "?groupId=grp-1&from=2025-09-01&to=2026-09-02", http.StatusBadRequest},
		{"unknown group", "?groupId=grp-x&from=2026-09-01&to=2026-09-30", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum := twoPupils()
			h, _ := newReportHandler(sum)
			rec := httptest.NewRecorder()
			h.Report(rec, httptest.NewRequest(http.MethodGet, "/api/v1/attendance/reports"+tt.query, nil))
			assert.Equal(t, tt.want, rec.Code)
			assert.Zero(t, sum.calls, "a refused query never reaches attendance")
		})
	}
}

func TestAttendanceReportForwardsAServiceRefusal(t *testing.T) {
	sum := &fakeSummarizer{err: connect.NewError(connect.CodeInvalidArgument, errors.New("the range spans more than 366 days"))}
	h, _ := newReportHandler(sum)
	rec := httptest.NewRecorder()
	h.Report(rec, httptest.NewRequest(http.MethodGet, "/api/v1/attendance/reports"+reportQuery, nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAttendanceReportExports(t *testing.T) {
	tests := []struct {
		name        string
		serve       func(*handler.AttendanceReportHandler) http.HandlerFunc
		contentType string
		magic       string
		file        string
	}{
		{"pdf", func(h *handler.AttendanceReportHandler) http.HandlerFunc { return h.ReportPDF }, "application/pdf", "%PDF", "reporte-asistencia-2026-09-01-2026-09-30.pdf"},
		{"xlsx", func(h *handler.AttendanceReportHandler) http.HandlerFunc { return h.ReportXLSX },
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "PK", "reporte-asistencia-2026-09-01-2026-09-30.xlsx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newReportHandler(twoPupils())
			rec := httptest.NewRecorder()
			tt.serve(h)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/attendance/reports/"+tt.name+reportQuery, nil))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tt.contentType, rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Header().Get("Content-Disposition"), tt.file)
			assert.Equal(t, tt.magic, rec.Body.String()[:len(tt.magic)])
		})
	}
}

// The JSON answers exactly what the contract requires of AttendanceReport.
func TestAttendanceReportMatchesTheContract(t *testing.T) {
	h, _ := newReportHandler(twoPupils())
	rec := httptest.NewRecorder()
	h.Report(rec, httptest.NewRequest(http.MethodGet, "/api/v1/attendance/reports"+reportQuery, nil))
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile(filepath.Clean(openAPIPath))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	required := doc.Components.Schemas["AttendanceReport"].Required
	require.NotEmpty(t, required)
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sort.Strings(required)
	assert.Equal(t, required, keys)
}
