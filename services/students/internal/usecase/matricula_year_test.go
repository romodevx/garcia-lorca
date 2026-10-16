package usecase_test

import (
	"context"
	"testing"
	"time"
)

// T5b review: the matrícula's year is the school's calendar year of the
// registration. At 19:00 on 31 December 2026 in Morelia it is already 2027 in
// UTC — the year the database used to stamp — and the pupil is still 2026.
var matriculaNights = []struct {
	name string
	at   time.Time
	want int
}{
	{"19:00 on 31 Dec in Morelia", time.Date(2027, time.January, 1, 1, 0, 0, 0, time.UTC), 2026},
	{"00:00 on 1 Jan in Morelia", time.Date(2027, time.January, 1, 6, 0, 0, 0, time.UTC), 2027},
}

func morelia(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestARegistrationIsNumberedInTheSchoolsYear(t *testing.T) {
	for _, tt := range matriculaNights {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newService(t)
			svc = svc.WithClock(func() time.Time { return tt.at }).WithSchoolZone(morelia(t))

			if _, err := svc.CreateApplicant(context.Background(), validInput()); err != nil {
				t.Fatalf("CreateApplicant: %v", err)
			}
			if len(repo.numberYears) != 1 || repo.numberYears[0] != tt.want {
				t.Errorf("numbered in %v, want [%d]", repo.numberYears, tt.want)
			}
		})
	}
}

func TestAnImportIsNumberedInTheSchoolsYear(t *testing.T) {
	for _, tt := range matriculaNights {
		t.Run(tt.name, func(t *testing.T) {
			f := newImportFixtureAt(t, tt.at)
			f.svc = f.svc.WithSchoolZone(morelia(t))

			f.run(t, false, row(1, "HEGA050312MDFRNN08"))
			if len(f.repo.imported) != 1 || f.repo.imported[0].NumberYear != tt.want {
				t.Fatalf("imported %d pupils; want one numbered in %d", len(f.repo.imported), tt.want)
			}
		})
	}
}
