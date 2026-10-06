//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

const (
	importBatchID = "ba000000-0000-0000-0000-000000000001"
	// importGroup is these tests' own group: other suites count groupOne/Two.
	importGroup = "40000000-0000-0000-0000-0000000001a0"
)

var matriculaShape = regexp.MustCompile(`^[0-9]{4}-[0-9]{5}$`)

// importCommand builds one imported pupil as the use case does.
func importCommand(t *testing.T, tenant, id, enrollmentID, curp, ref string) ports.ImportCommand {
	t.Helper()
	bank, err := domain.NewBankReference(ref)
	if err != nil {
		t.Fatalf("bank reference: %v", err)
	}
	profile, _ := domain.NewProfileLenient(domain.ProfileInput{
		Sex:     "F",
		Phones:  []domain.Phone{{Kind: "celular", Number: "5512345678"}},
		Address: domain.Address{Street: "Av. Juárez 10", PostalCode: "58000", State: "MICHOACAN DE OCAMPO"},
	}, time.Now())
	student, err := domain.NewImported(domain.NewApplicantParams{
		ID: id, TenantID: tenant, CURP: mustCURP(t, curp), FirstName: "Lucía", LastName: "de la Cruz",
		MaternalLastName: "Hernández", Level: domain.RestoreLevel(primariaLevelID, "Primaria"),
		Profile: profile, BankReference: bank,
	})
	if err != nil {
		t.Fatalf("NewImported: %v", err)
	}
	student.PlaceIn("3° B")
	e, err := domain.NewEnrollment(enrollmentID, tenant, id, cycleUUID, importGroup, "", domain.EnrollmentStatusActive, time.Now())
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	return ports.ImportCommand{Student: student, Enrollment: e.WithOrigin(domain.EnrollmentOriginImport), BatchID: importBatchID}
}

