package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// dayStatus is the owner-approved day rule over every list taken that day.
func TestDayStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"all present", []string{"present", "present"}, "present"},
		{"present + late", []string{"present", "late"}, "late"},
		{"late + excused", []string{"excused", "late"}, "late"},
		{"present + excused", []string{"present", "excused"}, "excused"},
		{"all excused", []string{"excused", "excused"}, "excused"},
		{"late + excused + present", []string{"present", "excused", "late"}, "late"},
		{"absent in some", []string{"absent", "present"}, "partial"},
		{"absent + late", []string{"late", "absent"}, "partial"},
		{"absent in all", []string{"absent", "absent"}, "absent"},
		{"single absent", []string{"absent"}, "absent"},
		{"single late", []string{"late"}, "late"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dayStatus(tt.statuses))
		})
	}
}
