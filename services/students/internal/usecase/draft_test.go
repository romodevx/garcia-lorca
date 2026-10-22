package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	draftCURP  = "HEGA050312MDFRNN08"
	actorUser  = "a0000000-0000-0000-0000-0000000000a1"
	draftGroup = "4a000000-0000-0000-0000-00000000000d"
)

// seedDraft stores a pupil the import brought without a CURP.
func seedDraft(t *testing.T, repo *fakeRepo, id string) {
	t.Helper()
	repo.byID[id] = domain.Rehydrate(domain.NewApplicantParams{
		ID: id, TenantID: tenant, FirstName: "Ana", LastName: "Hernández",
		Level: domain.RestoreLevel("e1000000-0000-0000-0000-000000000001", "Primaria"),
	}, domain.AdmissionDraft)
}

// --- a draft is not placed nor linked --------------------------------------

func TestADraftIsNeverPlaced(t *testing.T) {
	cases := map[string]func(*testing.T, *usecase.EnrollmentService, *fakeRepo, *fakeEnrollRepo, *fakeCatalog) error{
		"enroll into a group": func(t *testing.T, svc *usecase.EnrollmentService, students *fakeRepo, _ *fakeEnrollRepo, catalog *fakeCatalog) error {
			seedDraft(t, students, "stu-1")
			catalog.groups["grp-1"] = groupRef{ID: "grp-1", CycleID: "cyc-99", Capacity: 30}
			_, err := svc.Enroll(context.Background(), "stu-1", "grp-1", "", time.Time{})
			return err
		},
		"enroll into a grade": func(t *testing.T, svc *usecase.EnrollmentService, students *fakeRepo, _ *fakeEnrollRepo, catalog *fakeCatalog) error {
			seedDraft(t, students, "stu-1")
			catalog.gradeGroups["gl-1"] = []ports.GroupSeats{{ID: "grp-1", CycleID: "cyc-99", Capacity: 30}}
			_, err := svc.EnrollIntoGrade(context.Background(), "stu-1", "gl-1", time.Time{})
			return err
		},
		"transfer": func(t *testing.T, svc *usecase.EnrollmentService, students *fakeRepo, _ *fakeEnrollRepo, catalog *fakeCatalog) error {
			e := enrolledStudent(t, svc, students, catalog)
			seedDraft(t, students, "stu-1") // a stale row: the pupil is a draft now
			catalog.groups["grp-2"] = groupRef{ID: "grp-2", CycleID: "cyc-99", Capacity: 30}
			_, err := svc.Transfer(context.Background(), e.ID(), "grp-2", "rebalance")
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			svc, students, enrolls, catalog, _ := newEnrollService(t)
			before := len(enrolls.createSeen)
			if err := run(t, svc, students, enrolls, catalog); !errors.Is(err, domain.ErrDraftNotPlaceable) {
				t.Fatalf("err = %v; want ErrDraftNotPlaceable", err)
			}
			if name != "transfer" && len(enrolls.createSeen) != before {
				t.Fatal("a draft was given an enrollment")
			}
		})
	}
}

func TestADraftIsNeverLinkedToAFamilyAccount(t *testing.T) {
	t.Run("link", func(t *testing.T) {
		svc, students, guardians, users := newGuardianService(t)
		seedDraft(t, students, "stu-1")
		seedGuardianUser(users, "usr-1", "guardian")
		if _, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother"); !errors.Is(err, domain.ErrDraftNotPlaceable) {
			t.Fatalf("err = %v; want ErrDraftNotPlaceable", err)
		}
		if len(guardians.links) != 0 {
			t.Fatal("a draft was linked")
		}
	})
	t.Run("grant a contact the app", func(t *testing.T) {
		svc, students, _, users := newGuardianService(t)
		seedStudent(t, students, "stu-1")
		seedGuardianUser(users, "user-1", "guardian")
		contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")
		seedDraft(t, students, "stu-1")
		if _, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1"); !errors.Is(err, domain.ErrDraftNotPlaceable) {
			t.Fatalf("err = %v; want ErrDraftNotPlaceable", err)
		}
	})
}

// --- UpdateDetails: corrections close issues, a CURP promotes a draft -------

func detailsOf(id, curp string) usecase.UpdateDetailsInput {
	return usecase.UpdateDetailsInput{ID: id, FirstName: "Ana", LastName: "Hernández", CURP: curp, ActorUserID: actorUser}
}