func TestImportStudentStoresAnEnrolledPupilInOneTransaction(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-1a00-0000-0000-000000000001"
	cmd := importCommand(t, tenantA, id, "e1a00000-0000-0000-0000-000000000001", "IAAA100312MDFRNN01", "4401")

	got, err := repo.ImportStudent(ctx, cmd)
	if err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	if !matriculaShape.MatchString(got.EnrollmentNumber) || got.BankReferenceDropped {
		t.Fatalf("result = %+v; want a matrícula and the reference kept", got)
	}

	s, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if s.Status() != domain.AdmissionEnrolled || s.GradeGroup() != "3° B" || s.Level().ID() != primariaLevelID {
		t.Errorf("pupil = (%v, %q, %q); want enrolled in 3° B of Primaria", s.Status(), s.GradeGroup(), s.Level().ID())
	}
	if s.EnrollmentNumber() != got.EnrollmentNumber || s.BankReference() != "4401" || s.MaternalLastName() != "Hernández" {
		t.Errorf("file = (%q, %q, %q)", s.EnrollmentNumber(), s.BankReference(), s.MaternalLastName())
	}
	if p := s.Profile(); p.Sex() != domain.SexFemale || len(p.Phones()) != 1 || p.Address().State != "Michoacán" || p.Address().PostalCode != "58000" {
		t.Errorf("profile not stored whole: %+v %+v", p.Phones(), p.Address())
	}

	counts := map[string]string{
		"the import batch": "SELECT count(*) FROM students WHERE id = ? AND import_batch_id = '" + importBatchID + "'",
		"an active import enrollment dated now": `SELECT count(*) FROM enrollments WHERE student_id = ?
		      AND origin = 'import' AND status = 'active' AND enrolled_at > now() - interval '1 minute'`,
	}
	for what, q := range counts {
		if n, err := repo.CountAsTenant(ctx, q, id); err != nil || n != 1 {
			t.Errorf("%s: %d rows (err %v); want 1", what, n, err)
		}
	}
	// students keeps no outbox: an import publishes nothing because the
	// service holds no publisher. If an outbox ever appears, this test must
	// grow to prove the import writes no row to it.
	var outboxes int64
	if err := superDB.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name ILIKE '%outbox%'`).Scan(&outboxes).Error; err != nil || outboxes != 0 {
		t.Errorf("outbox tables = %d (err %v); assert the import leaves no event rows in it", outboxes, err)
	}
}

func TestImportStudentDropsATakenBankReference(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	holder := withBankReference(t, applicant(t, "aaaaaaaa-1a00-0000-0000-000000000010", tenantA, "IAAB100312MDFRNN02", "1° A"), "8800")
	if err := repo.Create(ctx, holder); err != nil {
		t.Fatalf("seed holder: %v", err)
	}
	const id = "aaaaaaaa-1a00-0000-0000-000000000011"
	got, err := repo.ImportStudent(ctx, importCommand(t, tenantA, id, "e1a00000-0000-0000-0000-000000000011", "IAAC100312MDFRNN03", "8800"))
	if err != nil {
		t.Fatalf("ImportStudent with a taken reference must still import: %v", err)
	}
	if !got.BankReferenceDropped {
		t.Fatalf("result = %+v; want the reference reported dropped", got)
	}
	s, err := repo.FindByID(ctx, id)
	if err != nil || !s.BankReference().IsZero() {
		t.Fatalf("imported pupil reference = %q (err %v); want none", s.BankReference(), err)
	}
}

func TestImportStudentWithATakenCURPWritesNothing(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	seedStudent(t, tenantA, "aaaaaaaa-1a00-0000-0000-000000000020", "IAAD100312MDFRNN04")
	_, err := repo.ImportStudent(ctx, importCommand(t, tenantA, "aaaaaaaa-1a00-0000-0000-000000000021",
		"e1a00000-0000-0000-0000-000000000021", "IAAD100312MDFRNN04", ""))
	if !errors.Is(err, domain.ErrCURPTaken) {
		t.Fatalf("err = %v; want ErrCURPTaken", err)
	}
	if n, _ := repo.CountAsTenant(ctx, "SELECT count(*) FROM enrollments WHERE id = 'e1a00000-0000-0000-0000-000000000021'"); n != 0 {
		t.Fatalf("the enrollment of a refused pupil was stored")
	}
}

func TestImportReadsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	repoA, repoB := appRepo(t, tenantA), appRepo(t, tenantB)
	const id = "aaaaaaaa-1a00-0000-0000-000000000030"
	if _, err := repoA.ImportStudent(ctx, importCommand(t, tenantA, id, "e1a00000-0000-0000-0000-000000000030", "IAAE100312MDFRNN05", "7300")); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}

	found, err := repoA.StudentsByCURP(ctx, []string{"IAAE100312MDFRNN05", "XAXX010101HDFRNN09"})
	if err != nil {
		t.Fatalf("StudentsByCURP: %v", err)
	}
	pupil, ok := found["IAAE100312MDFRNN05"]
	if !ok || pupil.StudentID != id || len(pupil.ActiveGroupIDs) != 1 || pupil.ActiveGroupIDs[0] != importGroup || len(found) != 1 {
		t.Fatalf("StudentsByCURP = %+v; want the pupil with its active group only", found)
	}
	taken, err := repoA.BankReferencesTaken(ctx, []string{"7300", "7301"})
	if err != nil || !taken["7300"] || taken["7301"] {
		t.Fatalf("BankReferencesTaken = %v (err %v); want only 7300", taken, err)
	}

	if found, _ := repoB.StudentsByCURP(ctx, []string{"IAAE100312MDFRNN05"}); len(found) != 0 {
		t.Fatalf("tenant B sees tenant A's pupil by CURP (RLS breach): %+v", found)
	}
	if taken, _ := repoB.BankReferencesTaken(ctx, []string{"7300"}); taken["7300"] {
		t.Fatal("tenant B sees tenant A's bank reference (RLS breach)")
	}
}
