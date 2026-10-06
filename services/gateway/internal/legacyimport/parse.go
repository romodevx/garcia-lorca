// Package legacyimport reads the student report the school's previous system
// exports (one .xlsx per level) and turns it into rows the import can plan
// with (ADR 0033). It is pure: no network, no clock, no domain rules — those
// live in students and config, which judge every row. This package only finds
// the columns, cleans the obvious noise and keeps the spreadsheet row number so
// every answer can point the office back at the line to fix.
package legacyimport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Limits bound what one upload may cost the gateway.
type Limits struct {
	MaxRows  int   // data rows, header excluded
	MaxBytes int64 // the compressed file as uploaded
}

// DefaultLimits fits the largest real file (Primaria, 297 rows, ~40 KB) many
// times over while keeping one commit inside the gateway's write timeout.
var DefaultLimits = Limits{MaxRows: 1000, MaxBytes: 2 << 20}

// Decompression caps: a 2 MB upload that inflates past these is a zip bomb,
// not a school's student list.
const (
	unzipSizeLimit    = 20 << 20
	unzipXMLSizeLimit = 10 << 20
	maxColumns        = 40
)

// Errors the upload screen words for the office.
var (
	ErrNotXLSX     = errors.New("legacyimport: the file is not an .xlsx workbook")
	ErrEmpty       = errors.New("legacyimport: the sheet has no students")
	ErrTooManyRows = errors.New("legacyimport: the sheet has too many rows")
	ErrTooLarge    = errors.New("legacyimport: the file is too large")
)

// MissingColumnError names a required column the sheet does not have.
type MissingColumnError struct{ Column string }

func (e MissingColumnError) Error() string {
	return fmt.Sprintf("legacyimport: the sheet has no %q column", e.Column)
}

// Row is one pupil as the previous system wrote it, trimmed. Number is the
// spreadsheet row (the header is row 1).
type Row struct {
	Number       int
	LegacyRef    string // "Matrícula Interna": the BBVA reference (O1)
	FullName     string // "APELLIDOS, NOMBRES"
	LevelName    string // "Escolaridad"
	Grade        string // "Grado / Nivel", as written
	GroupName    string // upper-cased
	Status       string // the pupil's "Estado": Activo | Baja
	BirthDate    string // dd/mm/yyyy (an Excel date serial is converted)
	CURP         string // upper-cased
	Email        string
	HomePhone    string
	MobilePhone  string
	Street       string
	State        string // the address's "Estado"
	Municipality string
	PostalCode   string
}

type field int

const (
	fLegacyRef field = iota
	fFullName
	fLevel
	fGrade
	fGroup
	fStatus
	fBirth
	fCURP
	fEmail
	fHomePhone
	fMobile
	fStreet
	fState
	fMunicipality
	fPostalCode
)

// headers maps a folded header to its field. "estado" is resolved by
// position: the first is the pupil's status, the second the address's state.
var headers = map[string]field{
	"matricula interna": fLegacyRef, "matricula": fLegacyRef,
	"nombre": fFullName, "escolaridad": fLevel,
	"grado / nivel": fGrade, "grado": fGrade, "grupo": fGroup,
	"cumpleanos": fBirth, "fecha de nacimiento": fBirth, "curp": fCURP, "email": fEmail, "correo": fEmail,
	"telefono de casa": fHomePhone, "numero de celular": fMobile, "celular": fMobile,
	"direccion": fStreet, "ciudad/municipio": fMunicipality, "municipio": fMunicipality,
	"codigo postal": fPostalCode,
}

// required are the columns without which a row means nothing, worded as the
// previous system names them.
var required = []struct {
	f    field
	name string
}{
	{fFullName, "Nombre"}, {fLevel, "Escolaridad"}, {fGrade, "Grado / Nivel"},
	{fGroup, "Grupo"}, {fStatus, "Estado"}, {fCURP, "CURP"},
}

// Parse reads the first sheet of an .xlsx from the previous system.
func Parse(r io.Reader, lim Limits) ([]Row, error) {
	data, err := io.ReadAll(io.LimitReader(r, lim.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("legacyimport: read upload: %w", err)
	}
	if int64(len(data)) > lim.MaxBytes {
		return nil, ErrTooLarge
	}
	book, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit: unzipSizeLimit, UnzipXMLSizeLimit: unzipXMLSizeLimit,
	})
	if err != nil {
		return nil, ErrNotXLSX
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrEmpty
	}
	iter, err := book.Rows(sheets[0])
	if err != nil {
		return nil, ErrNotXLSX
	}
	defer iter.Close()
	return readRows(iter, lim)
}

func readRows(iter *excelize.Rows, lim Limits) ([]Row, error) {
	var (
		columns map[field]int
		out     []Row
		number  int
	)
	for iter.Next() {
		number++
		cells, err := iter.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, ErrNotXLSX
		}
		if len(cells) > maxColumns {
			cells = cells[:maxColumns]
		}
		if blank(cells) {
			continue
		}
		if columns == nil {
			if columns, err = mapColumns(cells); err != nil {
				return nil, err
			}
			continue
		}
		if len(out) == lim.MaxRows {
			return nil, ErrTooManyRows
		}
		out = append(out, toRow(number, cells, columns))
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

func mapColumns(cells []string) (map[field]int, error) {
	columns := map[field]int{}
	seenStatus := false
	for i, c := range cells {
		key := foldHeader(c)
		if key == "estado" {
			if seenStatus {
				columns[fState] = i
			} else {
				columns[fStatus], seenStatus = i, true
			}
			continue
		}
		if f, ok := headers[key]; ok {
			if _, dup := columns[f]; !dup {
				columns[f] = i
			}
		}
	}
	for _, req := range required {
		if _, ok := columns[req.f]; !ok {
			return nil, MissingColumnError{Column: req.name}
		}
	}
	return columns, nil
}

func toRow(number int, cells []string, columns map[field]int) Row {
	get := func(f field) string {
		i, ok := columns[f]
		if !ok || i >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[i])
	}
	return Row{
		Number: number, LegacyRef: get(fLegacyRef), FullName: collapse(get(fFullName)),
		LevelName: collapse(get(fLevel)), Grade: get(fGrade), GroupName: strings.ToUpper(get(fGroup)),
		Status: get(fStatus), BirthDate: excelDate(get(fBirth)), CURP: strings.ToUpper(get(fCURP)),
		Email: get(fEmail), HomePhone: get(fHomePhone), MobilePhone: get(fMobile),
		Street: collapse(get(fStreet)), State: get(fState), Municipality: collapse(get(fMunicipality)),
		PostalCode: get(fPostalCode),
	}
}

var serialDate = regexp.MustCompile(`^\d{4,6}(\.\d+)?$`)

// excelDate turns an Excel date serial (what a real date cell holds once the
// display format is ignored) into the dd/mm/yyyy the previous system types as
// text. Anything else is returned as it came, for the planner to judge.
func excelDate(raw string) string {
	if !serialDate.MatchString(raw) {
		return raw
	}
	serial, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	day, err := excelize.ExcelDateToTime(serial, false)
	if err != nil {
		return raw
	}
	return day.Format("02/01/2006")
}

func blank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// foldHeader lower-cases, drops accents (ñ included: "Cumpleaños" is matched
// as "cumpleanos") and collapses spacing, so "Número de Celular" and
// "numero  de celular" are the same column.
func foldHeader(s string) string {
	return collapse(strings.ReplaceAll(strings.ToLower(Fold(s)), "ñ", "n"))
}
