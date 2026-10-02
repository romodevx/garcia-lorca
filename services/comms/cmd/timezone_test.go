package main

import "testing"

// The school's zone decides what "today" is for the birthday greeting (T11).
func TestSchoolLocation(t *testing.T) {
	t.Setenv("SCHOOL_TIMEZONE", "")
	loc, err := schoolLocation()
	if err != nil || loc.String() != "America/Mexico_City" {
		t.Errorf("default = %v %v, want America/Mexico_City", loc, err)
	}

	t.Setenv("SCHOOL_TIMEZONE", "America/Tijuana")
	if loc, err := schoolLocation(); err != nil || loc.String() != "America/Tijuana" {
		t.Errorf("override = %v %v", loc, err)
	}

	// A misspelt zone fails startup: guessing UTC would greet a day early every
	// evening after 18:00.
	t.Setenv("SCHOOL_TIMEZONE", "Mexico/Guadalajarra")
	if _, err := schoolLocation(); err == nil {
		t.Error("want an error for an unknown zone")
	}
}
