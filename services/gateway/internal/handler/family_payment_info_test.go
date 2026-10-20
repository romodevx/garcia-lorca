package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type paymentInfoBody struct {
	CLABE       string `json:"clabe"`
	Bank        string `json:"bank"`
	Beneficiary string `json:"beneficiary"`
	Reference   string `json:"reference"`
}

func readPaymentInfo(t *testing.T, f *fakeCards, studentID string) (int, paymentInfoBody) {
	t.Helper()
	h := handler.NewFamilyPaymentInfoHandler(f, f, f)
	rec := httptest.NewRecorder()
	h.PaymentInfo(rec, asGuardian(httptest.NewRequest(http.MethodGet, "/api/v1/family/students/"+studentID+"/payment-info", nil), studentID))
	var body paymentInfoBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, body
}

// A family reads where to transfer: the school's CLABE, its bank, who receives
// the money and the child's own reference, so the school can tell whose it is.
func TestAFamilyReadsHowToPayForItsChild(t *testing.T) {
	f := &fakeCards{
		links:   []client.GuardianLink{{StudentID: "stu-1"}},
		student: client.StudentResult{ID: "stu-1", BankReference: "1234567"},
		profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", LegalName: "Educación García Lorca, S.C.", CLABE: "012180001234567899", CLABEBank: "BBVA México"},
	}
	code, got := readPaymentInfo(t, f, "stu-1")
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200", code)
	}
	want := paymentInfoBody{CLABE: "012180001234567899", Bank: "BBVA México", Beneficiary: "Educación García Lorca, S.C.", Reference: "1234567"}
	if got != want {
		t.Errorf("payment info = %+v, want %+v", got, want)
	}
}

// Without a legal name the beneficiary is the school's name; without a CLABE
// the app has nothing to show, and says nothing.
func TestPaymentInfoFallsBackToTheSchoolsName(t *testing.T) {
	f := &fakeCards{
		links:   []client.GuardianLink{{StudentID: "stu-1"}},
		student: client.StudentResult{ID: "stu-1"},
		profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", CLABE: "012180001234567899"},
	}
	_, got := readPaymentInfo(t, f, "stu-1")
	if got.Beneficiary != "COLEGIO GARCÍA LORCA" || got.Reference != "" {
		t.Errorf("payment info = %+v; want the school's name and no reference", got)
	}
}

// Another family's child is 404, like every by-child family read.
func TestPaymentInfoOfAnotherFamilysChildIsNotFound(t *testing.T) {
	f := &fakeCards{
		links:   []client.GuardianLink{{StudentID: "stu-1"}},
		student: client.StudentResult{ID: "stu-2", BankReference: "999"},
		profile: client.SchoolProfile{Name: "X", CLABE: "012180001234567899"},
	}
	if code, _ := readPaymentInfo(t, f, "stu-2"); code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", code)
	}
}