func TestUpdateDetailsClosesTheIssuesOfTheFieldsItChanged(t *testing.T) {
	svc, repo, _ := newService(t)
	s := seedApplicant(t, svc)
	s.AssignOpenIssues(3)
	repo.correction = ports.CorrectionResult{Corrected: 2}

	in := detailsOf(s.ID(), s.CURP().String())
	in.FirstName, in.LastName, in.MaternalLastName = "Ada", s.LastName(), s.MaternalLastName()
	in.Profile = domain.ProfileInput{Phones: []domain.Phone{{Kind: "celular", Number: "5512345678"}}}
	got, err := svc.UpdateDetails(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	want := ports.DetailsCorrection{Fields: []domain.IssueField{domain.FieldFirstName, domain.FieldPhoneMobile}, By: actorUser}
	if !reflect.DeepEqual(repo.corrections, []ports.DetailsCorrection{want}) {
		t.Fatalf("corrections = %+v; want %+v", repo.corrections, want)
	}
	if got.Student.OpenIssues() != 1 || got.PromotedFromDraft {
		t.Fatalf("result = open %d promoted %v; want 1 open, not promoted", got.Student.OpenIssues(), got.PromotedFromDraft)
	}
}

func TestUpdateDetailsPromotesADraftGivenACURP(t *testing.T) {
	svc, repo, _ := newService(t)
	seedDraft(t, repo, "stu-d")
	repo.correction = ports.CorrectionResult{Corrected: 1, IntendedGroupID: draftGroup}

	got, err := svc.UpdateDetails(context.Background(), detailsOf("stu-d", draftCURP))
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if got.Student.Status() != domain.AdmissionPending || !got.PromotedFromDraft || got.IntendedGroupID != draftGroup {
		t.Fatalf("result = %v promoted=%v group=%q; want pending, promoted, %s",
			got.Student.Status(), got.PromotedFromDraft, got.IntendedGroupID, draftGroup)
	}
	if fields := repo.corrections[0].Fields; len(fields) != 1 || fields[0] != domain.FieldCURP {
		t.Fatalf("fields = %v; want [curp]", fields)
	}
}

func TestUpdateDetailsCURPRules(t *testing.T) {
	cases := []struct {
		name     string
		seed     func(*testing.T, *fakeRepo, *usecase.StudentService) string
		curp     string
		actor    string
		wantKind glerrors.Kind
		status   domain.AdmissionStatus
	}{
		{"a draft is edited without a CURP and stays a draft",
			func(t *testing.T, r *fakeRepo, _ *usecase.StudentService) string {
				seedDraft(t, r, "stu-d")
				return "stu-d"
			},
			"", actorUser, glerrors.KindUnknown, domain.AdmissionDraft},
		{"a pupil that is not a draft still needs one",
			func(t *testing.T, _ *fakeRepo, svc *usecase.StudentService) string { return seedApplicant(t, svc).ID() },
			"", actorUser, glerrors.KindInvalid, domain.AdmissionPending},
		{"a malformed CURP is refused",
			func(t *testing.T, r *fakeRepo, _ *usecase.StudentService) string {
				seedDraft(t, r, "stu-d")
				return "stu-d"
			},
			"NOTACURP", actorUser, glerrors.KindInvalid, domain.AdmissionDraft},
		{"an actor that is not a user id is refused",
			func(t *testing.T, r *fakeRepo, _ *usecase.StudentService) string {
				seedDraft(t, r, "stu-d")
				return "stu-d"
			},
			draftCURP, "someone", glerrors.KindInvalid, domain.AdmissionDraft},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newService(t)
			id := tc.seed(t, repo, svc)
			in := detailsOf(id, tc.curp)
			in.ActorUserID = tc.actor
			_, err := svc.UpdateDetails(context.Background(), in)
			if glerrors.KindOf(err) != tc.wantKind {
				t.Fatalf("err = %v; want kind %v", err, tc.wantKind)
			}
			if got := repo.byID[id].Status(); got != tc.status {
				t.Fatalf("status = %v; want %v", got, tc.status)
			}
		})
	}
}

func TestUpdateDetailsADuplicateCURPOnADraftIsCURPTaken(t *testing.T) {
	svc, repo, _ := newService(t)
	seedDraft(t, repo, "stu-d")
	repo.updateErr = domain.ErrCURPTaken
	if _, err := svc.UpdateDetails(context.Background(), detailsOf("stu-d", draftCURP)); !errors.Is(err, domain.ErrCURPTaken) {
		t.Fatalf("err = %v; want ErrCURPTaken", err)
	}
}
