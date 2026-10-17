//go:build integration

package postgres_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// tenantBirthdays is this file's own tenant: the container is shared, and RLS
// keeps every other test's pupils out of these lists.
const tenantBirthdays = "b1d0b1d0-0000-0000-0000-000000000001"

// bornOn stores an applicant born on birth ("" = no date), enrolled or not.
func bornOn(t *testing.T, repo interface {
	Create(context.Context, *domain.Student, int) error
	Update(context.Context, *domain.Student) error
}, id, curp, birth string, enrolled bool) {
	t.Helper()
	s := applicant(t, id, tenantBirthdays, curp, "1° A")
	profile, err := domain.NewProfile(domain.ProfileInput{BirthDate: birth}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	c, _ := domain.NewCURP(curp)
	if err := s.UpdateDetails(s.FirstName(), s.LastName(), "", c, profile); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	ctx := context.Background()
	if err := repo.Create(ctx, s, testNumberYear); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
	if !enrolled {
		return
	}
	if _, err := s.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if _, _, err := s.Enroll("c0000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if err := repo.Update(ctx, s); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func ids(students []*domain.Student) []string {
	out := make([]string, 0, len(students))
	for _, s := range students {
		out = append(out, s.ID())
	}
	sort.Strings(out)
	return out
}

// T11: the daily greeting asks "who was born on this day, of any year". The
// birth date written at intake is what it reads.
func TestListByBirthday(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantBirthdays)
	const (
		oct1      = "b1d00000-0000-0000-0000-000000000001"
		oct1Other = "b1d00000-0000-0000-0000-000000000002"
		oct2      = "b1d00000-0000-0000-0000-000000000003"
		feb28     = "b1d00000-0000-0000-0000-000000000004"
		feb29     = "b1d00000-0000-0000-0000-000000000005"
		noDate    = "b1d00000-0000-0000-0000-000000000006"
		pending   = "b1d00000-0000-0000-0000-000000000007"
	)
	bornOn(t, repo, oct1, "BIRA161001MDFRNN01", "2016-10-01", true)
	bornOn(t, repo, oct1Other, "BIRA191001MDFRNN02", "2019-10-01", true)
	bornOn(t, repo, oct2, "BIRA161002MDFRNN03", "2016-10-02", true)
	bornOn(t, repo, feb28, "BIRA170228MDFRNN04", "2017-02-28", true)
	bornOn(t, repo, feb29, "BIRA160229MDFRNN05", "2016-02-29", true)
	bornOn(t, repo, noDate, "BIRA100101MDFRNN06", "", true)
	bornOn(t, repo, pending, "BIRA161001MDFRNN07", "2016-10-01", false)

	list := func(f ports.ListFilter) []string {
		t.Helper()
		got, _, err := repo.List(ctx, f)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		return ids(got)
	}
	enrolled := domain.AdmissionEnrolled

	if got := list(ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 10, Day: 1}}); !equal(got, []string{oct1, oct1Other}) {
		t.Errorf("october 1 enrolled = %v", got)
	}
	// Without the status filter the pending applicant is there too: who counts
	// is the caller's choice, the day is the filter's.
	if got := list(ports.ListFilter{Birthday: &ports.BirthdayFilter{Month: 10, Day: 1}}); len(got) != 3 {
		t.Errorf("october 1 any status = %v", got)
	}
	if got := list(ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 2, Day: 28}}); !equal(got, []string{feb28}) {
		t.Errorf("february 28 = %v", got)
	}
	if got := list(ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 2, Day: 28, IncludeFeb29: true}}); !equal(got, []string{feb28, feb29}) {
		t.Errorf("february 28 + leap day = %v", got)
	}
	if got := list(ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 2, Day: 29}}); !equal(got, []string{feb29}) {
		t.Errorf("february 29 = %v", got)
	}
	// Paginated like every list: a page of one still finds both, in two pages.
	first, next, err := repo.List(ctx, ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 10, Day: 1}, PageSize: 1})
	if err != nil || len(first) != 1 || next == "" {
		t.Fatalf("page 1 = %d %q %v", len(first), next, err)
	}
	second, last, err := repo.List(ctx, ports.ListFilter{Status: enrolled, Birthday: &ports.BirthdayFilter{Month: 10, Day: 1}, PageSize: 1, PageToken: next})
	if err != nil || len(second) != 1 || last != "" || second[0].ID() == first[0].ID() {
		t.Errorf("page 2 = %d %q %v", len(second), last, err)
	}
	// RLS: another tenant asking for the same day sees nobody of ours.
	if got, _, _ := appRepo(t, tenantB).List(ctx, ports.ListFilter{Birthday: &ports.BirthdayFilter{Month: 10, Day: 1}}); containsAny(ids(got), oct1, oct1Other) {
		t.Errorf("tenant B sees tenant birthdays: %v", ids(got))
	}
}

