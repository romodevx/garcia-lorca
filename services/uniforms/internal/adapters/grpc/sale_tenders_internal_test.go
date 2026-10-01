package grpcadapter

import (
	"testing"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// T8: every field of a sale's tenders crosses from the wire to the use case.
func TestSaleTendersCarryEveryField(t *testing.T) {
	got := saleTenders([]*uniformsv1.SaleTender{
		{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"},
		{Method: "card", AmountCents: 44000, CardType: "credit"},
	})
	want := []usecase.Tender{
		{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"},
		{Method: "card", AmountCents: 44000, CardType: "credit"},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tenders = %+v, want %+v", got, want)
	}
}
