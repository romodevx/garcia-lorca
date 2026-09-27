package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Both skip paths of the ownership rule report what they skipped: withoutHeld
// (a mark another group's list held when the plan was made) and asStored (the
// ON CONFLICT ... WHERE backstop: a row another list committed after the plan).
func TestSkippedMarksAreReportedAsHeldElsewhere(t *testing.T) {
	mark := func(enrollmentID string) service.AttendanceRecord {
		return service.AttendanceRecord{ID: "new-" + enrollmentID, EnrollmentID: enrollmentID, Status: "absent"}
	}
	tests := []struct {
		name     string
		planned  service.ListChanges
		held     map[string]bool
		stored   map[string]storedMark
		wantHeld []string
		wantIns  int
	}{
		{
			name:    "nothing held, everything stored",
			planned: service.ListChanges{Inserts: []service.AttendanceRecord{mark("a"), mark("b")}},
			stored:  map[string]storedMark{"a": {ID: "new-a", EnrollmentID: "a", Inserted: true}, "b": {ID: "new-b", EnrollmentID: "b", Inserted: true}},
			wantIns: 2,
		},
		{
			name:     "held at plan time: dropped before the upsert",
			planned:  service.ListChanges{Inserts: []service.AttendanceRecord{mark("a"), mark("b")}},
			held:     map[string]bool{"a": true},
			stored:   map[string]storedMark{"b": {ID: "new-b", EnrollmentID: "b", Inserted: true}},
			wantHeld: []string{"a"},
			wantIns:  1,
		},
		{
			name:     "claimed concurrently: the upsert backstop skipped it",
			planned:  service.ListChanges{Inserts: []service.AttendanceRecord{mark("a"), mark("b")}},
			stored:   map[string]storedMark{"a": {ID: "new-a", EnrollmentID: "a", Inserted: true}},
			wantHeld: []string{"b"},
			wantIns:  1,
		},
		{
			name:     "both paths at once, reported ascending",
			planned:  service.ListChanges{Inserts: []service.AttendanceRecord{mark("c"), mark("b"), mark("a")}},
			held:     map[string]bool{"c": true},
			stored:   map[string]storedMark{},
			wantHeld: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asStored(withoutHeld(tt.planned, tt.held), tt.stored)
			assert.Equal(t, tt.wantHeld, got.HeldElsewhere)
			assert.Len(t, got.Inserts, tt.wantIns)
		})
	}
}
