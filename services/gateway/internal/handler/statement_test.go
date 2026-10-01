package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

type fakeStatementSource struct {
	ledger    client.Statement
	err       error
	studentID string
	family    bool
	calls     int
}

func (f *fakeStatementSource) Ledger(_ context.Context, studentID string, family bool) (client.Statement, error) {
	f.studentID, f.family = studentID, family
	f.calls++
	return f.ledger, f.err
}

func (f *fakeStatementSource) Document(_ context.Context, studentID string, family bool) (receipt.StatementDoc, error) {
	f.studentID, f.family = studentID, family
	f.calls++
	return receipt.StatementDoc{Pupil: "Zamora Sánchez Camila", Statement: f.ledger}, f.err
}

func oneLedger() client.Statement {
	return client.Statement{
		Movements: []client.StatementMovement{{Kind: "charge", Date: "2026-09-10", Description: "Colegiatura", AmountCents: 320000, BalanceCents: 320000}},
		Totals:    client.StatementTotals{ChargedCents: 320000, BalanceCents: 320000},
		AsOf:      "2026-10-01",
	}
}

func studentRequest(path, id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("studentId", id)
	return req
}

// T7: the office reads the ledger as JSON and as a PDF, never cached.
func TestTheOfficeReadsTheStatementAsJSONAndPDF(t *testing.T) {
	src := &fakeStatementSource{ledger: oneLedger()}
	h := handler.NewStatementHandler(src)

	rec := httptest.NewRecorder()
	h.JSON(rec, studentRequest("/api/v1/finance/accounts/stu-1/statement", "stu-1"))
	if rec.Code != http.StatusOK || src.studentID != "stu-1" || src.family {
		t.Fatalf("JSON: %d, read %q family=%v", rec.Code, src.studentID, src.family)
	}
	var body client.Statement
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Movements) != 1 || body.Totals.BalanceCents != 320000 {
		t.Errorf("body = %s (%v)", rec.Body.String(), err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	h.PDF(rec, studentRequest("/api/v1/finance/accounts/stu-1/statement/pdf", "stu-1"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="estado-de-cuenta-2026-10-01.pdf"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("PDF Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestAStatementFinanceRefusesKeepsItsStatus(t *testing.T) {
	h := handler.NewStatementHandler(&fakeStatementSource{err: connect.NewError(connect.CodeNotFound, nil)})
	for name, serve := range map[string]http.HandlerFunc{"json": h.JSON, "pdf": h.PDF} {
		rec := httptest.NewRecorder()
		serve(rec, studentRequest("/x", "stu-1"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
	}
}

// The family reads its own child's ledger — the family's view — and only its own.
func TestTheFamilyReadsItsChildsStatement(t *testing.T) {
	src := &fakeStatementSource{ledger: oneLedger()}
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, nil, nil, nil, nil, handler.WithFamilyStatements(src))

	rec := httptest.NewRecorder()
	h.Statement(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/statement", "usr-9", "studentId", "stu-1"))
	if rec.Code != http.StatusOK || src.studentID != "stu-1" || !src.family {
		t.Fatalf("status %d, read %q family=%v", rec.Code, src.studentID, src.family)
	}

	rec = httptest.NewRecorder()
	h.StatementPDF(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/statement/pdf", "usr-9", "studentId", "stu-1"))
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) || !src.family {
		t.Fatalf("PDF: status %d family=%v", rec.Code, src.family)
	}
}

func TestAnotherFamilysStatementIs404AndIsNeverRead(t *testing.T) {
	src := &fakeStatementSource{ledger: oneLedger()}
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, nil, nil, nil, nil, handler.WithFamilyStatements(src))

	for name, serve := range map[string]http.HandlerFunc{"json": h.Statement, "pdf": h.StatementPDF} {
		rec := httptest.NewRecorder()
		serve(rec, guardianRequest(http.MethodGet, "/x", "usr-9", "studentId", "stu-999"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
	}
	if src.calls != 0 {
		t.Errorf("finance was read %d times for a child that is not the caller's", src.calls)
	}
}

func TestAFamilyStatementWithoutAPrincipalIs401(t *testing.T) {
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, nil, nil, nil, nil, handler.WithFamilyStatements(&fakeStatementSource{}))
	rec := httptest.NewRecorder()
	h.Statement(rec, studentRequest("/x", "stu-1"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
