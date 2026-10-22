//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// twiceGroup is this test's own group (and cycle): other suites count theirs.
const (
	twiceGroup = "40000000-0000-0000-0000-0000000022b0"
	twiceCycle = "c0000000-0000-0000-0000-0000000022b0"
	twiceGrade = "9a000000-0000-0000-0000-0000000022b0"
)

// oneGroupCatalog is config as the import sees it: one group of Primaria.
type oneGroupCatalog struct{}

func (oneGroupCatalog) GroupByID(_ context.Context, id string) (ports.GroupRef, error) {
	if id != twiceGroup {
		return ports.GroupRef{}, glerrors.ErrNotFound
	}
	return ports.GroupRef{ID: twiceGroup, CycleID: twiceCycle, Name: "6° Z", GradeLevelID: twiceGrade, Capacity: 30}, nil
}
func (oneGroupCatalog) TermByID(context.Context, string) (ports.TermRef, error) {
	return ports.TermRef{}, glerrors.ErrNotFound
}
func (oneGroupCatalog) CycleByID(_ context.Context, id string) (ports.CycleRef, error) {
	return ports.CycleRef{ID: id, StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}, nil
}
func (oneGroupCatalog) GroupsOfGrade(context.Context, string) ([]ports.GroupSeats, error) {
	return nil, nil
}
func (oneGroupCatalog) LevelOfGrade(context.Context, string) (string, error) {
	return primariaLevelID, nil
}
func (oneGroupCatalog) LevelByID(_ context.Context, id string) (domain.Level, error) {
	return domain.NewLevel(id, "Primaria")
}

// The same spreadsheet uploaded twice stores its drafts once: the second
// upload reports them ALREADY_IMPORTED (a draft has no CURP to match by).
func TestImportingTheSameFileTwiceCreatesNoNewDrafts(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	ids := 0
	svc := usecase.NewImportService(repo, oneGroupCatalog{}, oneGroupCatalog{}, tenantA,
		func() string { ids++; return fmt.Sprintf("22b00000-0000-0000-0000-%012d", ids) },
		func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) })
	rows := []usecase.ImportRow{
		{RowNumber: 2, FirstName: "Iker", LastName: "Twice", GroupID: twiceGroup, BankReference: "22301",
			Action: usecase.ImportActionDraft, Profile: domain.ProfileInput{BirthDate: "2015-03-12"}},
		{RowNumber: 3, FirstName: "Iker", LastName: "Twice", GroupID: twiceGroup, BankReference: "22302",
			Action: usecase.ImportActionDraft, Profile: domain.ProfileInput{BirthDate: "2016-01-01"}},
	}
	first, err := svc.ImportStudents(ctx, rows, false, "ba000000-0000-0000-0000-0000000022b1")
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	second, err := svc.ImportStudents(ctx, rows, false, "ba000000-0000-0000-0000-0000000022b2")
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	for i := range rows {
		if first[i].Outcome != usecase.ImportImported || !first[i].Draft {
			t.Fatalf("first upload row %d = %+v; want an IMPORTED draft", rows[i].RowNumber, first[i])
		}
		if second[i].Outcome != usecase.ImportAlreadyImported || second[i].StudentID != first[i].StudentID {
			t.Fatalf("second upload row %d = %+v; want ALREADY_IMPORTED of %s", rows[i].RowNumber, second[i], first[i].StudentID)
		}
	}
	n, err := repo.CountAsTenant(ctx, "SELECT count(*) FROM students WHERE last_name = 'Twice' AND admission_status = 'draft'")
	if err != nil || n != 2 {
		t.Fatalf("drafts = %d (err %v); want 2 after two uploads", n, err)
	}
}

// Migration 0022: an issue cannot name a pupil of another tenant, even for a
// writer that bypasses RLS — the FK is (tenant_id, student_id).
func TestAnIssueCannotPointAtAnotherTenantsPupil(t *testing.T) {
	const id = "aaaaaaaa-2200-0000-0000-0000000000b9"
	seedStudent(t, tenantA, id, "IIAF100312MDFRNN06")
	err := superDB.Exec(`INSERT INTO student_import_issues (id, tenant_id, student_id, field, code)
		VALUES ('1d220000-0000-0000-0000-0000000000b9', ?, ?, 'email', 'x')`, tenantB, id).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("cross-tenant issue: err = %v; want 23503 (foreign key violation)", err)
	}
}

// import a draft → the office saves its CURP → the same row is uploaded
// again: it is that pupil (ALREADY_IMPORTED), not a new draft beside them.
func TestReuploadingAPromotedDraftCreatesNoNewPupil(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	ids := 100
	svc := usecase.NewImportService(repo, oneGroupCatalog{}, oneGroupCatalog{}, tenantA,
		func() string { ids++; return fmt.Sprintf("22b00000-0000-0000-0000-%012d", ids) },
		func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) })
	row := []usecase.ImportRow{{RowNumber: 2, FirstName: "Ona", LastName: "Promoted", GroupID: twiceGroup,
		Action: usecase.ImportActionDraft, Profile: domain.ProfileInput{BirthDate: "2015-05-05"}}}
	first, err := svc.ImportStudents(ctx, row, false, "ba000000-0000-0000-0000-0000000022c1")
	if err != nil || !first[0].Draft {
		t.Fatalf("first upload = %+v, %v; want a draft", first, err)
	}
	s, err := repo.FindByID(ctx, first[0].StudentID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if err := s.UpdateDetails(s.FirstName(), s.LastName(), "", mustCURP(t, "IIAG100312MDFRNN07"), s.Profile()); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := repo.UpdateDetails(ctx, s, ports.DetailsCorrection{Fields: []domain.IssueField{domain.FieldCURP}}); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}

	again, err := svc.ImportStudents(ctx, row, false, "ba000000-0000-0000-0000-0000000022c2")
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if again[0].Outcome != usecase.ImportAlreadyImported || again[0].StudentID != first[0].StudentID || again[0].Draft {
		t.Fatalf("second upload = %+v; want ALREADY_IMPORTED of the promoted pupil %s", again[0], first[0].StudentID)
	}
	if n, _ := repo.CountAsTenant(ctx, "SELECT count(*) FROM students WHERE last_name = 'Promoted'"); n != 1 {
		t.Fatalf("pupils = %d; want 1", n)
	}
}