// The birth date captured at intake (CreateApplicant) is stored on insert.
func TestCreateStoresTheBirthDate(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantBirthdays)
	const id = "b1d00000-0000-0000-0000-0000000000a1"
	c, _ := domain.NewCURP("BIRA150612MDFRNN08")
	profile, err := domain.NewProfile(domain.ProfileInput{BirthDate: "2015-06-12"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID: id, TenantID: tenantBirthdays, CURP: c, FirstName: "Lu", LastName: "R",
		Level: domain.RestoreLevel(primariaLevelID, "Primaria"), Profile: profile,
	})
	if err != nil {
		t.Fatalf("NewApplicant: %v", err)
	}
	if err := repo.Create(ctx, s, testNumberYear); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Profile().BirthDate().Format("2006-01-02") != "2015-06-12" {
		t.Errorf("birth date = %v", got.Profile().BirthDate())
	}
}

func equal(a, b []string) bool {
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsAny(list []string, want ...string) bool {
	for _, l := range list {
		for _, w := range want {
			if l == w {
				return true
			}
		}
	}
	return false
}

// Owner decision D4 (plan T11): the greeting reaches every pupil with an
// active enrollment, whatever the admission says — "en trámite" pupils who
// already attend included; a withdrawn enrollment, or none, does not count.
func TestListByActiveEnrollment(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantBirthdays)
	const (
		attendingPending = "b1d00000-0000-0000-0000-000000000011"
		withdrawn        = "b1d00000-0000-0000-0000-000000000012"
		neverPlaced      = "b1d00000-0000-0000-0000-000000000013"
		cycle            = "c0000000-0000-0000-0000-000000000001"
		group            = "6b000000-0000-4000-8000-000000000011"
	)
	bornOn(t, repo, attendingPending, "BIRA151105MDFRNN11", "2015-11-05", false)
	bornOn(t, repo, withdrawn, "BIRA151105MDFRNN12", "2015-11-05", false)
	bornOn(t, repo, neverPlaced, "BIRA151105MDFRNN13", "2015-11-05", false)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{attendingPending, withdrawn} {
		e, err := domain.NewEnrollment("e1d00000-0000-0000-0000-00000000001"+string(rune('1'+i)), tenantBirthdays, id, cycle, group, "", domain.EnrollmentStatusActive, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := createRoomy(ctx, repo, e); err != nil {
			t.Fatalf("enrol %s: %v", id, err)
		}
	}
	if err := repo.SetEnrollmentStatus(ctx, "e1d00000-0000-0000-0000-000000000012", domain.EnrollmentStatusWithdrawn); err != nil {
		t.Fatal(err)
	}

	got, _, err := repo.List(ctx, ports.ListFilter{ActiveEnrollment: true, Birthday: &ports.BirthdayFilter{Month: 11, Day: 5}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !equal(ids(got), []string{attendingPending}) {
		t.Errorf("november 5 with an active enrollment = %v, want only the attending pupil", ids(got))
	}
}
