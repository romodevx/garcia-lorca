package receipt_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

func sampleStatement() client.Statement {
	return client.Statement{
		Movements: []client.StatementMovement{
			{Kind: "charge", Date: "2026-09-10", Description: "Colegiatura septiembre 2026", AmountCents: 320000, BalanceCents: 320000},
			{Kind: "discount", Date: "2026-09-10", Description: "Pronto pago · Colegiatura septiembre 2026", AmountCents: 10000, BalanceCents: 310000},
			{Kind: "payment", Date: "2026-09-12", Description: "Pago", AmountCents: 100000, BalanceCents: 210000, Method: "card", CardType: "debit", Folio: "P-2026-000042"},
			{Kind: "payment", Date: "2026-09-13", Description: "Pago", AmountCents: 50000, BalanceCents: 210000, Method: "cash", Voided: true, VoidReason: "dos veces"},
			{Kind: "surcharge", Date: "2026-10-01", Description: "Recargo por atraso · Colegiatura septiembre 2026", AmountCents: 20000, BalanceCents: 230000, Accrued: true},
			{Kind: "charge", Date: "2027-01-10", Description: "Inscripción 2027", AmountCents: 50000, BalanceCents: 230000, NotYetDue: true},
		},
		Totals:        client.StatementTotals{ChargedCents: 340000, DiscountCents: 10000, PaidCents: 100000, BalanceCents: 230000, NotYetDueCents: 50000},
		DueTodayCents: 230000, AsOf: "2026-10-01",
	}
}

// T7: the statement PDF says what the drawer says — school, pupil, every
// movement with its running balance, the totals — accents included.
func TestTheStatementPDFSaysWhatTheLedgerSays(t *testing.T) {
	doc, err := receipt.StatementPDF(receipt.StatementDoc{
		School: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", RFC: "EGL150101AB1"},
		Pupil:  "Zamora Sánchez Camila", PupilDetail: "1° A",
		Statement: sampleStatement(),
	})
	if err != nil {
		t.Fatalf("StatementPDF: %v", err)
	}
	if !bytes.HasPrefix(doc, []byte("%PDF-")) {
		t.Fatalf("not a PDF")
	}
	text := textOf(t, doc)
	for _, want := range []string{
		"COLEGIO GARCÍA LORCA", "Estado de cuenta", "Zamora Sánchez Camila", "1° A",
		"Colegiatura septiembre 2026", "Pronto pago", "P-2026-000042", "Tarjeta de débito",
		"Cancelado", "acumulado", "Aún no exigible",
		"$3,200.00", "$2,300.00", "Total cargado", "Total pagado", "Saldo pendiente", "A pagar hoy",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not say %q; it reads:\n%s", want, text)
		}
	}
}

// A year of movements does not fit on a letter page: the ledger goes on to the
// next, and nothing is cut off.
func TestALongStatementRunsOntoMorePages(t *testing.T) {
	st := client.Statement{AsOf: "2026-10-01"}
	for i := range 80 {
		st.Movements = append(st.Movements, client.StatementMovement{
			Kind: "charge", Date: "2026-09-10", Description: fmt.Sprintf("Concepto número %02d", i), AmountCents: 100, BalanceCents: int64(100 * (i + 1)),
		})
	}
	doc, err := receipt.StatementPDF(receipt.StatementDoc{Pupil: "Alumno", Statement: st})
	if err != nil {
		t.Fatalf("StatementPDF: %v", err)
	}
	text := textOf(t, doc)
	if !strings.Contains(text, "Concepto número 79") {
		t.Error("the last movement is not in the PDF")
	}
	r, err := pdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("the PDF does not open: %v", err)
	}
	if r.NumPage() < 2 {
		t.Errorf("eighty movements on %d page — they were drawn off it", r.NumPage())
	}
}

type fakeStatements struct {
	got    client.Statement
	family bool
	err    error
}

func (f *fakeStatements) GetStatement(_ context.Context, _ string) (client.Statement, error) {
	f.family = false
	return f.got, f.err
}

func (f *fakeStatements) GetFamilyStatement(_ context.Context, _ string) (client.Statement, error) {
	f.family = true
	return f.got, f.err
}

func TestTheStatementBuilderNamesThePupilAndTheSchool(t *testing.T) {
	statements := &fakeStatements{got: sampleStatement()}
	b := receipt.NewStatementBuilder(statements, fakeSources{
		student: client.StudentResult{FirstName: "Camila", LastName: "Zamora", MaternalLastName: "Sánchez", GradeGroup: "1° A"},
		profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA"}, found: true,
	}, fakeSources{profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA"}, found: true})

	doc, err := b.Document(context.Background(), "stu-1", true)
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if !statements.family {
		t.Error("the family's document was read from the school's ledger")
	}
	if doc.Pupil != "Zamora Sánchez Camila" || doc.PupilDetail != "1° A" || doc.School.Name != "COLEGIO GARCÍA LORCA" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Statement.Movements) != len(sampleStatement().Movements) {
		t.Errorf("movements = %d", len(doc.Statement.Movements))
	}

	if _, err := b.Document(context.Background(), "stu-1", false); err != nil || statements.family {
		t.Errorf("the office's document: err %v, family %v", err, statements.family)
	}
}

func TestAStatementThatCannotBeReadIsAnError(t *testing.T) {
	boom := errors.New("finance down")
	b := receipt.NewStatementBuilder(&fakeStatements{err: boom}, fakeSources{}, fakeSources{})
	if _, err := b.Document(context.Background(), "stu-1", false); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the finance error", err)
	}
}
