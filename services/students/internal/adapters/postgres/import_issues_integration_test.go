//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// These tests' own ids (prefix 22) and CURPs: other suites count theirs.
const (
	issueActor       = "a0000000-0000-0000-0000-0000000022a1"
	issueIntendedGrp = "40000000-0000-0000-0000-0000000022a0"
)

var issueClock = time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)

func newIssue(t *testing.T, tenant, id, studentID string, field domain.IssueField, code, original, group string) domain.ImportIssue {
	t.Helper()
	i, err := domain.NewImportIssue(domain.ImportIssueParams{
		ID: id, TenantID: tenant, StudentID: studentID, Field: string(field), Code: code,
		OriginalValue: original, BatchID: importBatchID, IntendedGroupID: group, CreatedAt: issueClock,
	})
	if err != nil {
		t.Fatalf("NewImportIssue: %v", err)
	}
	return i
}

// draftCommand builds a pupil the import keeps as a draft (no CURP, no
// enrollment), with its curp issue and one more.
func draftCommand(t *testing.T, tenant, id, prefix string) ports.ImportCommand {
	t.Helper()
	student, err := domain.NewImportedDraft(domain.NewApplicantParams{
		ID: id, TenantID: tenant, FirstName: "Lucía", LastName: "de la Cruz",
		Level: domain.RestoreLevel(primariaLevelID, "Primaria"),
	})
	if err != nil {
		t.Fatalf("NewImportedDraft: %v", err)
	}
	return ports.ImportCommand{
		Student: student, BatchID: importBatchID, NumberYear: testNumberYear,
		Issues: []domain.ImportIssue{
			newIssue(t, tenant, prefix+"01", id, domain.FieldCURP, domain.IssueCURPMissing, "", issueIntendedGrp),
			newIssue(t, tenant, prefix+"02", id, domain.FieldPhoneMobile, domain.WarnPhoneInvalid, "55 12", ""),
		},
	}
}

func TestImportStoresADraftWithItsIssuesAndNoEnrollment(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-2200-0000-0000-000000000001"
	if _, err := repo.ImportStudent(ctx, draftCommand(t, tenantA, id, "1d220000-0000-0000-0000-0000000001")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}

	s, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !s.IsDraft() || !s.CURP().IsZero() || s.OpenIssues() != 2 {
		t.Fatalf("pupil = status %v curp %q open %d; want a draft without CURP and 2 open issues", s.Status(), s.CURP(), s.OpenIssues())
	}
	if n, _ := repo.CountAsTenant(ctx, "SELECT count(*) FROM enrollments WHERE student_id = ?", id); n != 0 {
		t.Fatalf("a draft was enrolled (%d rows)", n)
	}
	listed, _, err := repo.List(ctx, ports.ListFilter{IDs: []string{id}})
	if err != nil || len(listed) != 1 || listed[0].OpenIssues() != 2 || !listed[0].IsDraft() {
		t.Fatalf("List = %+v (err %v); want the draft with 2 open issues", listed, err)
	}
	issues, err := repo.ListImportIssues(ctx, id, false)
	if err != nil || len(issues) != 2 {
		t.Fatalf("ListImportIssues = %d (err %v); want 2", len(issues), err)
	}
	if c := issues[0]; c.Field() != domain.FieldCURP || c.IntendedGroupID() != issueIntendedGrp || c.BatchID() != importBatchID || !c.IsOpen() {
		t.Fatalf("curp issue = %+v", c)
	}
}

func TestSeveralDraftsCoexistWithoutACURP(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	for i, id := range []string{"aaaaaaaa-2200-0000-0000-000000000011", "aaaaaaaa-2200-0000-0000-000000000012"} {
		prefix := "1d220000-0000-0000-0000-000000000" + string(rune('8'+i))
		if _, err := repo.ImportStudent(ctx, draftCommand(t, tenantA, id, prefix)); err != nil {
			t.Fatalf("draft %d: %v (NULL CURPs must not collide)", i, err)
		}
	}
}

