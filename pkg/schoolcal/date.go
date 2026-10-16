package schoolcal

import "time"

// Date is the calendar day an instant falls on in the school's zone, held at
// midnight UTC so it compares exactly with a parsed YYYY-MM-DD or a stored
// date column. The school's day is America/Mexico_City's (SCHOOL_TIMEZONE),
// never the UTC one: at 19:00 on 31 Oct Morelia is still on 31 Oct (ADR 0035).
//
// The one implementation every service uses; copies of this rule drifted
// apart before (finance read the UTC day long after students stopped).
func Date(t time.Time, zone *time.Location) time.Time {
	y, m, d := t.In(zone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
