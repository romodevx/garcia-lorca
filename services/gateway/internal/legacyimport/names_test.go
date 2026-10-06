package legacyimport

import (
	"errors"
	"slices"
	"testing"
)

func TestSplitNameUsesTheCURPToPlaceEverySurname(t *testing.T) {
	cases := []struct {
		name, full, curp          string
		first, paternal, maternal string
		warnings                  []string
	}{
		{"plain", "ALCAZAR MELENDEZ, IRLANDA", "AAMI210219MJCLLRA0",
			"IRLANDA", "ALCAZAR", "MELENDEZ", nil},
		{"accented surname", "ÁVILA TAPIA, LUNA CHARLOTTE", "AITL220629MJCVPNA4",
			"LUNA CHARLOTTE", "ÁVILA", "TAPIA", nil},
		{"particles bind to the next word", "DE LA CRUZ HERNANDEZ, JUAN", "CUHJ120101HJCRRNA1",
			"JUAN", "DE LA CRUZ", "HERNANDEZ", nil},
		{"compound paternal decided by the CURP", "GARCIA DE LA TORRE LOPEZ, ANA", "GALA120101MJCRPNA1",
			"ANA", "GARCIA DE LA TORRE", "LOPEZ", nil},
		{"compound maternal decided by the CURP", "GARCIA DE LA TORRE LOPEZ, ANA", "GATA120101MJCRRNA1",
			"ANA", "GARCIA", "DE LA TORRE LOPEZ", nil},
		{"both splits fit: the compound paternal wins, flagged", "ESPINOSA DE LOS MONTEROS MENDOZA, EDUARDO", "EIME120101HJCSNDA1",
			"EDUARDO", "ESPINOSA DE LOS MONTEROS", "MENDOZA", []string{WarnSurnamesAmbiguous}},
		{"one surname only", "MADONNA, ANA", "MAXA120101MJCDXNA1",
			"ANA", "MADONNA", "", nil},
		{"enye in the surname", "NUÑEZ ROJAS, LUIS", "NURL120101HJCXJSA1",
			"LUIS", "NUÑEZ", "ROJAS", nil},
		{"maria jose takes the second name", "PEREZ PEREZ, MARIA JOSEFINA", "PEPJ120101MJCRRSA1",
			"MARIA JOSEFINA", "PEREZ", "PEREZ", nil},
		{"the CURP fits no split", "SOTO RUIZ, ANA", "ZZZZ120101MJCRRNA1",
			"ANA", "SOTO", "RUIZ", []string{WarnSurnamesMismatch}},
		{"no comma, surnames first", "LOPEZ PEREZ JUAN", "LOPJ120101HJCPRNA1",
			"JUAN", "LOPEZ", "PEREZ", []string{WarnNameWithoutComma}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SplitName(tc.full, tc.curp)
			if err != nil {
				t.Fatalf("SplitName: %v", err)
			}
			if got.First != tc.first || got.Paternal != tc.paternal || got.Maternal != tc.maternal {
				t.Errorf("got %q / %q / %q, want %q / %q / %q",
					got.First, got.Paternal, got.Maternal, tc.first, tc.paternal, tc.maternal)
			}
			if !slices.Equal(got.Warnings, tc.warnings) {
				t.Errorf("warnings = %v, want %v", got.Warnings, tc.warnings)
			}
		})
	}
}

// Without a CURP to check against there is nothing to decide by: the first
// legal split is used, and the office is told.
func TestSplitNameWithoutAUsableCURPWarns(t *testing.T) {
	got, err := SplitName("SOTO RUIZ, ANA", "")
	if err != nil {
		t.Fatalf("SplitName: %v", err)
	}
	if got.Paternal != "SOTO" || got.Maternal != "RUIZ" || !slices.Contains(got.Warnings, WarnSurnamesMismatch) {
		t.Errorf("got %+v", got)
	}
}

func TestSplitNameRefusesWhatHasNoNameAndSurname(t *testing.T) {
	for _, full := range []string{"", ",", "SOTO,", ", ANA", "ANA"} {
		if _, err := SplitName(full, "SOXA120101MJCRRNA1"); !errors.Is(err, ErrNameUnreadable) {
			t.Errorf("%q: err = %v, want ErrNameUnreadable", full, err)
		}
	}
}