func TestImportStoresATakenReferenceIssueOnlyWhenDropped(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	holder := withBankReference(t, applicant(t, "aaaaaaaa-2200-0000-0000-000000000020", tenantA, "IIAA100312MDFRNN01", "1° A"), "22001")
	if err := repo.Create(ctx, holder, testNumberYear); err != nil {
		t.Fatalf("seed holder: %v", err)
	}
	for _, tc := range []struct {
		id, enrollment, curp, ref string
		want                      int64
	}{
		{"aaaaaaaa-2200-0000-0000-000000000021", "e2200000-0000-0000-0000-000000000021", "IIAB100312MDFRNN02", "22001", 1},
		{"aaaaaaaa-2200-0000-0000-000000000022", "e2200000-0000-0000-0000-000000000022", "IIAC100312MDFRNN03", "22002", 0},
	} {
		cmd := importCommand(t, tenantA, tc.id, tc.enrollment, tc.curp, tc.ref)
		taken := newIssue(t, tenantA, "1d220000-0000-0000-0000-0"+tc.id[len(tc.id)-11:], tc.id, domain.FieldBankReference, "bank_reference_taken", tc.ref, "")
		cmd.ReferenceTakenIssue = &taken
		if _, err := repo.ImportStudent(ctx, cmd); err != nil {
			t.Fatalf("ImportStudent: %v", err)
		}
		n, _ := repo.CountAsTenant(ctx, "SELECT count(*) FROM student_import_issues WHERE student_id = ? AND code = 'bank_reference_taken'", tc.id)
		if n != tc.want {
			t.Fatalf("pupil %s: %d taken issues; want %d", tc.id, n, tc.want)
		}
	}
}

