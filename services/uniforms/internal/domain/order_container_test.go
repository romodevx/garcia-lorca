package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

var placedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

func TestNewOrderSnapshotsTheStudent(t *testing.T) {
	o, err := domain.NewOrder("ord-1", "t-1", "U-2026-000123", domain.OrderStudent{
		ID: "stu-1", FullName: "Zoe Villalobos", Level: domain.Level("c1000000-0000-0000-0000-000000000002"), LevelName: "Primaria",
		GradeGroup: "3 B", CycleID: "cyc-1",
	}, domain.SourceFamily, "usr-1", placedAt)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if o.StudentName() != "Zoe Villalobos" || o.StudentLevel() != domain.Level("c1000000-0000-0000-0000-000000000002") || o.StudentLevelName() != "Primaria" || o.StudentGradeGroup() != "3 B" {
		t.Errorf("the student's details were not snapshotted: %+v", o)
	}
}

// TestCounterSaleWithoutAStudentIsAllowed is the uncle at the window. It still
// moves stock and still takes money; it simply cannot land on anybody's account
// statement, so it needs a note saying who bought it.
func TestCounterSaleWithoutAStudentIsAllowed(t *testing.T) {
	o, err := domain.NewOrder("ord-1", "t-1", "U-2026-000123", domain.OrderStudent{},
		domain.SourceCounter, "usr-1", placedAt)
	if err != nil {
		t.Fatalf("an anonymous counter sale must be possible: %v", err)
	}
	if o.StudentID() != "" || o.CycleID() != "" {
		t.Error("an order with no student may not claim a cycle either")
	}
	if !o.IsAnonymous() {
		t.Error("IsAnonymous should report the sale with no student")
	}
}

// TestAnonymousOrdersAreOnlyForTheCounter: a family app order always names the
// child. Letting one through without would create an obligation nobody owns.
func TestAnonymousOrdersAreOnlyForTheCounter(t *testing.T) {
	_, err := domain.NewOrder("ord-1", "t-1", "U-2026-000123", domain.OrderStudent{},
		domain.SourceFamily, "usr-1", placedAt)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid for an anonymous family order, got %v", err)
	}
}

// TestOrderWithAStudentNeedsACycle mirrors the table's CHECK: a charge has to
// belong to a school year, so a named student always brings one.
func TestOrderWithAStudentNeedsACycle(t *testing.T) {
	_, err := domain.NewOrder("ord-1", "t-1", "U-2026-000123", domain.OrderStudent{
		ID: "stu-1", FullName: "Zoe", Level: domain.Level("c1000000-0000-0000-0000-000000000002"),
	}, domain.SourceFamily, "usr-1", placedAt)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid without a cycle, got %v", err)
	}
}

func TestNewOrderValidation(t *testing.T) {
	student := domain.OrderStudent{ID: "stu-1", FullName: "Zoe", Level: domain.Level("c1000000-0000-0000-0000-000000000002"), CycleID: "cyc-1"}
	cases := []struct {
		name              string
		id, tenant, folio string
		source            domain.OrderSource
	}{
		{"blank id", "", "t-1", "U-2026-000123", domain.SourceFamily},
		{"blank tenant", "ord-1", "", "U-2026-000123", domain.SourceFamily},
		{"malformed folio", "ord-1", "t-1", "nonsense", domain.SourceFamily},
		{"unknown source", "ord-1", "t-1", "U-2026-000123", domain.OrderSource("carrier pigeon")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewOrder(tc.id, tc.tenant, tc.folio, student, tc.source, "usr-1", placedAt)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestOrderClose(t *testing.T) {
	o, err := domain.NewOrder("ord-1", "t-1", "U-2026-000123", domain.OrderStudent{},
		domain.SourceCounter, "usr-1", placedAt)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	closed := o.Close(placedAt.Add(time.Hour))
	if closed.ClosedAt().IsZero() {
		t.Error("Close did not stamp the order")
	}
	if !o.ClosedAt().IsZero() {
		t.Error("Close must not mutate the receiver")
	}
}
