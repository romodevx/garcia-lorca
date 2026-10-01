package report

import (
	"fmt"
	"strings"
)

// ColumnKind says what a column holds, which decides its alignment in the PDF
// and its cell type in the workbook.
type ColumnKind int

const (
	// Text is free text, left-aligned.
	Text ColumnKind = iota
	// Money is integer cents, right-aligned; a number in the workbook.
	Money
	// Count is a whole number, right-aligned.
	Count
)

// Column is one column of a table. Weight is its share of the page's width
// (0 counts as 1).
type Column struct {
	Header string
	Kind   ColumnKind
	Weight float64
}

// Cell is one value: Text for a text column, Value for money (cents) and
// counts. The zero Cell is an empty cell.
type Cell struct {
	Text  string
	Value int64
	set   bool
}

// TextCell is a text value.
func TextCell(s string) Cell { return Cell{Text: s, set: true} }

// MoneyCell is an amount of cents.
func MoneyCell(cents int64) Cell { return Cell{Value: cents, set: true} }

// CountCell is a whole number.
func CountCell(n int64) Cell { return Cell{Value: n, set: true} }

// Table is a titled table; Total, when present, is printed as its last row.
type Table struct {
	Title   string
	Columns []Column
	Rows    [][]Cell
	Total   []Cell
}

// Document is a report: who issues it (Header lines: the school), what it is
// (Title) and for when (Subtitle), and its tables in order.
type Document struct {
	Title    string
	Subtitle string
	Header   []string
	Tables   []Table
}

// display renders a cell as the PDF prints it.
func display(kind ColumnKind, c Cell) string {
	if !c.set {
		return ""
	}
	switch kind {
	case Money:
		return FormatMoney(c.Value)
	case Count:
		return fmt.Sprintf("%d", c.Value)
	default:
		return c.Text
	}
}

// FormatMoney is "$1,250.50", "-$250.00": the receipts' format.
func FormatMoney(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	pesos := fmt.Sprintf("%d", cents/100)
	var grouped strings.Builder
	for i, digit := range pesos {
		if i > 0 && (len(pesos)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, grouped.String(), cents%100)
}
