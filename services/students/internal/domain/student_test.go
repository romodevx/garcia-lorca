package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// primaria is a catalog level as config would hand it over.
func primaria(t *testing.T) domain.Level {
	t.Helper()
	l, err := domain.NewLevel("lvl-primaria", "Primaria")
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	return l
}

func newApplicant(t *testing.T) *domain.Student {
	t.Helper()
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID:           "s1",
		TenantID:     "t1",
		CURP:         curp,
		FirstName:    "Ana",
		LastName:     "Hernández",
		Level:        primaria(t),
		GradeGroup:   "4° A",
		GuardianName: "Marisol Ríos",
	})
	if err != nil {
		t.Fatalf("NewApplicant: %v", err)
	}
	return s
}

func TestNewApplicantStartsPending(t *testing.T) {
	s := newApplicant(t)
	if s.Status() != domain.AdmissionPending {
		t.Errorf("status = %v, want pending", s.Status())
	}
	if s.ID() != "s1" || s.CURP().String() != "HEGA050312MDFRNN08" || s.Level() != primaria(t) {
		t.Errorf("accessors mismatch: %+v", s)
	}
}

func TestNewApplicantValidates(t *testing.T) {
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	base := domain.NewApplicantParams{ID: "s1", TenantID: "t1", CURP: curp, FirstName: "Ana", LastName: "H", Level: primaria(t)}

	bad := map[string]func(p *domain.NewApplicantParams){
		"blank id":     func(p *domain.NewApplicantParams) { p.ID = "" },
		"blank tenant": func(p *domain.NewApplicantParams) { p.TenantID = "" },
		"blank first":  func(p *domain.NewApplicantParams) { p.FirstName = "  " },
		"blank last":   func(p *domain.NewApplicantParams) { p.LastName = "" },
		"no level":     func(p *domain.NewApplicantParams) { p.Level = domain.Level{} },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			p := base
			mutate(&p)
			if _, err := domain.NewApplicant(p); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestHappyPathAcceptThenEnroll(t *testing.T) {
	s := newApplicant(t)

	ev, err := s.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if s.Status() != domain.AdmissionAccepted {
		t.Errorf("status = %v, want accepted", s.Status())
	}
	if ev.Previous != domain.AdmissionPending || ev.New != domain.AdmissionAccepted {
		t.Errorf("accept event = %+v", ev)
	}

	enrolled, changed, err := s.Enroll("cycle-2026")
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if s.Status() != domain.AdmissionEnrolled {
		t.Errorf("status = %v, want enrolled", s.Status())
	}
	if enrolled.StudentID != "s1" || enrolled.CycleID != "cycle-2026" || enrolled.LevelID != "lvl-primaria" {
		t.Errorf("StudentEnrolled event = %+v", enrolled)
	}
	if changed.New != domain.AdmissionEnrolled {
		t.Errorf("changed event = %+v", changed)
	}
}

func TestRejectFromPending(t *testing.T) {
	s := newApplicant(t)
	ev, err := s.Reject("incomplete documents")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if s.Status() != domain.AdmissionRejected || ev.Reason != "incomplete documents" {
		t.Errorf("reject: status=%v event=%+v", s.Status(), ev)
	}
}

func TestInvalidTransitionsAreFailedPrecondition(t *testing.T) {
	s := newApplicant(t) // pending
	if _, _, err := s.Enroll("c"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("enroll from pending: want KindConflict, got %v", err)
	}
	if _, err := s.Withdraw("x"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("withdraw from pending: want KindConflict, got %v", err)
	}
	// After reject (terminal), nothing works.
	_, _ = s.Reject("x")
	if _, err := s.Accept(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("accept from rejected: want KindConflict, got %v", err)
	}
}

func TestWithdrawFromEnrolled(t *testing.T) {
	s := newApplicant(t)
	_, _ = s.Accept()
	_, _, _ = s.Enroll("c")
	ev, err := s.Withdraw("moved city")
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if s.Status() != domain.AdmissionWithdrawn || ev.Previous != domain.AdmissionEnrolled {
		t.Errorf("withdraw: status=%v event=%+v", s.Status(), ev)
	}
}
