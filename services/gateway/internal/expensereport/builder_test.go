package expensereport_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/expensereport"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

// fakeSource is called from the builder's three goroutines at once: only the
// list call records what it was asked.
type fakeSource struct {
	query client.ExpenseQuery
}

func (f *fakeSource) FinanceSummary(context.Context, string, string) (client.FinanceSummary, error) {
	return client.FinanceSummary{
		Income:          client.MethodTotals{CashCents: 500000, CardCents: 100000, TotalCents: 600000},
		CreditNoteCents: 20000,
		Expenses:        client.MethodTotals{CashCents: 125050, TotalCents: 125050},
		Balance:         client.MethodTotals{CashCents: 374950, CardCents: 100000, TotalCents: 474950},
		ByCategory:      []client.CategoryTotal{{CategoryName: "Papelería", AmountCents: 125050, Count: 1}},
	}, nil
}

func (f *fakeSource) IncomeByConcept(context.Context, string, string) (client.Income, error) {
	return client.Income{Lines: []client.IncomeLine{{Concept: "Colegiatura", AmountCents: 580000}}, TotalCents: 580000, CreditNoteCents: 20000}, nil
}

func (f *fakeSource) ListExpenses(_ context.Context, q client.ExpenseQuery) ([]client.Expense, error) {
	f.query = q
	return []client.Expense{{SpentOn: "2026-10-01", CategoryName: "Papelería", Description: "Tóner", Method: "cash", AmountCents: 125050, Status: "active"}}, nil
}

type fakeProfile struct{}

func (fakeProfile) GetSchoolProfile(context.Context) (client.SchoolProfile, bool, error) {
	return client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", RFC: "EGL150101AB1"}, true, nil
}

func TestReportReadsThePeriod(t *testing.T) {
	src := &fakeSource{}
	b := expensereport.NewBuilder(src, fakeProfile{})
	rep, err := b.Report(context.Background(), "monthly", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if rep.From != "2026-10-01" || rep.To != "2026-10-31" || rep.Label != "Octubre de 2026" || rep.Period != "monthly" {
		t.Fatalf("period = %+v", rep)
	}
	if src.query.From != "2026-10-01" || src.query.To != "2026-10-31" || src.query.IncludeCancelled {
		t.Fatalf("expenses asked for %+v", src.query)
	}
	if rep.Summary.Balance.TotalCents != 474950 || len(rep.Income.Lines) != 1 || len(rep.Expenses) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if _, err := b.Report(context.Background(), "hourly", time.Now()); err == nil {
		t.Fatalf("an unknown period must be refused")
	}
}

func TestDocumentLaysOutEveryTable(t *testing.T) {
	b := expensereport.NewBuilder(&fakeSource{}, fakeProfile{})
	rep, _ := b.Report(context.Background(), "daily", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	doc, err := b.Document(context.Background(), rep)
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if doc.Header[0] != "COLEGIO GARCÍA LORCA" || doc.Subtitle != "1 de octubre de 2026" {
		t.Fatalf("heading = %+v", doc)
	}
	titles := []string{}
	for _, tb := range doc.Tables {
		titles = append(titles, tb.Title)
	}
	if strings.Join(titles, ",") != "Resumen,Por forma de pago,Ingresos por concepto,Egresos por categoría,Egresos" {
		t.Fatalf("tables = %v", titles)
	}
	out, err := report.PDF(doc)
	if err != nil || len(out) == 0 {
		t.Fatalf("PDF: %v", err)
	}
	// The summary says credit notes are not income, on their own line.
	summary := doc.Tables[0]
	found := false
	for _, row := range summary.Rows {
		if strings.HasPrefix(row[0].Text, "Notas de crédito") && row[1].Value == 20000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("summary rows = %+v", summary.Rows)
	}
}

type manyExpenses struct{ fakeSource }

func (manyExpenses) ListExpenses(context.Context, client.ExpenseQuery) ([]client.Expense, error) {
	return make([]client.Expense, 500), nil
}

// A year with more expenses than one list holds says the list is partial;
// its totals stay the whole period's.
func TestReportSaysWhenTheListIsPartial(t *testing.T) {
	b := expensereport.NewBuilder(&manyExpenses{}, fakeProfile{})
	rep, err := b.Report(context.Background(), "yearly", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !rep.ExpensesTruncated {
		t.Fatalf("truncated = %v, %v", rep.ExpensesTruncated, err)
	}
	doc, _ := b.Document(context.Background(), rep)
	if !strings.Contains(doc.Tables[4].Title, "500 más recientes") {
		t.Fatalf("title = %q", doc.Tables[4].Title)
	}
}
