package report

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Workbook layout.
const (
	maxSheetName = 31 // Excel's limit
	moneyFormat  = `"$"#,##0.00;-"$"#,##0.00`
	textColWidth = 18.0
	wideColWidth = 40.0
	numColWidth  = 16.0
	centsPerPeso = 100.0
	// firstTableRow leaves the title, the subtitle and a blank line above
	// each table.
	firstTableRow = 4
)

// XLSX renders the document as a workbook: one sheet per table, each headed by
// the document's title and subtitle. Money is a number with a currency format
// (pesos), counts are integers, and text is always written as text — a value
// that looks like a formula ("=HYPERLINK(…)") never becomes one.
func XLSX(doc Document) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	styles, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for i, t := range doc.Tables {
		name := sheetName(t.Title, i, used)
		if i == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return nil, fmt.Errorf("naming sheet %q: %w", name, err)
			}
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("adding sheet %q: %w", name, err)
		}
		if err := writeSheet(f, name, doc, t, styles); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	if err := f.Write(&out); err != nil {
		return nil, fmt.Errorf("writing the workbook: %w", err)
	}
	return out.Bytes(), nil
}

type sheetStyles struct{ title, head, money, moneyBold, bold int }

func newStyles(f *excelize.File) (sheetStyles, error) {
	format := moneyFormat
	specs := []*excelize.Style{
		{Font: &excelize.Font{Bold: true, Size: 14}},
		{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E8EFE9"}}},
		{CustomNumFmt: &format},
		{CustomNumFmt: &format, Font: &excelize.Font{Bold: true}},
		{Font: &excelize.Font{Bold: true}},
	}
	ids := make([]int, len(specs))
	for i, s := range specs {
		id, err := f.NewStyle(s)
		if err != nil {
			return sheetStyles{}, fmt.Errorf("defining a style: %w", err)
		}
		ids[i] = id
	}
	return sheetStyles{title: ids[0], head: ids[1], money: ids[2], moneyBold: ids[3], bold: ids[4]}, nil
}

// sheetName is the table's title made a valid, unique sheet name.
func sheetName(title string, i int, used map[string]bool) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		name = fmt.Sprintf("Hoja %d", i+1)
	}
	if r := []rune(name); len(r) > maxSheetName {
		name = string(r[:maxSheetName])
	}
	for base, n := name, 2; used[strings.ToLower(name)]; n++ {
		suffix := fmt.Sprintf(" %d", n)
		r := []rune(base)
		name = string(r[:min(len(r), maxSheetName-len(suffix))]) + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}

func writeSheet(f *excelize.File, sheet string, doc Document, t Table, st sheetStyles) error {
	put := func(col, row int, value any, style int) error {
		cell, err := excelize.CoordinatesToCellName(col, row)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			return fmt.Errorf("writing %s!%s: %w", sheet, cell, err)
		}
		if style != 0 {
			return f.SetCellStyle(sheet, cell, cell, style)
		}
		return nil
	}
	if err := put(1, 1, doc.Title, st.title); err != nil {
		return err
	}
	if err := put(1, 2, strings.TrimSpace(doc.Subtitle+" · "+t.Title), 0); err != nil {
		return err
	}
	row := firstTableRow
	for c, col := range t.Columns {
		if err := put(c+1, row, col.Header, st.head); err != nil {
			return err
		}
		if err := setWidth(f, sheet, c+1, col); err != nil {
			return err
		}
	}
	for _, cells := range t.Rows {
		row++
		if err := writeRow(put, t.Columns, cells, row, st.money, 0); err != nil {
			return err
		}
	}
	if len(t.Total) > 0 {
		row++
		return writeRow(put, t.Columns, t.Total, row, st.moneyBold, st.bold)
	}
	return nil
}

func writeRow(put func(int, int, any, int) error, cols []Column, cells []Cell, row, moneyStyle, textStyle int) error {
	for c, col := range cols {
		if c >= len(cells) || !cells[c].set {
			continue
		}
		var err error
		switch col.Kind {
		case Money:
			err = put(c+1, row, float64(cells[c].Value)/centsPerPeso, moneyStyle)
		case Count:
			err = put(c+1, row, cells[c].Value, textStyle)
		default:
			// A string value is stored as text: excelize never turns it into
			// a formula, whatever it starts with.
			err = put(c+1, row, cells[c].Text, textStyle)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func setWidth(f *excelize.File, sheet string, col int, c Column) error {
	name, err := excelize.ColumnNumberToName(col)
	if err != nil {
		return err
	}
	width := numColWidth
	if c.Kind == Text {
		width = textColWidth
		if c.Weight > 2 {
			width = wideColWidth
		}
	}
	return f.SetColWidth(sheet, name, name, width)
}
