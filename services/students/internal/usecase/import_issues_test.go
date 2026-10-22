package usecase_test

import (
	"context"
	"fmt"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// issueView is what a test compares of a stored issue.
type issueView struct {
	student, field, code, original, stored, group string
}

func viewsOf(issues []domain.ImportIssue) []issueView {
	out := make([]issueView, 0, len(issues))
	for _, i := range issues {
		out = append(out, issueView{i.StudentID(), string(i.Field()), i.Code(), i.OriginalValue(), i.StoredValue(), i.IntendedGroupID()})
	}
	return out
}

func assertIssues(t *testing.T, got []domain.ImportIssue, want []issueView) {
	t.Helper()
	views := viewsOf(got)
	if fmt.Sprint(views) != fmt.Sprint(want) {
		t.Fatalf("issues =\n%+v\nwant\n%+v", views, want)
	}
	for _, i := range got {
		if i.BatchID() != importBatch || !i.CreatedAt().Equal(importNow) || !i.IsOpen() || i.TenantID() != tenant {
			t.Fatalf("issue %+v: want open, of batch %s, created now, in the tenant", i, importBatch)
		}
	}
}

func draftRow(n int, curp string) usecase.ImportRow {
	r := row(n, curp)
	r.Action = usecase.ImportActionDraft
	return r
}

func TestImportStoresARowWithoutCURPAsADraft(t *testing.T) {
	cases := []struct {
		name, curp, code string
	}{
		{"missing", "", domain.IssueCURPMissing},
		{"invalid", "HEGA0503", domain.IssueCURPInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newImportFixture(t)
			got := f.run(t, false, draftRow(4, tc.curp))[0]
			if got.Outcome != usecase.ImportImported || !got.Draft || got.EnrollmentNumber == "" {
				t.Fatalf("result = %+v; want an IMPORTED draft with its matrícula", got)
			}
			cmd := f.repo.imported[0]
			s := cmd.Student
			if !s.IsDraft() || !s.CURP().IsZero() || s.Level().ID() != primariaLevel || s.GradeGroup() != "" {
				t.Fatalf("draft = status %v curp %q level %q group %q; want a draft of the group's level, unplaced",
					s.Status(), s.CURP(), s.Level().ID(), s.GradeGroup())
			}
			if cmd.Enrollment.ID() != "" {
				t.Fatalf("a draft was given an enrollment: %+v", cmd.Enrollment)
			}
			assertIssues(t, f.repo.issues, []issueView{{s.ID(), "curp", tc.code, tc.curp, "", importGroupA}})
		})
	}
}

func TestImportDraftRules(t *testing.T) {
	t.Run("without the draft action a row without CURP is still rejected", func(t *testing.T) {
		f := newImportFixture(t)
		got := f.run(t, false, row(1, ""))[0]
		if got.Outcome != usecase.ImportRejected || got.Reason != usecase.ReasonCURPInvalid {
			t.Fatalf("result = %+v; want REJECTED curp_invalid", got)
		}
	})
	t.Run("a draft row with a valid CURP imports normally", func(t *testing.T) {
		f := newImportFixture(t)
		got := f.run(t, false, draftRow(1, importCURP))[0]
		if got.Outcome != usecase.ImportImported || got.Draft || f.repo.imported[0].Student.Status() != domain.AdmissionEnrolled {
			t.Fatalf("result = %+v; want an enrolled pupil", got)
		}
	})
	t.Run("several drafts in one file never collide", func(t *testing.T) {
		f := newImportFixture(t)
		results := f.run(t, false, draftRow(1, ""), draftRow(2, ""))
		for _, r := range results {
			if r.Outcome != usecase.ImportImported || !r.Draft {
				t.Fatalf("row %d = %+v; want an IMPORTED draft", r.RowNumber, r)
			}
		}
	})
	t.Run("a draft still needs its names and a known group", func(t *testing.T) {
		f := newImportFixture(t)
		noName, noGroup := draftRow(1, ""), draftRow(2, "")
		noName.FirstName = ""
		noGroup.GroupID = "4a000000-0000-0000-0000-0000000000ff"
		results := f.run(t, false, noName, noGroup)
		if results[0].Reason != usecase.ReasonNameRequired || results[1].Reason != usecase.ReasonGroupNotFound {
			t.Fatalf("results = %+v; want name_required and group_not_found", results)
		}
	})
	t.Run("validating judges a draft and writes nothing", func(t *testing.T) {
		f := newImportFixture(t)
		got := f.run(t, true, draftRow(1, ""))[0]
		if got.Outcome != usecase.ImportImported || !got.Draft || len(f.repo.imported) != 0 || len(f.repo.issues) != 0 {
			t.Fatalf("result = %+v (stored %d); want a judged draft, nothing written", got, len(f.repo.imported))
		}
	})
}

