package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeMedical struct {
	gotID  string
	gotRec client.StudentMedical
	res    client.StudentMedical
}

func (f *fakeMedical) GetStudentMedical(_ context.Context, id string) (client.StudentMedical, error) {
	f.gotID = id
	return f.res, nil
}

func (f *fakeMedical) UpdateStudentMedical(_ context.Context, id string, rec client.StudentMedical) (client.StudentMedical, error) {
	f.gotID, f.gotRec = id, rec
	return rec, nil
}

func TestMedicalGetAnswersTheRecord(t *testing.T) {
	svc := &fakeMedical{res: client.StudentMedical{BloodType: "O+", Allergies: "Penicilina"}}
	h := handler.NewMedicalHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/stu-1/medical", nil)
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	var out map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.Code != http.StatusOK || svc.gotID != "stu-1" || out["bloodType"] != "O+" || out["allergies"] != "Penicilina" {
		t.Errorf("status %d body %v", rec.Code, out)
	}
}

func TestMedicalPutForwardsTheRecord(t *testing.T) {
	svc := &fakeMedical{}
	h := handler.NewMedicalHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1/medical",
		strings.NewReader(`{"bloodType":"AB-","allergies":"Nueces","conditions":"Asma"}`))
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	h.Put(rec, req)

	if rec.Code != http.StatusOK || svc.gotRec.BloodType != "AB-" || svc.gotRec.Conditions != "Asma" {
		t.Errorf("status %d forwarded %+v", rec.Code, svc.gotRec)
	}
}
