package legacyimport

import (
	"slices"
	"testing"
)

func row(n int, full, level, grade, group, status, birth, curp string) Row {
	return Row{Number: n, LegacyRef: "25910", FullName: full, LevelName: level, Grade: grade,
		GroupName: group, Status: status, BirthDate: birth, CURP: curp}
}

func TestPlanBuildsACandidatePerRow(t *testing.T) {
	got := Plan([]Row{row(2, "ALCAZAR MELENDEZ, IRLANDA", "Preescolar", "3", "B", "Activo", "19/02/2021", "AAMI210219MJCLLRA0")})

	c := got.Candidates[0]
	if c.Skip != "" {
		t.Fatalf("skip = %q", c.Skip)
	}
	if c.Name.Paternal != "ALCAZAR" || c.Sex != "F" || c.BirthDate != "2021-02-19" || c.Ordinal != 3 || c.BankRef != "25910" {
		t.Errorf("candidate = %+v", c)
	}
	if len(c.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", c.Warnings)
	}
}

func TestPlanSkipsWhatIsNotToBeImported(t *testing.T) {
	cases := map[string]struct {
		r    Row
		skip string
	}{
		"baja":               {row(2, "A B, C", "Primaria", "1", "A", "Baja", "", "AAMI210219MJCLLRA0"), SkipWithdrawn},
		"unknown status":     {row(2, "A B, C", "Primaria", "1", "A", "Egresado", "", "AAMI210219MJCLLRA0"), SkipStatusUnknown},
		"grade not a number": {row(2, "A B, C", "Primaria", "primero", "A", "Activo", "", "AAMI210219MJCLLRA0"), SkipGradeInvalid},
		"no group":           {row(2, "A B, C", "Primaria", "1", "", "Activo", "", "AAMI210219MJCLLRA0"), SkipGroupInvalid},
		"no level":           {row(2, "A B, C", "", "1", "A", "Activo", "", "AAMI210219MJCLLRA0"), SkipLevelMissing},
		"no surname":         {row(2, ", C", "Primaria", "1", "A", "Activo", "", "AAMI210219MJCLLRA0"), SkipNameUnreadable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Plan([]Row{tc.r}).Candidates[0].Skip; got != tc.skip {
				t.Errorf("skip = %q, want %q", got, tc.skip)
			}
		})
	}
}

// The CURP carries the birth date (positions 5–10, century in position 17):
// when the column is unreadable or disagrees, the CURP's is the one kept.
func TestPlanFallsBackToTheCURPForTheBirthDate(t *testing.T) {
	cases := map[string]struct {
		birth, curp, want string
		warn              string
	}{
		"unreadable":          {"31/02/2021", "AAMI210219MJCLLRA0", "2021-02-19", WarnBirthDateFromCURP},
		"empty":               {"", "AAMI210219MJCLLRA0", "2021-02-19", WarnBirthDateFromCURP},
		"disagrees":           {"20/02/2021", "AAMI210219MJCLLRA0", "2021-02-19", WarnBirthDateFromCURP},
		"1900s by digit":      {"", "AAGJ990101MJCRRH09", "1999-01-01", WarnBirthDateFromCURP},
		"no curp date either": {"", "", "", WarnBirthDateInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := Plan([]Row{row(2, "A B, C", "Primaria", "1", "A", "Activo", tc.birth, tc.curp)}).Candidates[0]
			if c.BirthDate != tc.want || !slices.Contains(c.Warnings, tc.warn) {
				t.Errorf("birth = %q warnings %v, want %q with %q", c.BirthDate, c.Warnings, tc.want, tc.warn)
			}
		})
	}
}

// Placements are counted per (level, grade, group) among the rows that will
// be imported, so a new group can be sized for the pupils it receives.
func TestPlanCountsThePupilsOfEachPlacement(t *testing.T) {
	got := Plan([]Row{
		row(2, "A B, C", "Primaria", "1", "A", "Activo", "", "AAMI210219MJCLLRA0"),
		row(3, "A B, C", "primaria ", "1", "a", "Activo", "", "AAMI210219MJCLLRA0"),
		row(4, "A B, C", "Primaria", "1", "B", "Activo", "", "AAMI210219MJCLLRA0"),
		row(5, "A B, C", "Primaria", "1", "A", "Baja", "", "AAMI210219MJCLLRA0"),
	})

	if len(got.Levels) != 1 || got.Levels[0] != "Primaria" {
		t.Errorf("levels = %v, want [Primaria]", got.Levels)
	}
	if n := got.Placements[PlacementKey{Level: "Primaria", Ordinal: 1, Group: "A"}]; n != 2 {
		t.Errorf("1°A = %d pupils, want 2 (the Baja does not count)", n)
	}
	if n := got.Placements[PlacementKey{Level: "Primaria", Ordinal: 1, Group: "B"}]; n != 1 {
		t.Errorf("1°B = %d pupils, want 1", n)
	}
}

func TestSexFromCURP(t *testing.T) {
	for curp, want := range map[string]string{"AAMI210219MJCLLRA0": "F", "BAME220202HJCRRMA7": "M", "": "", "SHORT": ""} {
		if got := sexFromCURP(curp); got != want {
			t.Errorf("%q: sex = %q, want %q", curp, got, want)
		}
	}
}
