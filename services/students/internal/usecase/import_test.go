package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// --- fakes -----------------------------------------------------------------

// fakeImportRepo is an in-memory ImportRepository. ImportStudent behaves as the
// transaction does: a taken reference is dropped, never refused.
type fakeImportRepo struct {
	existing  map[string]ports.ExistingPupil // by CURP
	refsTaken map[string]bool
	imported  []ports.ImportCommand
	// interrupt runs instead of ImportStudent for a CURP, once: what another
	// writer did between the reads and the transaction.
	interrupt map[string]func() error
	seq       int
}

func newImportRepo() *fakeImportRepo {
	return &fakeImportRepo{
		existing: map[string]ports.ExistingPupil{}, refsTaken: map[string]bool{}, interrupt: map[string]func() error{},
	}
}

func (r *fakeImportRepo) StudentsByCURP(_ context.Context, curps []string) (map[string]ports.ExistingPupil, error) {
	out := map[string]ports.ExistingPupil{}
	for _, c := range curps {
		if p, ok := r.existing[c]; ok {
			out[c] = p
		}
	}
	return out, nil
}

func (r *fakeImportRepo) BankReferencesTaken(_ context.Context, refs []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, ref := range refs {
		if r.refsTaken[ref] {
			out[ref] = true
		}
	}
	return out, nil
}

func (r *fakeImportRepo) ImportStudent(_ context.Context, cmd ports.ImportCommand) (ports.ImportedPupil, error) {
	curp := cmd.Student.CURP().String()
	if race, ok := r.interrupt[curp]; ok {
		delete(r.interrupt, curp)
		return ports.ImportedPupil{}, race()
	}
	var out ports.ImportedPupil
	if ref := cmd.Student.BankReference().String(); ref != "" {
		if r.refsTaken[ref] {
			cmd.Student.ChangeBankReference("")
			out.BankReferenceDropped = true
		} else {
			r.refsTaken[ref] = true
		}
	}
	r.seq++
	out.EnrollmentNumber = fmt.Sprintf("2026-%05d", r.seq)
	cmd.Student.AssignEnrollmentNumber(out.EnrollmentNumber)
	r.existing[curp] = ports.ExistingPupil{StudentID: cmd.Student.ID(), ActiveGroupIDs: []string{cmd.Enrollment.GroupID()}}
	r.imported = append(r.imported, cmd)
	return out, nil
}

// countingCatalog counts how often config is asked about a group.
type countingCatalog struct {
	*fakeCatalog
	groupCalls map[string]int
	groupErr   error
}

func (c *countingCatalog) GroupByID(ctx context.Context, id string) (groupRef, error) {
	c.groupCalls[id]++
	if c.groupErr != nil {
		return groupRef{}, c.groupErr
	}
	return c.fakeCatalog.GroupByID(ctx, id)
}

// --- fixtures --------------------------------------------------------------

const (
	importGroupA  = "4a000000-0000-0000-0000-00000000000a"
	importGroupB  = "4a000000-0000-0000-0000-00000000000b"
	importCycle   = "c1000000-0000-0000-0000-000000000001"
	importBatch   = "b0000000-0000-0000-0000-000000000001"
	importCURP    = "HEGA050312MDFRNN08"
	importCURPTwo = "LOPB060415HDFRNN09"
)

var importNow = time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC)

type importFixture struct {
	svc     *usecase.ImportService
	repo    *fakeImportRepo
	catalog *countingCatalog
}

func newImportFixture(t *testing.T) importFixture {
	t.Helper()
	catalog := &countingCatalog{fakeCatalog: newCatalog(), groupCalls: map[string]int{}}
	catalog.groups[importGroupA] = groupRef{ID: importGroupA, CycleID: importCycle, Name: "1° A", GradeLevelID: "gl-1", Capacity: 1}
	catalog.groups[importGroupB] = groupRef{ID: importGroupB, CycleID: importCycle, Name: "1° B", GradeLevelID: "gl-1", Capacity: 25}
	repo := newImportRepo()
	ids := 0
	svc := usecase.NewImportService(repo, catalog, fakeLevels{primariaLevel: "Primaria"}, tenant,
		func() string { ids++; return fmt.Sprintf("00000000-0000-0000-0000-%012d", ids) },
		func() time.Time { return importNow })
	return importFixture{svc: svc, repo: repo, catalog: catalog}
}

func row(n int, curp string) usecase.ImportRow {
	return usecase.ImportRow{RowNumber: n, CURP: curp, FirstName: "Ana", LastName: "Hernández", MaternalLastName: "Ríos", GroupID: importGroupA}
}

