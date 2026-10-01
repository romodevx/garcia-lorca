// Package expensereport assembles the Egresos module's reports (T10): a
// period's income, expenses and balance by method, income by concept,
// expenses by category and the expenses themselves — as JSON for the screen
// and as a report.Document the PDF and Excel renderers print. It computes no
// money: finance does; this package only arranges it.
package expensereport

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

const (
	dayLayout = "2006-01-02"
	// listCap is finance's bound on one list of expenses. A period with more
	// lists the newest ones and says so; its totals are always the whole
	// period's (FinanceSummary sums in SQL, uncapped).
	listCap = 500
)

// Source is the finance surface a report reads.
type Source interface {
	FinanceSummary(ctx context.Context, from, to string) (client.FinanceSummary, error)
	IncomeByConcept(ctx context.Context, from, to string) (client.Income, error)
	ListExpenses(ctx context.Context, q client.ExpenseQuery) ([]client.Expense, error)
}

// ProfileReader reads the school's details (config) for the printed heading.
type ProfileReader interface {
	GetSchoolProfile(ctx context.Context) (client.SchoolProfile, bool, error)
}

// Report is one period's money, as the screen shows it.
type Report struct {
	Period   string                `json:"period"`
	From     string                `json:"from"`
	To       string                `json:"to"`
	Label    string                `json:"label"`
	Summary  client.FinanceSummary `json:"summary"`
	Income   client.Income         `json:"incomeByConcept"`
	Expenses []client.Expense      `json:"expenses"`
	// ExpensesTruncated: the period has more expenses than the list carries.
	ExpensesTruncated bool `json:"expensesTruncated"`
}

// Builder reads a report's parts from finance and the school's details.
type Builder struct {
	src      Source
	profiles ProfileReader
}

// NewBuilder wires a Builder.
func NewBuilder(src Source, profiles ProfileReader) *Builder {
	return &Builder{src: src, profiles: profiles}
}

// Report reads the period of kind that holds anchor. The three reads share
// nothing, so they run at once.
func (b *Builder) Report(ctx context.Context, kind string, anchor time.Time) (Report, error) {
	p, err := report.NewPeriod(kind, anchor)
	if err != nil {
		return Report{}, err
	}
	from, to := p.From.Format(dayLayout), p.To.Format(dayLayout)
	rep := Report{Period: p.Kind, From: from, To: to, Label: p.Label()}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		s, err := b.src.FinanceSummary(gctx, from, to)
		rep.Summary = s
		return err
	})
	g.Go(func() error {
		in, err := b.src.IncomeByConcept(gctx, from, to)
		rep.Income = in
		return err
	})
	g.Go(func() error {
		es, err := b.src.ListExpenses(gctx, client.ExpenseQuery{From: from, To: to})
		rep.Expenses = es
		return err
	})
	if err := g.Wait(); err != nil {
		return Report{}, err
	}
	rep.ExpensesTruncated = len(rep.Expenses) >= listCap
	return rep, nil
}

// Document lays the report out for the PDF and the workbook.
func (b *Builder) Document(ctx context.Context, rep Report) (report.Document, error) {
	school, found, err := b.profiles.GetSchoolProfile(ctx)
	if err != nil {
		return report.Document{}, fmt.Errorf("reading the school's details: %w", err)
	}
	header := []string{"Reporte de finanzas"}
	if found {
		header = nonEmpty(school.Name, school.LegalName, prefixed("RFC ", school.RFC))
	}
	return report.Document{
		Title:    "Ingresos y egresos",
		Subtitle: rep.Label,
		Header:   header,
		Tables:   []report.Table{summaryTable(rep), methodTable(rep), incomeTable(rep), categoryTable(rep), expensesTable(rep)},
	}, nil
}

func summaryTable(rep Report) report.Table {
	s := rep.Summary
	return report.Table{
		Title:   "Resumen",
		Columns: []report.Column{{Header: "Concepto", Kind: report.Text, Weight: 3}, {Header: "Monto", Kind: report.Money, Weight: 1}},
		Rows: [][]report.Cell{
			{report.TextCell("Ingresos"), report.MoneyCell(s.Income.TotalCents)},
			{report.TextCell("Egresos"), report.MoneyCell(s.Expenses.TotalCents)},
			{report.TextCell("Balance (ingresos − egresos)"), report.MoneyCell(s.Balance.TotalCents)},
			// Not money: settled debt, brought nothing in (ADR 0025).
			{report.TextCell("Notas de crédito aplicadas (no es ingreso)"), report.MoneyCell(s.CreditNoteCents)},
		},
	}
}

