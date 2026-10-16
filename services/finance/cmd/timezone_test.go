package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Finance's "today" is the school's calendar day (ADR 0035): Mexico City
// unless SCHOOL_TIMEZONE says otherwise, and an unknown zone fails startup
// instead of falling back to the UTC day.
func TestSchoolLocation(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "default", env: "", want: "America/Mexico_City"},
		{name: "configured", env: "America/Monterrey", want: "America/Monterrey"},
		{name: "unknown zone fails", env: "Not/AZone", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SCHOOL_TIMEZONE", tt.env)
			loc, err := schoolLocation()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "SCHOOL_TIMEZONE")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, loc.String())
		})
	}
}
