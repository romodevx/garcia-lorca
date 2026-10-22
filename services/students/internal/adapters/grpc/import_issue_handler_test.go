package grpcadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	handlerActor = "a0000000-0000-0000-0000-0000000000a1"
	handlerIssue = "1d000000-0000-0000-0000-000000000001"
	draftGroupID = "4a000000-0000-0000-0000-00000000000d"
)

var handlerNow = time.Date(2026, 10, 21, 18, 0, 0, 0, time.UTC)

// fakeIssueRepo is an in-memory ImportIssueRepository.
type fakeIssueRepo struct{ byID map[string]domain.ImportIssue }

func (r *fakeIssueRepo) ListImportIssues(_ context.Context, studentID string, _ bool) ([]domain.ImportIssue, error) {
	var out []domain.ImportIssue
	for _, i := range r.byID {
		if i.StudentID() == studentID {
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

func (r *fakeIssueRepo) ResolveImportIssue(_ context.Context, i domain.ImportIssue) (domain.ImportIssue, error) {
	r.byID[i.ID()] = i
	return i, nil
}

// issueHandler serves the student RPCs and the import-issue ones over the same
// in-memory pupils.
func issueHandler(t *testing.T) (*grpcadapter.StudentsHandler, *fakeRepo, *fakeIssueRepo) {
	t.Helper()
	repo, issues := newFakeRepo(), &fakeIssueRepo{byID: map[string]domain.ImportIssue{}}
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students")).
		WithImportIssues(usecase.NewImportIssueService(issues, repo, func() time.Time { return handlerNow }))
	return h, repo, issues
}

func putDraft(repo *fakeRepo, id string, open int) {
	s := domain.Rehydrate(domain.NewApplicantParams{
		ID: id, TenantID: testTenant, FirstName: "Ana", LastName: "Herrera",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionDraft)
	s.AssignOpenIssues(open)
	repo.byID[id] = s
}

func putIssue(t *testing.T, issues *fakeIssueRepo, studentID string, field domain.IssueField) {
	t.Helper()
	i, err := domain.NewImportIssue(domain.ImportIssueParams{
		ID: handlerIssue, TenantID: testTenant, StudentID: studentID, Field: string(field), Code: "curp_missing",
		OriginalValue: "", IntendedGroupID: draftGroupID, BatchID: importHandlerBatch, CreatedAt: handlerNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	issues.byID[handlerIssue] = i
}

func TestAStudentCarriesItsOpenIssuesAndDraftOnTheWire(t *testing.T) {
	h, repo, _ := issueHandler(t)
	putDraft(repo, "stu-d", 2)
	res, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: "stu-d"}))
	if err != nil {
		t.Fatalf("GetStudent: %v", err)
	}
	s := res.Msg.GetStudent()
	if s.GetOpenIssues() != 2 || !s.GetDraft() || s.GetAdmissionStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_DRAFT || s.GetCurp() != "" {
		t.Fatalf("student = %+v; want a draft with 2 open issues and no CURP", s)
	}
}

func TestUpdateStudentPromotesADraftOnTheWire(t *testing.T) {
	h, repo, _ := issueHandler(t)
	putDraft(repo, "stu-d", 1)
	repo.correction = ports.CorrectionResult{Corrected: 1, IntendedGroupID: draftGroupID}

	res, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-d", FirstName: "Ana", LastName: "Herrera", Curp: validCURP, ActorUserId: handlerActor,
	}))
	if err != nil {
		t.Fatalf("UpdateStudent: %v", err)
	}
	m := res.Msg
	if !m.GetPromotedFromDraft() || m.GetIntendedGroupId() != draftGroupID || m.GetStudent().GetDraft() || m.GetStudent().GetOpenIssues() != 0 {
		t.Fatalf("response = %+v; want promoted, the intended group, no draft, no open issue", m)
	}
	if len(repo.corrections) != 1 || repo.corrections[0].By != handlerActor {
		t.Fatalf("corrections = %+v; want one by the actor", repo.corrections)
	}
}

