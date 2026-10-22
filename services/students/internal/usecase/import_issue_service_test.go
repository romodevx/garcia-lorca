package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// fakeIssueRepo is an in-memory ImportIssueRepository.
type fakeIssueRepo struct {
	byID     map[string]domain.ImportIssue
	accepted []domain.ImportIssue
	listed   []bool // include_resolved per call
}

func newIssueRepo() *fakeIssueRepo { return &fakeIssueRepo{byID: map[string]domain.ImportIssue{}} }

func (r *fakeIssueRepo) ListImportIssues(_ context.Context, studentID string, includeResolved bool) ([]domain.ImportIssue, error) {
	r.listed = append(r.listed, includeResolved)
	var out []domain.ImportIssue
	for _, i := range r.byID {
		if i.StudentID() == studentID && (includeResolved || i.IsOpen()) {
			out = append(out, i)
		}
	}
	return out, nil
}

func (r *fakeIssueRepo) ImportIssueByID(_ context.Context, studentID, issueID string) (domain.ImportIssue, error) {
	i, ok := r.byID[issueID]
	if !ok || i.StudentID() != studentID {
		return domain.ImportIssue{}, glerrors.ErrNotFound
	}
	return i, nil
}

func (r *fakeIssueRepo) ResolveImportIssue(_ context.Context, issue domain.ImportIssue) (domain.ImportIssue, error) {
	r.accepted = append(r.accepted, issue)
	r.byID[issue.ID()] = issue
	return issue, nil
}

const (
	issueOne = "1d000000-0000-0000-0000-000000000001"
	issueTwo = "1d000000-0000-0000-0000-000000000002"
)

var issueNow = time.Date(2026, 10, 21, 18, 0, 0, 0, time.UTC)

func seedIssue(t *testing.T, r *fakeIssueRepo, id, studentID string, field domain.IssueField) domain.ImportIssue {
	t.Helper()
	i, err := domain.NewImportIssue(domain.ImportIssueParams{
		ID: id, TenantID: tenant, StudentID: studentID, Field: string(field), Code: "phone_invalid",
		OriginalValue: "123", CreatedAt: issueNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	r.byID[id] = i
	return i
}

func newIssueService(t *testing.T) (*usecase.ImportIssueService, *fakeRepo, *fakeIssueRepo) {
	t.Helper()
	students, issues := newRepo(), newIssueRepo()
	return usecase.NewImportIssueService(issues, students, func() time.Time { return issueNow }), students, issues
}

func TestListImportIssues(t *testing.T) {
	svc, students, issues := newIssueService(t)
	seedStudent(t, students, "stu-1")
	seedIssue(t, issues, issueOne, "stu-1", domain.FieldPhoneHome)

	got, err := svc.ListImportIssues(context.Background(), "stu-1", true)
	if err != nil || len(got) != 1 || got[0].ID() != issueOne || issues.listed[0] != true {
		t.Fatalf("got %+v, %v; want the pupil's issue", got, err)
	}
	if _, err := svc.ListImportIssues(context.Background(), "nobody", false); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("unknown pupil: err = %v; want KindNotFound", err)
	}
}

func TestAcceptImportIssue(t *testing.T) {
	t.Run("closes it as accepted by the caller", func(t *testing.T) {
		svc, students, issues := newIssueService(t)
		seedStudent(t, students, "stu-1")
		seedIssue(t, issues, issueOne, "stu-1", domain.FieldPhoneHome)
		got, err := svc.AcceptImportIssue(context.Background(), "stu-1", issueOne, actorUser)
		if err != nil {
			t.Fatalf("AcceptImportIssue: %v", err)
		}
		if got.Resolution() != domain.ResolutionAccepted || got.ResolvedBy() != actorUser || !got.ResolvedAt().Equal(issueNow) {
			t.Fatalf("issue = %+v; want accepted by the caller now", got)
		}
		if len(issues.accepted) != 1 {
			t.Fatalf("stored %d resolutions; want 1", len(issues.accepted))
		}
	})
	t.Run("an issue already resolved is answered as it stands", func(t *testing.T) {
		svc, students, issues := newIssueService(t)
		seedStudent(t, students, "stu-1")
		issues.byID[issueOne] = seedIssue(t, issues, issueOne, "stu-1", domain.FieldEmail).Accept("someone-else", issueNow.Add(-time.Minute))
		got, err := svc.AcceptImportIssue(context.Background(), "stu-1", issueOne, actorUser)
		if err != nil || got.ResolvedBy() != "someone-else" || len(issues.accepted) != 0 {
			t.Fatalf("got %+v, %v (writes %d); want the first resolution, no write", got, err, len(issues.accepted))
		}
	})
	t.Run("a draft's curp issue closes only with a CURP", func(t *testing.T) {
		svc, students, issues := newIssueService(t)
		seedDraft(t, students, "stu-d")
		seedIssue(t, issues, issueOne, "stu-d", domain.FieldCURP)
		if _, err := svc.AcceptImportIssue(context.Background(), "stu-d", issueOne, actorUser); !errors.Is(err, domain.ErrDraftNotPlaceable) {
			t.Fatalf("err = %v; want ErrDraftNotPlaceable", err)
		}
		if len(issues.accepted) != 0 {
			t.Fatal("the curp issue was closed")
		}
	})
	t.Run("a draft's other issues may be accepted", func(t *testing.T) {
		svc, students, issues := newIssueService(t)
		seedDraft(t, students, "stu-d")
		seedIssue(t, issues, issueTwo, "stu-d", domain.FieldPhoneMobile)
		if _, err := svc.AcceptImportIssue(context.Background(), "stu-d", issueTwo, actorUser); err != nil {
			t.Fatalf("AcceptImportIssue: %v", err)
		}
	})
	refusals := []struct {
		name           string
		student, issue string
		user           string
		want           glerrors.Kind
	}{
		{"another pupil's issue", "stu-2", issueOne, actorUser, glerrors.KindNotFound},
		{"an unknown pupil", "nobody", issueOne, actorUser, glerrors.KindNotFound},
		{"an issue id that is not one", "stu-1", "nope", actorUser, glerrors.KindNotFound},
		{"a caller that is not a user id", "stu-1", issueOne, "someone", glerrors.KindInvalid},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			svc, students, issues := newIssueService(t)
			seedStudent(t, students, "stu-1")
			seedStudent(t, students, "stu-2")
			seedIssue(t, issues, issueOne, "stu-1", domain.FieldPhoneHome)
			if _, err := svc.AcceptImportIssue(context.Background(), tc.student, tc.issue, tc.user); glerrors.KindOf(err) != tc.want {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if len(issues.accepted) != 0 {
				t.Fatal("something was resolved")
			}
		})
	}
}

// A withdrawn pupil's file is read-only (T14): its issues are not closed either.
func TestAcceptImportIssueOfAWithdrawnPupilIsRefused(t *testing.T) {
	svc, students, issues := newIssueService(t)
	students.byID["stu-w"] = domain.Rehydrate(domain.NewApplicantParams{
		ID: "stu-w", TenantID: tenant, FirstName: "Ana", LastName: "H",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionWithdrawn)
	seedIssue(t, issues, issueOne, "stu-w", domain.FieldPhoneHome)
	if _, err := svc.AcceptImportIssue(context.Background(), "stu-w", issueOne, actorUser); !errors.Is(err, domain.ErrWithdrawnReadOnly) {
		t.Fatalf("err = %v; want ErrWithdrawnReadOnly", err)
	}
	if len(issues.accepted) != 0 {
		t.Fatal("a withdrawn pupil's issue was closed")
	}
}
