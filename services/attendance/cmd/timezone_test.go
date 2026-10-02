package main

import "testing"

// The report's windows are read on the school's calendar: Mexico City unless
// SCHOOL_TIMEZONE says otherwise, and an unknown zone fails startup.
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
