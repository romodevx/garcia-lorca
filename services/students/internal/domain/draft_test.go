package domain_test

import (
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func newDraft(t *testing.T) *domain.Student {
	t.Helper()
	s, err := domain.NewImportedDraft(domain.NewApplicantParams{
		ID: "d1", TenantID: "t1", FirstName: "Ana", LastName: "Hernández", Level: primaria(t),
	})
	if err != nil {
		t.Fatalf("NewImportedDraft: %v", err)
	}
	return s
}

func TestNewImportedDraftIsADraftWithoutCURP(t *testing.T) {
	s := newDraft(t)
	if s.Status() != domain.AdmissionDraft || !s.IsDraft() {
		t.Fatalf("status = %v; want draft", s.Status())
	}
	if !s.CURP().IsZero() {
		t.Fatalf("curp = %q; want none", s.CURP())
	}
}

func TestNewImportedDraftRefusesACURP(t *testing.T) {
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	_, err := domain.NewImportedDraft(domain.NewApplicantParams{
		ID: "d1", TenantID: "t1", CURP: curp, FirstName: "Ana", LastName: "H", Level: primaria(t),
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v; want KindInvalid (a pupil with a CURP is imported, not drafted)", err)
	}
}

func TestDraftStatusParsesAndIsNotTerminal(t *testing.T) {
	got, err := domain.ParseAdmissionStatus("draft")
	if err != nil || got != domain.AdmissionDraft {
		t.Fatalf("ParseAdmissionStatus(draft) = %v, %v", got, err)
	}
	if domain.AdmissionDraft.IsTerminal() {
		t.Fatal("a draft is not terminal: a CURP promotes it")
	}
}

func TestDraftTransitions(t *testing.T) {
	cases := []struct {
		name string
		move func(*domain.Student) error
		ok   bool
	}{
		{"accept needs a CURP first", func(s *domain.Student) error { _, err := s.Accept(); return err }, false},
		{"reject is for an applicant", func(s *domain.Student) error { _, err := s.Reject("x"); return err }, false},
		{"enroll needs a CURP first", func(s *domain.Student) error { _, _, err := s.Enroll("c"); return err }, false},
		{"withdraw discards the draft", func(s *domain.Student) error { _, err := s.Withdraw("x"); return err }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.move(newDraft(t))
			if tc.ok && err != nil {
				t.Fatalf("err = %v; want ok", err)
			}
			if !tc.ok && glerrors.KindOf(err) != glerrors.KindConflict {
				t.Fatalf("err = %v; want KindConflict", err)
			}
		})
	}
}

func TestUpdateDetailsCURPRules(t *testing.T) {
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	cases := []struct {
		name       string
		student    func(*testing.T) *domain.Student
		curp       domain.CURP
		wantStatus domain.AdmissionStatus
		wantKind   glerrors.Kind
	}{
		{"draft stays a draft without a CURP", newDraft, "", domain.AdmissionDraft, glerrors.KindUnknown},
		{"a valid CURP promotes the draft to pending", newDraft, curp, domain.AdmissionPending, glerrors.KindUnknown},
		{"a pupil that is not a draft keeps needing one", newApplicant, "", domain.AdmissionPending, glerrors.KindInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.student(t)
			err := s.UpdateDetails("Ana", "Hernández", "", tc.curp, domain.Profile{})
			if glerrors.KindOf(err) != tc.wantKind {
				t.Fatalf("err = %v; want kind %v", err, tc.wantKind)
			}
			if s.Status() != tc.wantStatus {
				t.Fatalf("status = %v; want %v", s.Status(), tc.wantStatus)
			}
		})
	}
}

