package studentname_test

import (
	"testing"

	"github.com/garcia-lorca/pkg/studentname"
)

func TestFormatPutsSurnamesFirst(t *testing.T) {
	tests := []struct {
		name string
		in   studentname.Parts
		want string
	}{
		{"both surnames", studentname.Parts{First: "Ana Sofía", Paternal: "Pérez", Maternal: "Núñez"}, "Pérez Núñez Ana Sofía"},
		{"no maternal surname", studentname.Parts{First: "Luis", Paternal: "Torres"}, "Torres Luis"},
		{"legacy row with both surnames in paternal", studentname.Parts{First: "Luis", Paternal: "TORRES GARCÍA"}, "TORRES GARCÍA Luis"},
		{"blanks are trimmed", studentname.Parts{First: "  Luis ", Paternal: " Torres ", Maternal: "  "}, "Torres Luis"},
		{"only first name", studentname.Parts{First: "Luis"}, "Luis"},
		{"empty", studentname.Parts{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := studentname.Format(tt.in); got != tt.want {
				t.Errorf("Format(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSortOrdersBySurnamesThenFirstNameInSpanish(t *testing.T) {
	pupils := []studentname.Parts{
		{First: "Zoe", Paternal: "Ñuñez"},
		{First: "Beto", Paternal: "álvarez", Maternal: "Ruiz"},
		{First: "Ana", Paternal: "Alvarez", Maternal: "Ruiz"},
		{First: "Carla", Paternal: "Nuñez"},
		{First: "Dora", Paternal: "Álvarez", Maternal: "López"},
		{First: "Eva", Paternal: "Oropeza"},
	}
	studentname.Sort(pupils, func(p studentname.Parts) studentname.Parts { return p })

	want := []string{
		"Álvarez López Dora",
		"Alvarez Ruiz Ana",
		"álvarez Ruiz Beto",
		"Nuñez Carla",
		"Ñuñez Zoe",
		"Oropeza Eva",
	}
	for i, p := range pupils {
		if got := studentname.Format(p); got != want[i] {
			t.Errorf("position %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestSortIsStableOnIdenticalNames(t *testing.T) {
	type row struct {
		id   string
		name studentname.Parts
	}
	rows := []row{
		{"b", studentname.Parts{First: "Ana", Paternal: "Ruiz"}},
		{"a", studentname.Parts{First: "Ana", Paternal: "Ruiz"}},
	}
	studentname.Sort(rows, func(r row) studentname.Parts { return r.name })
	if rows[0].id != "b" || rows[1].id != "a" {
		t.Errorf("identical names must keep their order, got %s,%s", rows[0].id, rows[1].id)
	}
}
