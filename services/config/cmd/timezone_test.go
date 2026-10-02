package main

import "testing"

// The timetable reads "today" (has the cycle started? which changes are still
// ahead?) on the school's calendar: Mexico City unless SCHOOL_TIMEZONE says
// otherwise, and an unknown zone fails startup (T16).
func TestSchoolLocation(t *testing.T) {
	loc, err := schoolLocation()
	if err != nil || loc.String() != "America/Mexico_City" {
		t.Fatalf("default zone = %v, %v; want America/Mexico_City", loc, err)
	}
	t.Setenv("SCHOOL_TIMEZONE", "Not/AZone")
	if _, err := schoolLocation(); err == nil {
		t.Fatal("an unknown zone must fail")
	}
}
