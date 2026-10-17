package handler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeLabelSources struct {
	enrollments []client.Enrollment
	transfers   []client.Transfer
	listErr     error
	groups      map[string]client.GroupResult
	grades      map[string]client.GradeLevelResult
	levels      map[string]client.LevelResult
}

func (f fakeLabelSources) ListEnrollments(_ context.Context, _, _, _ string) ([]client.Enrollment, error) {
	return f.enrollments, f.listErr
}

func (f fakeLabelSources) ListTransfers(_ context.Context, _ string) ([]client.Transfer, error) {
	return f.transfers, nil
}

func (f fakeLabelSources) GetGroup(_ context.Context, id string) (client.GroupResult, error) {
	g, ok := f.groups[id]
	if !ok {
		return client.GroupResult{}, connect.NewError(connect.CodeNotFound, errors.New("no group"))
	}
	return g, nil
}

func (f fakeLabelSources) GetGradeLevel(_ context.Context, id string) (client.GradeLevelResult, error) {
	return f.grades[id], nil
}

func (f fakeLabelSources) GetLevel(_ context.Context, id string) (client.LevelResult, error) {
	return f.levels[id], nil
}

// at is a school-local instant on the given day (Morelia, UTC−6).
func at(day string) time.Time {
	t, err := time.ParseInLocation(time.DateOnly, day, morelia)
	if err != nil {
		panic(err)
	}
	return t.Add(10 * time.Hour)
}

var morelia = time.FixedZone("America/Mexico_City", -6*60*60)

// ISSUE-017: the receipt names the group the pupil was in ON THE DAY OF THE
// PAYMENT — a reprinted receipt of a withdrawn or moved pupil included.
func TestStudentGroupLabelOnTheDay(t *testing.T) {
	catalog := fakeLabelSources{
		groups: map[string]client.GroupResult{
			"g-a": {ID: "g-a", GradeLevelID: "gr-1", Name: "A"},
			"g-b": {ID: "g-b", GradeLevelID: "gr-1", Name: "B"},
			"g-2": {ID: "g-2", GradeLevelID: "gr-2", Name: "A"},
		},
		grades: map[string]client.GradeLevelResult{"gr-1": {ID: "gr-1", LevelID: "lv-p", Name: "1°"}, "gr-2": {ID: "gr-2", LevelID: "lv-p", Name: "2°"}},
		levels: map[string]client.LevelResult{"lv-p": {ID: "lv-p", Name: "Primaria"}},
	}
	const paidOn = "2026-10-05"
	cases := []struct {
		name        string
		enrollments []client.Enrollment
		transfers   []client.Transfer
		want        string
		wantFound   bool
	}{
		{"enrolled on the day", []client.Enrollment{
			{ID: "e-old", GroupID: "g-2", Status: "withdrawn", EnrolledAt: at("2025-08-25"), WithdrawnOn: "2026-07-10"},
			{ID: "e-now", GroupID: "g-a", Status: "active", EnrolledAt: at("2026-08-24")},
		}, nil, "1° Primaria · A", true},
		{"withdrawn after the payment", []client.Enrollment{
			{ID: "e-1", GroupID: "g-b", Status: "withdrawn", EnrolledAt: at("2026-08-24"), WithdrawnOn: "2026-10-12", EndedAt: at("2026-10-12")},
		}, nil, "1° Primaria · B", true},
		{"withdrawn on the day of the payment still counts", []client.Enrollment{
			{ID: "e-1", GroupID: "g-b", Status: "withdrawn", EnrolledAt: at("2026-08-24"), WithdrawnOn: paidOn},
		}, nil, "1° Primaria · B", true},
		{"moved to another group after the payment", []client.Enrollment{
			{ID: "e-1", GroupID: "g-b", Status: "active", EnrolledAt: at("2026-08-24")},
		}, []client.Transfer{
			{EnrollmentID: "e-1", FromGroupID: "g-a", ToGroupID: "g-b", MovedAt: at("2026-10-20")},
		}, "1° Primaria · A", true},
		{"moved before the payment", []client.Enrollment{
			{ID: "e-1", GroupID: "g-b", Status: "active", EnrolledAt: at("2026-08-24")},
		}, []client.Transfer{
			{EnrollmentID: "e-1", FromGroupID: "g-a", ToGroupID: "g-b", MovedAt: at("2026-09-01")},
		}, "1° Primaria · B", true},
		{"none on the day: the most recent of any status", []client.Enrollment{
			{ID: "e-old", GroupID: "g-2", Status: "withdrawn", EnrolledAt: at("2025-08-25"), WithdrawnOn: "2026-01-10"},
			{ID: "e-new", GroupID: "g-b", Status: "withdrawn", EnrolledAt: at("2026-02-01"), WithdrawnOn: "2026-06-30"},
		}, nil, "1° Primaria · B", true},
		{"no enrollment at all", nil, nil, "", false},
		{"a group the catalog no longer holds", []client.Enrollment{
			{ID: "e-1", GroupID: "g-x", Status: "active", EnrolledAt: at("2026-08-24")},
		}, nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := catalog
			src.enrollments, src.transfers = tc.enrollments, tc.transfers
			got, found, err := handler.NewStudentGroupLabeler(src, src, morelia).StudentGroupLabel(context.Background(), "stu-1", paidOn)
			if err != nil || found != tc.wantFound || got != tc.want {
				t.Errorf("= %q, %v, %v; want %q, %v", got, found, err, tc.want, tc.wantFound)
			}
		})
	}
}

func TestStudentGroupLabelWhenStudentsCannotAnswer(t *testing.T) {
	src := fakeLabelSources{listErr: errors.New("down")}
	if _, _, err := handler.NewStudentGroupLabeler(src, src, morelia).StudentGroupLabel(context.Background(), "stu-1", "2026-10-05"); err == nil {
		t.Error("want the students error")
	}
}