func (f importFixture) run(t *testing.T, validateOnly bool, rows ...usecase.ImportRow) []usecase.ImportResult {
	t.Helper()
	results, err := f.svc.ImportStudents(context.Background(), rows, validateOnly, importBatch)
	if err != nil {
		t.Fatalf("ImportStudents: %v", err)
	}
	if len(results) != len(rows) {
		t.Fatalf("results = %d; want one per row (%d)", len(results), len(rows))
	}
	for i, r := range results {
		if r.RowNumber != rows[i].RowNumber {
			t.Fatalf("result %d is row %d; want request order (row %d)", i, r.RowNumber, rows[i].RowNumber)
		}
	}
	return results
}

// --- tests -----------------------------------------------------------------

func TestImportStoresAnEnrolledPupilWithAnImportEnrollment(t *testing.T) {
	f := newImportFixture(t)
	in := row(7, importCURP)
	in.BankReference = "0012345"
	in.Profile = domain.ProfileInput{Sex: "F", Phones: []domain.Phone{{Kind: "celular", Number: "5512345678"}}}

	got := f.run(t, false, in)[0]

	if got.Outcome != usecase.ImportImported || got.Reason != "" || len(got.Warnings) != 0 {
		t.Fatalf("result = %+v; want a clean IMPORTED", got)
	}
	if len(f.repo.imported) != 1 {
		t.Fatalf("stored %d pupils; want 1", len(f.repo.imported))
	}
	cmd := f.repo.imported[0]
	s, e := cmd.Student, cmd.Enrollment
	if got.StudentID != s.ID() || got.EnrollmentNumber != "2026-00001" {
		t.Errorf("result ids = (%q, %q); want (%q, 2026-00001)", got.StudentID, got.EnrollmentNumber, s.ID())
	}
	if s.Status() != domain.AdmissionEnrolled {
		t.Errorf("status = %v; want enrolled", s.Status())
	}
	if s.Level().ID() != primariaLevel || s.Level().Name() != "Primaria" || s.GradeGroup() != "1° A" {
		t.Errorf("placement = (%q, %q, %q); want the group's level and name", s.Level().ID(), s.Level().Name(), s.GradeGroup())
	}
	if s.MaternalLastName() != "Ríos" || s.BankReference() != "0012345" || s.Profile().Sex() != domain.SexFemale {
		t.Errorf("file = %q %q %q", s.MaternalLastName(), s.BankReference(), s.Profile().Sex())
	}
	if e.Origin() != domain.EnrollmentOriginImport || e.CycleID() != importCycle || e.GroupID() != importGroupA ||
		e.StudentID() != s.ID() || e.Status() != domain.EnrollmentStatusActive || !e.EnrolledAt().Equal(importNow) {
		t.Errorf("enrollment = %+v; want an active import enrollment dated now in the group's cycle", e)
	}
	if cmd.BatchID != importBatch {
		t.Errorf("batch = %q; want %q", cmd.BatchID, importBatch)
	}
}

// Capacity is planned in config before the import; a group whose seats are
// all taken still takes the imported pupils (the fixture's 1° A holds 1).
func TestImportIsNeverRefusedForCapacity(t *testing.T) {
	f := newImportFixture(t)
	results := f.run(t, false, row(1, importCURP), row(2, importCURPTwo))
	for _, r := range results {
		if r.Outcome != usecase.ImportImported {
			t.Fatalf("row %d = %+v; want IMPORTED", r.RowNumber, r)
		}
	}
}

func TestImportValidateOnlyWritesNothing(t *testing.T) {
	f := newImportFixture(t)
	noGroup := row(2, importCURPTwo)
	noGroup.GroupID = "" // a group the import has not created yet
	results := f.run(t, true, row(1, importCURP), noGroup)
	for _, r := range results {
		if r.Outcome != usecase.ImportImported || r.StudentID != "" || r.EnrollmentNumber != "" {
			t.Errorf("row %d = %+v; want IMPORTED without ids", r.RowNumber, r)
		}
	}
	if len(f.repo.imported) != 0 {
		t.Fatalf("validate_only stored %d pupils", len(f.repo.imported))
	}
}

func TestImportRowOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		seed    func(*fakeImportRepo)
		rows    func() []usecase.ImportRow
		want    []usecase.ImportOutcome
		reasons []string
	}{
		{
			name: "invalid CURP",
			rows: func() []usecase.ImportRow { return []usecase.ImportRow{row(1, "NOTACURP")} },
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonCURPInvalid},
		},
		{
			name: "empty CURP",
			rows: func() []usecase.ImportRow { return []usecase.ImportRow{row(1, "  ")} },
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonCURPInvalid},
		},
		{
			name: "CURP repeated in the file: the second is rejected",
			rows: func() []usecase.ImportRow {
				return []usecase.ImportRow{row(1, importCURP), row(2, strings.ToLower(importCURP))}
			},
			want:    []usecase.ImportOutcome{usecase.ImportImported, usecase.ImportRejected},
			reasons: []string{"", usecase.ReasonCURPDuplicated},
		},
		{
			name: "CURP already in the school",
			seed: func(r *fakeImportRepo) {
				r.existing[importCURP] = ports.ExistingPupil{StudentID: "old-1", ActiveGroupIDs: []string{importGroupB}}
			},
			rows: func() []usecase.ImportRow { return []usecase.ImportRow{row(1, importCURP)} },
			want: []usecase.ImportOutcome{usecase.ImportAlreadyExists}, reasons: []string{""},
		},
		{
			name: "CURP already placed in this very group",
			seed: func(r *fakeImportRepo) {
				r.existing[importCURP] = ports.ExistingPupil{StudentID: "old-1", ActiveGroupIDs: []string{importGroupA}}
			},
			rows: func() []usecase.ImportRow { return []usecase.ImportRow{row(1, importCURP)} },
			want: []usecase.ImportOutcome{usecase.ImportAlreadyImported}, reasons: []string{""},
		},
		{
			name: "missing first name",
			rows: func() []usecase.ImportRow {
				r := row(1, importCURP)
				r.FirstName = " "
				return []usecase.ImportRow{r}
			},
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonNameRequired},
		},
		{
			name: "missing paternal surname",
			rows: func() []usecase.ImportRow {
				r := row(1, importCURP)
				r.LastName = ""
				return []usecase.ImportRow{r}
			},
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonNameRequired},
		},
		{
			name: "unknown group",
			rows: func() []usecase.ImportRow {
				r := row(1, importCURP)
				r.GroupID = "4a000000-0000-0000-0000-0000000000ff"
				return []usecase.ImportRow{r}
			},
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonGroupNotFound},
		},
		{
			name: "group id that is not an id",
			rows: func() []usecase.ImportRow {
				r := row(1, importCURP)
				r.GroupID = "1° A"
				return []usecase.ImportRow{r}
			},
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonGroupNotFound},
		},
		{
			name: "no group on a commit",
			rows: func() []usecase.ImportRow {
				r := row(1, importCURP)
				r.GroupID = ""
				return []usecase.ImportRow{r}
			},
			want: []usecase.ImportOutcome{usecase.ImportRejected}, reasons: []string{usecase.ReasonGroupNotFound},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newImportFixture(t)
			if tc.seed != nil {
				tc.seed(f.repo)
			}
			results := f.run(t, false, tc.rows()...)
			for i, r := range results {
				if r.Outcome != tc.want[i] || r.Reason != tc.reasons[i] {
					t.Errorf("row %d = (%v, %q); want (%v, %q)", r.RowNumber, r.Outcome, r.Reason, tc.want[i], tc.reasons[i])
				}
			}
			for i, r := range results {
				switch tc.want[i] {
				case usecase.ImportAlreadyExists, usecase.ImportAlreadyImported:
					if r.StudentID != "old-1" {
						t.Errorf("row %d student id = %q; want the existing pupil", r.RowNumber, r.StudentID)
					}
				case usecase.ImportRejected:
					if r.StudentID != "" {
						t.Errorf("rejected row %d carries a student id %q", r.RowNumber, r.StudentID)
					}
				}
			}
			stored := 0
			for _, w := range tc.want {
				if w == usecase.ImportImported {
					stored++
				}
			}
			if len(f.repo.imported) != stored {
				t.Errorf("stored %d pupils; want %d", len(f.repo.imported), stored)
			}
		})
	}
}

