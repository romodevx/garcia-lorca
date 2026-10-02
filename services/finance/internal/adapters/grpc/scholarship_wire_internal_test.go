package grpcadapter

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T18 fields reach the wire (checklist #2): the counter's line and a charge.
func TestScholarshipFieldsReachTheWire(t *testing.T) {
	lines := toProtoExpectedLines([]usecase.ExpectedLine{{
		ChargeID: "ch-1", Description: "Colegiatura", DueDate: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		OwedCents: 150000, ScholarshipCents: 150000, TotalCents: 150000, Due: true,
	}})
	if lines[0].GetScholarshipCents() != 150000 || lines[0].GetOwedCents() != 150000 {
		t.Fatalf("expected line = %+v", lines[0])
	}

	c, err := domain.NewCharge("ch-1", "t1", "stu-1", "cy-1", "co-1", "Colegiatura", 300000, 0, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.WithScholarship("beca-1", 150000, 0)
	if err != nil {
		t.Fatal(err)
	}
	wire := toProtoCharge(ports.ChargeWithPaid{Charge: c})
	if wire.GetScholarshipCents() != 150000 || wire.GetScholarshipId() != "beca-1" || wire.GetRemainingCents() != 150000 {
		t.Fatalf("charge = %+v", wire)
	}
}