func TestListImportIssuesMapsTheWire(t *testing.T) {
	h, repo, issues := issueHandler(t)
	putDraft(repo, "stu-d", 1)
	putIssue(t, issues, "stu-d", domain.FieldCURP)

	res, err := h.ListImportIssues(context.Background(), connect.NewRequest(&studentsv1.ListImportIssuesRequest{StudentId: "stu-d"}))
	if err != nil {
		t.Fatalf("ListImportIssues: %v", err)
	}
	got := res.Msg.GetIssues()
	if len(got) != 1 {
		t.Fatalf("issues = %d; want 1", len(got))
	}
	i := got[0]
	if i.GetId() != handlerIssue || i.GetStudentId() != "stu-d" || i.GetField() != "curp" || i.GetCode() != "curp_missing" ||
		i.GetIntendedGroupId() != draftGroupID || i.GetImportBatchId() != importHandlerBatch ||
		!i.GetCreatedAt().AsTime().Equal(handlerNow.Add(-time.Hour)) || i.GetResolvedAt() != nil || i.GetResolution() != "" {
		t.Fatalf("issue = %+v", i)
	}

	_, err = h.ListImportIssues(context.Background(), connect.NewRequest(&studentsv1.ListImportIssuesRequest{StudentId: "nobody"}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestAcceptImportIssueOnTheWire(t *testing.T) {
	t.Run("accepted by the caller", func(t *testing.T) {
		h, repo, issues := issueHandler(t)
		putDraft(repo, "stu-d", 1)
		putIssue(t, issues, "stu-d", domain.FieldPhoneMobile)
		res, err := h.AcceptImportIssue(context.Background(), connect.NewRequest(&studentsv1.AcceptImportIssueRequest{
			StudentId: "stu-d", IssueId: handlerIssue, UserId: handlerActor,
		}))
		if err != nil {
			t.Fatalf("AcceptImportIssue: %v", err)
		}
		i := res.Msg.GetIssue()
		if i.GetResolution() != "accepted" || i.GetResolvedBy() != handlerActor || !i.GetResolvedAt().AsTime().Equal(handlerNow) {
			t.Fatalf("issue = %+v; want accepted by the caller now", i)
		}
	})
	t.Run("a draft's curp issue is refused with its code", func(t *testing.T) {
		h, repo, issues := issueHandler(t)
		putDraft(repo, "stu-d", 1)
		putIssue(t, issues, "stu-d", domain.FieldCURP)
		_, err := h.AcceptImportIssue(context.Background(), connect.NewRequest(&studentsv1.AcceptImportIssueRequest{
			StudentId: "stu-d", IssueId: handlerIssue, UserId: handlerActor,
		}))
		assertCode(t, err, connect.CodeAlreadyExists)
		var ce *connect.Error
		if !errors.As(err, &ce) || ce.Meta().Get(grpcadapter.ErrorCodeMeta) != domain.CodeStudentDraft {
			t.Fatalf("err = %v; want X-Error-Code %s", err, domain.CodeStudentDraft)
		}
	})
	t.Run("another pupil's issue is not found", func(t *testing.T) {
		h, repo, issues := issueHandler(t)
		putDraft(repo, "stu-d", 1)
		putDraft(repo, "stu-x", 0)
		putIssue(t, issues, "stu-d", domain.FieldEmail)
		_, err := h.AcceptImportIssue(context.Background(), connect.NewRequest(&studentsv1.AcceptImportIssueRequest{
			StudentId: "stu-x", IssueId: handlerIssue, UserId: handlerActor,
		}))
		assertCode(t, err, connect.CodeNotFound)
	})
}

func TestImportIssueRPCsWithoutTheServiceAreUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ListImportIssues(context.Background(), connect.NewRequest(&studentsv1.ListImportIssuesRequest{}))
	assertCode(t, err, connect.CodeUnimplemented)
	_, err = h.AcceptImportIssue(context.Background(), connect.NewRequest(&studentsv1.AcceptImportIssueRequest{}))
	assertCode(t, err, connect.CodeUnimplemented)
}

func TestImportStudentsCarriesTheActionIssuesAndOriginals(t *testing.T) {
	h, repo := importHandler(t)
	res, err := h.ImportStudents(context.Background(), connect.NewRequest(&studentsv1.ImportStudentsRequest{
		BatchId: importHandlerBatch,
		Rows: []*studentsv1.ImportStudentRow{{
			RowNumber: 5, FirstName: "Ana", LastName: "Herrera", GroupId: enrollGroup,
			Action:   studentsv1.ImportRowAction_IMPORT_ROW_ACTION_DRAFT,
			Profile:  &studentsv1.StudentProfile{Phones: []*studentsv1.StudentPhone{{Kind: "casa", Number: "123"}}},
			Issues:   []*studentsv1.ImportIssue{{Field: "birth_date", Code: "birth_date_invalid", OriginalValue: "31/02/19"}},
			Original: &studentsv1.ImportOriginalValues{PhoneHome: "Tel 123"},
		}},
	}))
	if err != nil {
		t.Fatalf("ImportStudents: %v", err)
	}
	if r := res.Msg.GetResults()[0]; r.GetOutcome() != studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_IMPORTED || !r.GetDraft() {
		t.Fatalf("row = %+v; want an IMPORTED draft", r)
	}
	cmd := repo.stored[0]
	if !cmd.Student.IsDraft() || len(cmd.Issues) != 3 {
		t.Fatalf("stored = draft %v, %d issues; want a draft with curp + gateway + phone issues", cmd.Student.IsDraft(), len(cmd.Issues))
	}
	if i := cmd.Issues[1]; i.Field() != domain.FieldBirthDate || i.OriginalValue() != "31/02/19" {
		t.Errorf("gateway issue = %+v", i)
	}
	if i := cmd.Issues[2]; i.Field() != domain.FieldPhoneHome || i.OriginalValue() != "Tel 123" {
		t.Errorf("phone issue = %+v; want the spreadsheet's original text", i)
	}
}