func TestSavingACURPPromotesTheDraftAndClosesItsIssue(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-2200-0000-0000-000000000031"
	if _, err := repo.ImportStudent(ctx, draftCommand(t, tenantA, id, "1d220000-0000-0000-0000-0000000003")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	s, _ := repo.FindByID(ctx, id)
	if err := s.UpdateDetails(s.FirstName(), s.LastName(), "", mustCURP(t, "IIAD100312MDFRNN04"), s.Profile()); err != nil {
		t.Fatalf("UpdateDetails (domain): %v", err)
	}
	got, err := repo.UpdateDetails(ctx, s, ports.DetailsCorrection{Fields: []domain.IssueField{domain.FieldCURP}, By: issueActor})
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if got.Corrected != 1 || got.IntendedGroupID != issueIntendedGrp {
		t.Fatalf("result = %+v; want 1 corrected and the intended group", got)
	}
	reread, _ := repo.FindByID(ctx, id)
	if reread.Status() != domain.AdmissionPending || reread.CURP().String() != "IIAD100312MDFRNN04" || reread.OpenIssues() != 1 {
		t.Fatalf("pupil = %v %q open %d; want pending with its CURP and the phone issue still open", reread.Status(), reread.CURP(), reread.OpenIssues())
	}
	all, _ := repo.ListImportIssues(ctx, id, true)
	var curp domain.ImportIssue
	for _, i := range all {
		if i.Field() == domain.FieldCURP {
			curp = i
		}
	}
	if curp.Resolution() != domain.ResolutionCorrected || curp.ResolvedBy() != issueActor || curp.ResolvedAt().IsZero() {
		t.Fatalf("curp issue = %+v; want corrected by the actor", curp)
	}
	if all[0].Field() != domain.FieldPhoneMobile {
		t.Fatalf("order = %v first; want the open issue first", all[0].Field())
	}
}

func TestADuplicateCURPLeavesTheDraftAndItsIssuesAlone(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	seedStudent(t, tenantA, "aaaaaaaa-2200-0000-0000-000000000040", "IIAE100312MDFRNN05")
	const id = "aaaaaaaa-2200-0000-0000-000000000041"
	if _, err := repo.ImportStudent(ctx, draftCommand(t, tenantA, id, "1d220000-0000-0000-0000-0000000004")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	s, _ := repo.FindByID(ctx, id)
	_ = s.UpdateDetails(s.FirstName(), s.LastName(), "", mustCURP(t, "IIAE100312MDFRNN05"), s.Profile())
	_, err := repo.UpdateDetails(ctx, s, ports.DetailsCorrection{Fields: []domain.IssueField{domain.FieldCURP}, By: issueActor})
	if !errors.Is(err, domain.ErrCURPTaken) {
		t.Fatalf("err = %v; want ErrCURPTaken", err)
	}
	reread, _ := repo.FindByID(ctx, id)
	if !reread.IsDraft() || reread.OpenIssues() != 2 {
		t.Fatalf("pupil = %v open %d; want the draft untouched with both issues open", reread.Status(), reread.OpenIssues())
	}
}

func TestResolveImportIssueIsACompareAndSet(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-2200-0000-0000-000000000051"
	if _, err := repo.ImportStudent(ctx, draftCommand(t, tenantA, id, "1d220000-0000-0000-0000-0000000005")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	phone, err := repo.ImportIssueByID(ctx, id, "1d220000-0000-0000-0000-000000000502")
	if err != nil {
		t.Fatalf("ImportIssueByID: %v", err)
	}
	first, err := repo.ResolveImportIssue(ctx, phone.Accept(issueActor, issueClock.Add(time.Hour)))
	if err != nil || first.Resolution() != domain.ResolutionAccepted || first.ResolvedBy() != issueActor {
		t.Fatalf("first = %+v (err %v); want accepted by the actor", first, err)
	}
	other := "a0000000-0000-0000-0000-0000000022b2"
	second, err := repo.ResolveImportIssue(ctx, phone.Accept(other, issueClock.Add(2*time.Hour)))
	if err != nil || second.ResolvedBy() != issueActor || !second.ResolvedAt().Equal(first.ResolvedAt()) {
		t.Fatalf("second = %+v (err %v); want the first resolution kept", second, err)
	}
	if _, err := repo.ImportIssueByID(ctx, "aaaaaaaa-2200-0000-0000-000000000031", phone.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("another pupil's issue: err = %v; want KindNotFound", err)
	}
}

// RLS (migration 0022): tenant B neither reads nor writes tenant A's issues.
func TestImportIssuesAreTenantIsolated(t *testing.T) {
	ctx := context.Background()
	repoA, repoB := appRepo(t, tenantA), appRepo(t, tenantB)
	const id = "aaaaaaaa-2200-0000-0000-000000000061"
	if _, err := repoA.ImportStudent(ctx, draftCommand(t, tenantA, id, "1d220000-0000-0000-0000-0000000006")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	const issueID = "1d220000-0000-0000-0000-000000000602"

	if got, err := repoB.ListImportIssues(ctx, id, true); err != nil || len(got) != 0 {
		t.Fatalf("tenant B listed %d of A's issues (err %v): RLS breach", len(got), err)
	}
	if _, err := repoB.ImportIssueByID(ctx, id, issueID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("tenant B read A's issue: err = %v", err)
	}
	if n, _ := repoB.CountAsTenant(ctx, "SELECT count(*) FROM student_import_issues"); n != 0 {
		t.Fatalf("tenant B counts %d issues; want none of A's", n)
	}

	// Writes: an UPDATE from B matches nothing; an INSERT naming A is refused.
	if err := repoB.ExecAsTenant(ctx, `UPDATE student_import_issues SET resolved_at = now(), resolution = 'accepted'
		WHERE id = '`+issueID+`'`); err != nil {
		t.Fatalf("update as B: %v", err)
	}
	if got, _ := repoA.ImportIssueByID(ctx, id, issueID); !got.IsOpen() {
		t.Fatal("tenant B resolved tenant A's issue: RLS breach")
	}
	err := repoB.ExecAsTenant(ctx, `INSERT INTO student_import_issues (id, tenant_id, student_id, field, code)
		VALUES ('1d220000-0000-0000-0000-0000000006ff', '`+tenantA+`', '`+id+`', 'email', 'x')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("insert naming tenant A as B: err = %v; want 42501 (row-level security)", err)
	}
	if err := repoB.ExecAsTenant(ctx, `DELETE FROM student_import_issues WHERE id = '`+issueID+`'`); err != nil {
		t.Fatalf("delete as B: %v", err)
	}
	if _, err := repoA.ImportIssueByID(ctx, id, issueID); err != nil {
		t.Fatalf("tenant B deleted tenant A's issue: %v", err)
	}
}

// Migration 0022: only a draft goes without a CURP.
func TestOnlyADraftMayLackACURP(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	insert := func(id, status string) error {
		return repo.ExecAsTenant(ctx, `INSERT INTO students (id, tenant_id, curp, first_name, last_name, level_name, admission_status)
			VALUES ('`+id+`', '`+tenantA+`', NULL, 'Ana', 'H', 'Primaria', '`+status+`')`)
	}
	var pgErr *pgconn.PgError
	if err := insert("aaaaaaaa-2200-0000-0000-000000000071", "pending"); !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("a pending pupil without CURP: err = %v; want 23514 (check violation)", err)
	}
	if err := insert("aaaaaaaa-2200-0000-0000-000000000072", "draft"); err != nil {
		t.Fatalf("a draft without CURP must be stored: %v", err)
	}
}