func TestImportBankReferenceWarnings(t *testing.T) {
	t.Run("invalid is stored without and warned", func(t *testing.T) {
		f := newImportFixture(t)
		r := row(1, importCURP)
		r.BankReference = "S/R 123"
		got := f.run(t, false, r)[0]
		if got.Outcome != usecase.ImportImported || !reflect.DeepEqual(got.Warnings, []string{usecase.WarnBankReferenceInvalid}) {
			t.Fatalf("result = %+v; want IMPORTED with bank_reference_invalid", got)
		}
		if !f.repo.imported[0].Student.BankReference().IsZero() {
			t.Fatalf("an invalid reference was stored: %q", f.repo.imported[0].Student.BankReference())
		}
	})
	t.Run("taken is imported without it", func(t *testing.T) {
		f := newImportFixture(t)
		f.repo.refsTaken["555"] = true
		r := row(1, importCURP)
		r.BankReference = "555"
		got := f.run(t, false, r)[0]
		if got.Outcome != usecase.ImportImported || !reflect.DeepEqual(got.Warnings, []string{usecase.WarnBankReferenceTaken}) {
			t.Fatalf("result = %+v; want IMPORTED with bank_reference_taken", got)
		}
	})
	t.Run("validating, taken and repeated in the file both warn", func(t *testing.T) {
		f := newImportFixture(t)
		f.repo.refsTaken["555"] = true
		first, second, third := row(1, importCURP), row(2, importCURPTwo), row(3, "MAPC070101HDFRNN01")
		first.BankReference, second.BankReference, third.BankReference = "555", "777", "777"
		results := f.run(t, true, first, second, third)
		want := [][]string{{usecase.WarnBankReferenceTaken}, nil, {usecase.WarnBankReferenceTaken}}
		for i, r := range results {
			if !reflect.DeepEqual(r.Warnings, want[i]) {
				t.Errorf("row %d warnings = %v; want %v", r.RowNumber, r.Warnings, want[i])
			}
		}
	})
	t.Run("taken between the check and the insert is retried without it", func(t *testing.T) {
		f := newImportFixture(t)
		f.repo.interrupt[importCURP] = func() error { return fmt.Errorf("%w: race", domain.ErrBankReferenceTaken) }
		r := row(1, importCURP)
		r.BankReference = "999"
		got := f.run(t, false, r)[0]
		if got.Outcome != usecase.ImportImported || !reflect.DeepEqual(got.Warnings, []string{usecase.WarnBankReferenceTaken}) {
			t.Fatalf("result = %+v; want IMPORTED with bank_reference_taken", got)
		}
		if !f.repo.imported[0].Student.BankReference().IsZero() {
			t.Fatalf("the retry kept the taken reference")
		}
	})
}

func TestImportReportsTheLenientProfileWarnings(t *testing.T) {
	f := newImportFixture(t)
	r := row(1, importCURP)
	r.Profile = domain.ProfileInput{
		Email:   "nope@",
		Phones:  []domain.Phone{{Kind: "casa", Number: "123"}},
		Address: domain.Address{Street: "Juárez 1", State: "Narnia"},
	}
	got := f.run(t, false, r)[0]
	want := []string{domain.WarnEmailInvalid, domain.WarnPhoneInvalid, domain.WarnStateUnknown}
	if got.Outcome != usecase.ImportImported || !reflect.DeepEqual(got.Warnings, want) {
		t.Fatalf("result = %+v; want IMPORTED with %v", got, want)
	}
	if a := f.repo.imported[0].Student.Profile().Address(); a.Street != "Juárez 1" || a.State != "" {
		t.Fatalf("address = %+v; want the street kept and the state dropped", a)
	}
}

func TestImportAsksConfigOncePerGroup(t *testing.T) {
	f := newImportFixture(t)
	other := row(3, "MAPC070101HDFRNN01")
	other.GroupID = importGroupB
	f.run(t, false, row(1, importCURP), row(2, importCURPTwo), other)
	if f.catalog.groupCalls[importGroupA] != 1 || f.catalog.groupCalls[importGroupB] != 1 {
		t.Fatalf("group lookups = %v; want one per distinct group", f.catalog.groupCalls)
	}
}

func TestImportACURPTakenMeanwhileIsAlreadyExists(t *testing.T) {
	f := newImportFixture(t)
	f.repo.interrupt[importCURP] = func() error {
		f.repo.existing[importCURP] = ports.ExistingPupil{StudentID: "racer"}
		return fmt.Errorf("%w: race", domain.ErrCURPTaken)
	}
	got := f.run(t, false, row(1, importCURP))[0]
	if got.Outcome != usecase.ImportAlreadyExists || got.StudentID != "racer" {
		t.Fatalf("result = %+v; want ALREADY_EXISTS naming the pupil that won", got)
	}
}

func TestImportRefusesTheWholeRequest(t *testing.T) {
	t.Run("more than 1,000 rows", func(t *testing.T) {
		f := newImportFixture(t)
		rows := make([]usecase.ImportRow, usecase.MaxImportRows+1)
		_, err := f.svc.ImportStudents(context.Background(), rows, true, "")
		if glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Fatalf("err = %v; want KindInvalid", err)
		}
	})
	t.Run("a commit without a batch id", func(t *testing.T) {
		f := newImportFixture(t)
		_, err := f.svc.ImportStudents(context.Background(), []usecase.ImportRow{row(1, importCURP)}, false, "batch")
		if glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Fatalf("err = %v; want KindInvalid", err)
		}
	})
	t.Run("config is down", func(t *testing.T) {
		f := newImportFixture(t)
		f.catalog.groupErr = errors.New("connection refused")
		_, err := f.svc.ImportStudents(context.Background(), []usecase.ImportRow{row(1, importCURP)}, false, importBatch)
		if err == nil || len(f.repo.imported) != 0 {
			t.Fatalf("err = %v, stored %d; want the request refused and nothing stored", err, len(f.repo.imported))
		}
	})
}
