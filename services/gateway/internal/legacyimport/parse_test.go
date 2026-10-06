package legacyimport

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// The previous system's report, as the five real files carry it. Preescolar's
// lacks "Programa"; every file repeats "Estado" — the pupil's, then the
// address's.
var legacyHeader = []any{
	"Matrícula Interna", "Nombre", "Escolaridad", "Programa", "Grado / Nivel", "Grupo", "Estado",
	"Cumpleaños", "CURP", "Email", "Telefono de Casa", "Número de Celular", "Dirección",
	"Dirección 2", "Estado", "Ciudad/Municipio", "Código Postal",
}

func legacyRow(ref, name, level, grade, group, status, birth, curp string) []any {
	return []any{ref, name, level, "", grade, group, status, birth, curp, "a@b.mx",
		"3314626871", "3311105850", "LERDO DE TEJADA 35. CENTRO", "", "Jalisco", "Tlajomulco de Zuñiga", "45640"}
}

func workbook(t *testing.T, rows ...[]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow(sheet, cell, &r); err != nil {
			t.Fatalf("SetSheetRow: %v", err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

func TestParseReadsTheLegacyReportByHeader(t *testing.T) {
	file := workbook(t, legacyHeader,
		legacyRow(" 25910", "ALCAZAR MELENDEZ, IRLANDA ", "Preescolar", "3", " b", "Activo", "19/02/2021", "aami210219mjcllra0"))

	rows, err := Parse(bytes.NewReader(file), DefaultLimits)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	want := Row{
		Number: 2, LegacyRef: "25910", FullName: "ALCAZAR MELENDEZ, IRLANDA", LevelName: "Preescolar",
		Grade: "3", GroupName: "B", Status: "Activo", BirthDate: "19/02/2021", CURP: "AAMI210219MJCLLRA0",
		Email: "a@b.mx", HomePhone: "3314626871", MobilePhone: "3311105850",
		Street: "LERDO DE TEJADA 35. CENTRO", State: "Jalisco", Municipality: "Tlajomulco de Zuñiga", PostalCode: "45640",
	}
	if r != want {
		t.Errorf("row =\n%+v\nwant\n%+v", r, want)
	}
}

// Preescolar's export has no "Programa" column; position must not matter.
func TestParseFindsColumnsWhateverTheirPosition(t *testing.T) {
	header := []any{"Nombre", "CURP", "Grupo", "Grado / Nivel", "Escolaridad", "Estado"}
	file := workbook(t, header, []any{"BEAS LARA, AMY", "BELA210908MJCSRMA2", "A", "3", "Preescolar", "Activo"})

	rows, err := Parse(bytes.NewReader(file), DefaultLimits)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rows[0].LevelName != "Preescolar" || rows[0].GroupName != "A" || rows[0].State != "" {
		t.Errorf("row = %+v", rows[0])
	}
}

// A date typed as a real Excel date arrives as a serial number, not as text.
func TestParseTurnsAnExcelDateIntoDayMonthYear(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &legacyHeader)
	row := legacyRow("1", "A B, C", "Primaria", "1", "A", "Activo", "", "AAMI210219MJCLLRA0")
	_ = f.SetSheetRow(sheet, "A2", &row)
	_ = f.SetCellValue(sheet, "H2", 44246.0) // 2021-02-19
	var buf bytes.Buffer
	_ = f.Write(&buf)

	rows, err := Parse(bytes.NewReader(buf.Bytes()), DefaultLimits)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rows[0].BirthDate != "19/02/2021" {
		t.Errorf("birth date = %q, want 19/02/2021", rows[0].BirthDate)
	}
}

func TestParseSkipsBlankRows(t *testing.T) {
	file := workbook(t, legacyHeader,
		legacyRow("1", "A B, C", "Primaria", "1", "A", "Activo", "", "X"),
		[]any{"", "", ""},
		legacyRow("2", "D E, F", "Primaria", "1", "A", "Activo", "", "Y"))

	rows, err := Parse(bytes.NewReader(file), DefaultLimits)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rows) != 2 || rows[1].Number != 4 {
		t.Errorf("got %d rows, last at %d; want 2, the last at sheet row 4", len(rows), rows[len(rows)-1].Number)
	}
}

func TestParseRefusesWhatItCannotRead(t *testing.T) {
	many := [][]any{legacyHeader}
	for i := 0; i < 4; i++ {
		many = append(many, legacyRow("1", "A B, C", "Primaria", "1", "A", "Activo", "", "X"))
	}
	cases := map[string]struct {
		file   []byte
		limits Limits
		want   error
	}{
		"not a workbook":   {[]byte("Matrícula,Nombre\n1,A"), DefaultLimits, ErrNotXLSX},
		"empty sheet":      {workbook(t, legacyHeader), DefaultLimits, ErrEmpty},
		"too many rows":    {workbook(t, many...), Limits{MaxRows: 3, MaxBytes: DefaultLimits.MaxBytes}, ErrTooManyRows},
		"too many bytes":   {workbook(t, many...), Limits{MaxRows: 100, MaxBytes: 100}, ErrTooLarge},
		"no header at all": {workbook(t), DefaultLimits, ErrEmpty},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(bytes.NewReader(tc.file), tc.limits); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// The message names the column, so the office knows what to fix.
func TestParseNamesTheMissingColumn(t *testing.T) {
	file := workbook(t, []any{"Nombre", "Escolaridad", "Grado / Nivel", "Grupo", "Estado"},
		[]any{"A B, C", "Primaria", "1", "A", "Activo"})

	_, err := Parse(bytes.NewReader(file), DefaultLimits)

	var missing MissingColumnError
	if !errors.As(err, &missing) || missing.Column != "CURP" {
		t.Fatalf("err = %v, want the missing CURP column named", err)
	}
	if !strings.Contains(err.Error(), "CURP") {
		t.Errorf("message %q does not name the column", err)
	}
}
