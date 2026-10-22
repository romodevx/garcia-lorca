package domain_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var issueAt = time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)

func issueParams() domain.ImportIssueParams {
	return domain.ImportIssueParams{
		ID: "i1", TenantID: "t1", StudentID: "s1", Field: "phone_mobile", Code: "phone_invalid",
		OriginalValue: "55 1234", BatchID: "b1", CreatedAt: issueAt,
	}
}

func TestNewImportIssueValidates(t *testing.T) {
	bad := map[string]func(*domain.ImportIssueParams){
		"blank id":       func(p *domain.ImportIssueParams) { p.ID = "" },
		"blank student":  func(p *domain.ImportIssueParams) { p.StudentID = "" },
		"unknown field":  func(p *domain.ImportIssueParams) { p.Field = "favourite_colour" },
		"blank code":     func(p *domain.ImportIssueParams) { p.Code = " " },
		"huge original":  func(p *domain.ImportIssueParams) { p.OriginalValue = strings.Repeat("x", 501) },
		"huge stored":    func(p *domain.ImportIssueParams) { p.StoredValue = strings.Repeat("x", 501) },
		"no created_at":  func(p *domain.ImportIssueParams) { p.CreatedAt = time.Time{} },
		"overlong code":  func(p *domain.ImportIssueParams) { p.Code = strings.Repeat("c", 61) },
		"unknown status": func(p *domain.ImportIssueParams) { p.Resolution = "ignored" },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			p := issueParams()
			mutate(&p)
			if _, err := domain.NewImportIssue(p); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("err = %v; want KindInvalid", err)
			}
		})
	}
	issue, err := domain.NewImportIssue(issueParams())
	if err != nil {
		t.Fatalf("NewImportIssue: %v", err)
	}
	if !issue.IsOpen() || issue.Field() != domain.FieldPhoneMobile || issue.OriginalValue() != "55 1234" {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestImportIssueAccept(t *testing.T) {
	issue, _ := domain.NewImportIssue(issueParams())
	accepted := issue.Accept("u1", issueAt.Add(time.Hour))
	if accepted.IsOpen() || accepted.Resolution() != domain.ResolutionAccepted || accepted.ResolvedBy() != "u1" {
		t.Fatalf("accepted = %+v", accepted)
	}
	if !accepted.ResolvedAt().Equal(issueAt.Add(time.Hour)) {
		t.Fatalf("resolved_at = %v", accepted.ResolvedAt())
	}
	// A resolved issue is answered as it stands.
	again := accepted.Accept("u2", issueAt.Add(2*time.Hour))
	if again.ResolvedBy() != "u1" || !again.ResolvedAt().Equal(accepted.ResolvedAt()) {
		t.Fatalf("a second accept rewrote the resolution: %+v", again)
	}
	if !issue.IsOpen() {
		t.Fatal("Accept must not mutate the receiver")
	}
}

func TestDraftCURPIssueIsNotAccepted(t *testing.T) {
	p := issueParams()
	p.Field, p.Code, p.IntendedGroupID = "curp", "curp_missing", "g1"
	issue, _ := domain.NewImportIssue(p)
	if !issue.ClosesOnlyByCorrection() {
		t.Fatal("a curp issue is closed only by capturing the CURP")
	}
	if issue.IntendedGroupID() != "g1" {
		t.Fatalf("intended group = %q", issue.IntendedGroupID())
	}
}

func TestProfileLenientReportsWhatItDropped(t *testing.T) {
	in := domain.ProfileInput{
		BirthDate: "12/03/2019",
		Email:     "ana@correo",
		Phones:    []domain.Phone{{Kind: "casa", Number: "123"}, {Kind: "celular", Number: "sin tel"}, {Kind: "celular", Number: "5512345678"}},
		Address: domain.Address{
			Street: strings.Repeat("ñ", 121), PostalCode: "4410", State: "Narnia",
		},
	}
	_, drops := domain.NewProfileLenientReport(in, today)
	want := []domain.ProfileDrop{
		{Field: domain.FieldBirthDate, Code: domain.WarnBirthDateInvalid, Original: "12/03/2019"},
		{Field: domain.FieldEmail, Code: domain.WarnEmailInvalid, Original: "ana@correo"},
		{Field: domain.FieldPhoneHome, Code: domain.WarnPhoneInvalid, Original: "123"},
		{Field: domain.FieldPhoneMobile, Code: domain.WarnPhoneInvalid, Original: "sin tel"},
		{Field: domain.FieldAddress, Code: domain.WarnAddressTruncated, Original: strings.Repeat("ñ", 121), Stored: strings.Repeat("ñ", 120)},
		{Field: domain.FieldPostalCode, Code: domain.WarnPostalCodeInvalid, Original: "4410"},
		{Field: domain.FieldState, Code: domain.WarnStateUnknown, Original: "Narnia"},
	}
	if !reflect.DeepEqual(drops, want) {
		t.Fatalf("drops =\n%+v\nwant\n%+v", drops, want)
	}
	// The codes view stays what the import always reported: once each.
	_, codes := domain.NewProfileLenient(in, today)
	wantCodes := []string{
		domain.WarnBirthDateInvalid, domain.WarnEmailInvalid, domain.WarnPhoneInvalid,
		domain.WarnAddressTruncated, domain.WarnPostalCodeInvalid, domain.WarnStateUnknown,
	}
	if !reflect.DeepEqual(codes, wantCodes) {
		t.Fatalf("codes = %v; want %v", codes, wantCodes)
	}
}

func TestProfileLenientReportsAFourthPhoneAsDropped(t *testing.T) {
	in := domain.ProfileInput{Phones: []domain.Phone{
		{Kind: "casa", Number: "4431111111"}, {Kind: "casa", Number: "4432222222"},
		{Kind: "celular", Number: "4433333333"}, {Kind: "celular", Number: "4434444444"},
	}}
	_, drops := domain.NewProfileLenientReport(in, today)
	want := []domain.ProfileDrop{{Field: domain.FieldPhoneMobile, Code: domain.WarnPhoneInvalid, Original: "4434444444"}}
	if !reflect.DeepEqual(drops, want) {
		t.Fatalf("drops = %+v; want %+v", drops, want)
	}
}
