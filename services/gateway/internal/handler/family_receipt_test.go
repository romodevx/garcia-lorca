package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// receiptSources is finance (deposits), students, config and uniforms as the
// REAL receipt builder reads them, so the family route is tested against the
// builder's own pupil check, not against a fake of it.
type receiptSources struct {
	deposits map[string]client.DepositReceipt
	reads    int
}

func (f *receiptSources) DepositReceipt(_ context.Context, id string) (client.DepositReceipt, error) {
	f.reads++
	got, ok := f.deposits[id]
	if !ok {
		return client.DepositReceipt{}, connect.NewError(connect.CodeNotFound, errors.New("no such deposit"))
	}
	return got, nil
}

func (f *receiptSources) GetOrder(context.Context, string, []string) (client.UniformOrder, error) {
	return client.UniformOrder{}, connect.NewError(connect.CodeNotFound, nil)
}

func (f *receiptSources) GetStudent(_ context.Context, id string) (client.StudentResult, error) {
	return client.StudentResult{ID: id, FirstName: "Alumna", LastName: "Dos"}, nil
}

func (f *receiptSources) GetAccount(context.Context, string) (client.AccountStatement, error) {
	return client.AccountStatement{}, nil
}

func (f *receiptSources) GetSchoolProfile(context.Context) (client.SchoolProfile, bool, error) {
	return client.SchoolProfile{Name: "COLEGIO"}, true, nil
}

// Synthetic fixtures: stu-1 is the caller's child, stu-2 is not.
func familyReceiptFixture() (*handler.FamilyHandler, *receiptSources) {
	src := &receiptSources{deposits: map[string]client.DepositReceipt{
		"dep-own": {Deposit: client.Deposit{ID: "dep-own", StudentID: "stu-1", AmountCents: 300000, Method: "cash",
			PaidAt: "2026-10-17T00:00:00Z", Folio: "P-2026-000002"}},
		"dep-other": {Deposit: client.Deposit{ID: "dep-other", StudentID: "stu-2", AmountCents: 100000, Method: "cash",
			PaidAt: "2026-10-17T00:00:00Z", Folio: "P-2026-000009"}},
		"dep-void": {Deposit: client.Deposit{ID: "dep-void", StudentID: "stu-1", AmountCents: 60000, Method: "cash",
			PaidAt: "2026-10-16T00:00:00Z", Folio: "P-2026-000003", VoidedAt: "2026-10-16T18:00:00Z", VoidReason: "importe capturado mal"}},
		"dep-superseded": {Deposit: client.Deposit{ID: "dep-superseded", StudentID: "stu-1", AmountCents: 60000, Method: "cash",
			PaidAt: "2026-10-16T00:00:00Z", Folio: "P-2026-000004", SupersededBy: "dep-own", SupersededByFolio: "P-2026-000002"}},
	}}
	builder := receipt.NewBuilder(src, src, src, src, src)
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, nil, nil, nil, nil, handler.WithFamilyReceipts(builder))
	return h, src
}

func familyReceiptRequest(studentID, depositID string) *http.Request {
	req := guardianRequest(http.MethodGet, "/api/v1/family/students/"+studentID+"/receipts/"+depositID, "usr-9", "studentId", studentID)
	req.SetPathValue("depositId", depositID)
	return req
}

// ISSUE-T14-107: the family opens the receipt of its own child's payment, as
// JSON and as the PDF it shares, never cached.
func TestTheFamilyOpensItsOwnChildsReceipt(t *testing.T) {
	h, _ := familyReceiptFixture()

	rec := httptest.NewRecorder()
	h.ReceiptJSON(rec, familyReceiptRequest("stu-1", "dep-own"))
	if rec.Code != http.StatusOK {
		t.Fatalf("JSON: status %d, body %s", rec.Code, rec.Body.String())
	}
	var body receipt.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Folio != "P-2026-000002" || body.TotalCents != 300000 {
		t.Errorf("JSON body = %s (%v)", rec.Body.String(), err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	h.ReceiptPDF(rec, familyReceiptRequest("stu-1", "dep-own"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF: status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="comprobante-P-2026-000002.pdf"` {
		t.Errorf("Content-Disposition = %q", got)
	}
}

// Another family's child is 404 and finance is never asked; a deposit of
// another pupil under the caller's own child's id is 404 too, as is one that
// does not exist, and so is a voided or superseded one (the family's statement
// does not show them) — all read the same, so nothing leaks.
func TestAFamilyReceiptNotOfTheChildIs404(t *testing.T) {
	cases := []struct {
		name, studentID, depositID string
		financeRead                bool
	}{
		{"another family's child", "stu-2", "dep-other", false},
		{"another pupil's deposit under the own child", "stu-1", "dep-other", true},
		{"a deposit that does not exist", "stu-1", "dep-missing", true},
		{"a voided deposit", "stu-1", "dep-void", true},
		{"a superseded deposit", "stu-1", "dep-superseded", true},
	}
	for _, tc := range cases {
		for name, serve := range map[string]func(*handler.FamilyHandler) http.HandlerFunc{
			"json": func(h *handler.FamilyHandler) http.HandlerFunc { return h.ReceiptJSON },
			"pdf":  func(h *handler.FamilyHandler) http.HandlerFunc { return h.ReceiptPDF },
		} {
			h, src := familyReceiptFixture()
			rec := httptest.NewRecorder()
			serve(h)(rec, familyReceiptRequest(tc.studentID, tc.depositID))
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s (%s): status = %d, want 404", tc.name, name, rec.Code)
			}
			if got := src.reads > 0; got != tc.financeRead {
				t.Errorf("%s (%s): finance read = %v, want %v", tc.name, name, got, tc.financeRead)
			}
		}
	}
}

// No principal is 401; a handler composed without receipts is 404.
func TestAFamilyReceiptWithoutPrincipalOrWiringIsRefused(t *testing.T) {
	h, _ := familyReceiptFixture()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.SetPathValue("studentId", "stu-1")
	req.SetPathValue("depositId", "dep-own")
	rec := httptest.NewRecorder()
	h.ReceiptJSON(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no principal: status = %d, want 401", rec.Code)
	}

	bare := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, nil, nil, nil, nil)
	rec = httptest.NewRecorder()
	bare.ReceiptPDF(rec, familyReceiptRequest("stu-1", "dep-own"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("no receipts wired: status = %d, want 404", rec.Code)
	}
}