func methodTable(rep Report) report.Table {
	s := rep.Summary
	row := func(label string, in, out, bal int64) []report.Cell {
		return []report.Cell{report.TextCell(label), report.MoneyCell(in), report.MoneyCell(out), report.MoneyCell(bal)}
	}
	return report.Table{
		Title: "Por forma de pago",
		Columns: []report.Column{
			{Header: "Forma de pago", Kind: report.Text, Weight: 2},
			{Header: "Ingresos", Kind: report.Money, Weight: 1}, {Header: "Egresos", Kind: report.Money, Weight: 1},
			{Header: "Balance", Kind: report.Money, Weight: 1},
		},
		Rows: [][]report.Cell{
			row("Efectivo", s.Income.CashCents, s.Expenses.CashCents, s.Balance.CashCents),
			row("Tarjeta", s.Income.CardCents, s.Expenses.CardCents, s.Balance.CardCents),
			row("Transferencia", s.Income.TransferCents, s.Expenses.TransferCents, s.Balance.TransferCents),
		},
		Total: row("Total", s.Income.TotalCents, s.Expenses.TotalCents, s.Balance.TotalCents),
	}
}

func incomeTable(rep Report) report.Table {
	t := report.Table{
		Title:   "Ingresos por concepto",
		Columns: []report.Column{{Header: "Concepto", Kind: report.Text, Weight: 3}, {Header: "Monto", Kind: report.Money, Weight: 1}},
		Total:   []report.Cell{report.TextCell("Total aplicado"), report.MoneyCell(rep.Income.TotalCents)},
	}
	for _, l := range rep.Income.Lines {
		t.Rows = append(t.Rows, []report.Cell{report.TextCell(l.Concept), report.MoneyCell(l.AmountCents)})
	}
	return t
}

func categoryTable(rep Report) report.Table {
	t := report.Table{
		Title: "Egresos por categoría",
		Columns: []report.Column{
			{Header: "Categoría", Kind: report.Text, Weight: 3}, {Header: "Egresos", Kind: report.Count, Weight: 1},
			{Header: "Monto", Kind: report.Money, Weight: 1},
		},
		Total: []report.Cell{report.TextCell("Total"), {}, report.MoneyCell(rep.Summary.Expenses.TotalCents)},
	}
	for _, c := range rep.Summary.ByCategory {
		t.Rows = append(t.Rows, []report.Cell{report.TextCell(c.CategoryName), report.CountCell(int64(c.Count)), report.MoneyCell(c.AmountCents)})
	}
	return t
}

var methodLabels = map[string]string{"cash": "Efectivo", "card": "Tarjeta", "transfer": "Transferencia"}

func expensesTable(rep Report) report.Table {
	title := "Egresos"
	if rep.ExpensesTruncated {
		title = fmt.Sprintf("Egresos (los %d más recientes; el total es del periodo completo)", listCap)
	}
	t := report.Table{
		Title: title,
		Columns: []report.Column{
			{Header: "Fecha", Kind: report.Text, Weight: 1.2}, {Header: "Categoría", Kind: report.Text, Weight: 1.6},
			{Header: "Descripción", Kind: report.Text, Weight: 3}, {Header: "Forma de pago", Kind: report.Text, Weight: 1.3},
			{Header: "Monto", Kind: report.Money, Weight: 1.2},
		},
		Total: []report.Cell{report.TextCell("Total"), {}, {}, {}, report.MoneyCell(rep.Summary.Expenses.TotalCents)},
	}
	for _, e := range rep.Expenses {
		t.Rows = append(t.Rows, []report.Cell{
			report.TextCell(e.SpentOn), report.TextCell(e.CategoryName), report.TextCell(e.Description),
			report.TextCell(methodLabels[e.Method]), report.MoneyCell(e.AmountCents),
		})
	}
	return t
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