func TestImportStoresEveryWarningAsAnIssueWithTheOriginalValue(t *testing.T) {
	f := newImportFixture(t)
	r := row(1, importCURP)
	r.BankReference = "S/R 123"
	r.Profile = domain.ProfileInput{
		Email:   "nope@",
		Phones:  []domain.Phone{{Kind: "casa", Number: "123"}, {Kind: "celular", Number: "999"}},
		Address: domain.Address{State: "Narnia", PostalCode: "4410"},
	}
	// The spreadsheet's own text, before the gateway tidied it.
	r.Original = usecase.ImportOriginal{PhoneHome: "Tel. 123", Email: "NOPE@ "}
	r.Issues = []usecase.ImportIssueInput{{Field: "birth_date", Code: "birth_date_from_curp", OriginalValue: "31/02/2005", StoredValue: "2005-03-12"}}

	got := f.run(t, false, r)[0]
	if got.Outcome != usecase.ImportImported {
		t.Fatalf("result = %+v", got)
	}
	id := got.StudentID
	assertIssues(t, f.repo.issues, []issueView{
		{id, "birth_date", "birth_date_from_curp", "31/02/2005", "2005-03-12", ""},
		{id, "bank_reference", usecase.WarnBankReferenceInvalid, "S/R 123", "", ""},
		{id, "email", domain.WarnEmailInvalid, "NOPE@ ", "", ""},
		{id, "phone_home", domain.WarnPhoneInvalid, "Tel. 123", "", ""},
		{id, "phone_mobile", domain.WarnPhoneInvalid, "999", "", ""},
		{id, "postal_code", domain.WarnPostalCodeInvalid, "4410", "", ""},
		{id, "state", domain.WarnStateUnknown, "Narnia", "", ""},
	})
}

func TestImportStoresATakenReferenceAsAnIssue(t *testing.T) {
	t.Run("dropped by the store's check", func(t *testing.T) {
		f := newImportFixture(t)
		f.repo.refsTaken["555"] = true
		r := row(1, importCURP)
		r.BankReference = "555"
		got := f.run(t, false, r)[0]
		assertIssues(t, f.repo.issues, []issueView{{got.StudentID, "bank_reference", usecase.WarnBankReferenceTaken, "555", "", ""}})
	})
	t.Run("taken between the check and the insert", func(t *testing.T) {
		f := newImportFixture(t)
		f.repo.interrupt[importCURP] = func() error { return fmt.Errorf("%w: race", domain.ErrBankReferenceTaken) }
		r := row(1, importCURP)
		r.BankReference = "999"
		got := f.run(t, false, r)[0]
		assertIssues(t, f.repo.issues, []issueView{{got.StudentID, "bank_reference", usecase.WarnBankReferenceTaken, "999", "", ""}})
	})
	t.Run("a reference that is free leaves no issue", func(t *testing.T) {
		f := newImportFixture(t)
		r := row(1, importCURP)
		r.BankReference = "777"
		f.run(t, false, r)
		if len(f.repo.issues) != 0 {
			t.Fatalf("issues = %+v; want none", viewsOf(f.repo.issues))
		}
	})
}

func TestImportClipsAnOriginalValueTooLongToStore(t *testing.T) {
	f := newImportFixture(t)
	r := row(1, importCURP)
	long := make([]rune, 700)
	for i := range long {
		long[i] = 'x'
	}
	r.Profile = domain.ProfileInput{Address: domain.Address{Street: string(long)}}
	f.run(t, false, r)
	if len(f.repo.issues) != 1 || len([]rune(f.repo.issues[0].OriginalValue())) != 500 {
		t.Fatalf("issues = %+v; want one address issue with 500 runes of the original", viewsOf(f.repo.issues))
	}
}

func TestImportRefusesAGatewayIssueOfAnUnknownField(t *testing.T) {
	f := newImportFixture(t)
	r := row(1, importCURP)
	r.Issues = []usecase.ImportIssueInput{{Field: "favourite_colour", Code: "x"}}
	_, err := f.svc.ImportStudents(context.Background(), []usecase.ImportRow{r}, false, importBatch)
	if glerrors.KindOf(err) != glerrors.KindInvalid || len(f.repo.imported) != 0 {
		t.Fatalf("err = %v (stored %d); want KindInvalid before anything is stored", err, len(f.repo.imported))
	}
}