func TestRequirePlaceableRefusesADraft(t *testing.T) {
	if err := newDraft(t).RequirePlaceable(); !errors.Is(err, domain.ErrDraftNotPlaceable) {
		t.Fatalf("err = %v; want ErrDraftNotPlaceable", err)
	}
	if glerrors.KindOf(domain.ErrDraftNotPlaceable) != glerrors.KindConflict {
		t.Fatal("ErrDraftNotPlaceable must be a KindConflict (FailedPrecondition-like refusal)")
	}
	if err := newApplicant(t).RequirePlaceable(); err != nil {
		t.Fatalf("an applicant is placeable: %v", err)
	}
}

func TestChangedIssueFields(t *testing.T) {
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	full, _ := domain.NewProfile(validProfile(), today)
	cases := []struct {
		name   string
		mutate func(*testing.T, *domain.Student)
		want   []domain.IssueField
	}{
		{"nothing changed", func(t *testing.T, s *domain.Student) {}, nil},
		{"the CURP", func(t *testing.T, s *domain.Student) {
			other, _ := domain.NewCURP("LOPB060415HDFRNN09")
			mustUpdate(t, s, s.FirstName(), s.LastName(), s.MaternalLastName(), other, s.Profile())
		}, []domain.IssueField{domain.FieldCURP}},
		{"the maternal surname", func(t *testing.T, s *domain.Student) {
			mustUpdate(t, s, s.FirstName(), s.LastName(), "Otra", s.CURP(), s.Profile())
		}, []domain.IssueField{domain.FieldSurnames}},
		{"first name and birth date", func(t *testing.T, s *domain.Student) {
			in := validProfile()
			in.BirthDate = "2018-01-01"
			p, _ := domain.NewProfile(in, today)
			mustUpdate(t, s, "Ada", s.LastName(), s.MaternalLastName(), s.CURP(), p)
		}, []domain.IssueField{domain.FieldFirstName, domain.FieldBirthDate}},
		{"the mobile phone only", func(t *testing.T, s *domain.Student) {
			in := validProfile()
			in.Phones = replaceKind(in.Phones, domain.PhoneMobile, "5599999999")
			p, err := domain.NewProfile(in, today)
			if err != nil {
				t.Fatal(err)
			}
			mustUpdate(t, s, s.FirstName(), s.LastName(), s.MaternalLastName(), s.CURP(), p)
		}, []domain.IssueField{domain.FieldPhoneMobile}},
		{"email, postal code, state and street", func(t *testing.T, s *domain.Student) {
			in := validProfile()
			in.Email = "otra@correo.mx"
			in.Address.PostalCode = "58000"
			in.Address.State = "Michoacán"
			in.Address.Street = "Otra calle"
			p, err := domain.NewProfile(in, today)
			if err != nil {
				t.Fatal(err)
			}
			mustUpdate(t, s, s.FirstName(), s.LastName(), s.MaternalLastName(), s.CURP(), p)
		}, []domain.IssueField{domain.FieldEmail, domain.FieldPostalCode, domain.FieldState, domain.FieldAddress}},
		{"the bank reference", func(t *testing.T, s *domain.Student) { s.ChangeBankReference("998877") },
			[]domain.IssueField{domain.FieldBankReference}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newApplicant(t)
			mustUpdate(t, s, s.FirstName(), s.LastName(), s.MaternalLastName(), curp, full)
			before := s.Clone()
			tc.mutate(t, s)
			got := domain.ChangedIssueFields(before, s)
			if len(got) != len(tc.want) {
				t.Fatalf("changed = %v; want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("changed = %v; want %v", got, tc.want)
				}
			}
		})
	}
}

func mustUpdate(t *testing.T, s *domain.Student, first, last, maternal string, curp domain.CURP, p domain.Profile) {
	t.Helper()
	if err := s.UpdateDetails(first, last, maternal, curp, p); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
}

func replaceKind(phones []domain.Phone, kind domain.PhoneKind, number string) []domain.Phone {
	out := make([]domain.Phone, 0, len(phones))
	for _, ph := range phones {
		if ph.Kind == kind {
			ph.Number = number
		}
		out = append(out, ph)
	}
	return out
}
