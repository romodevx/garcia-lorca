package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// PaymentStudentReader reads the pupil whose bank reference a family needs.
type PaymentStudentReader interface {
	GetStudent(ctx context.Context, id string) (client.StudentResult, error)
}

// FamilyPaymentInfoHandler tells a family how to pay for its child by
// transfer: the school's CLABE and the child's own bank reference, which is how
// the school tells whose deposit it is (ADR 0033).
type FamilyPaymentInfoHandler struct {
	links    GuardianLinkReader
	students PaymentStudentReader
	profiles SchoolProfileReader
}

// NewFamilyPaymentInfoHandler wires the handler.
func NewFamilyPaymentInfoHandler(links GuardianLinkReader, students PaymentStudentReader, profiles SchoolProfileReader) *FamilyPaymentInfoHandler {
	return &FamilyPaymentInfoHandler{links: links, students: students, profiles: profiles}
}

// paymentInfoResponse is what the app's "Cómo pagar" card shows. An empty
// CLABE means the school has not captured one, and the app shows nothing.
type paymentInfoResponse struct {
	CLABE       string `json:"clabe"`
	Bank        string `json:"bank"`
	Beneficiary string `json:"beneficiary"`
	Reference   string `json:"reference"`
}

// PaymentInfo handles GET /api/v1/family/students/{studentId}/payment-info.
// Another family's child is 404: nothing says whether that child exists.
func (h *FamilyPaymentInfoHandler) PaymentInfo(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	studentID := r.PathValue("studentId")
	owned, err := ownsChild(ctx, h.links, p.UserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !owned {
		writeNotFound(w)
		return
	}
	pupil, err := h.students.GetStudent(ctx, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	school, _, err := h.profiles.GetSchoolProfile(ctx)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	beneficiary := school.LegalName
	if beneficiary == "" {
		beneficiary = school.Name
	}
	// The reference identifies a child: no cache keeps it.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, paymentInfoResponse{
		CLABE: school.CLABE, Bank: school.CLABEBank, Beneficiary: beneficiary, Reference: pupil.BankReference,
	})
}
