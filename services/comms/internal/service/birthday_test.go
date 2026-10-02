package service_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// --- fakes -------------------------------------------------------------------

// birthdayQuery is what the directory was asked: a calendar day, plus whether
// February 29 rides along.
type birthdayQuery struct {
	month, day   int
	includeFeb29 bool
}

// fakeDirectory is students' "who was born on this day", keyed by month/day.
type fakeDirectory struct {
	born    map[[2]int][]service.Pupil
	queries []birthdayQuery
	err     error
}

func (f *fakeDirectory) BirthdaysOn(_ context.Context, month, day int, includeFeb29 bool) ([]service.Pupil, error) {
	f.queries = append(f.queries, birthdayQuery{month, day, includeFeb29})
	if f.err != nil {
		return nil, f.err
	}
	out := append([]service.Pupil(nil), f.born[[2]int{month, day}]...)
	if includeFeb29 {
		out = append(out, f.born[[2]int{2, 29}]...)
	}
	return out, nil
}

// fakeTemplates is the school's saved greeting; found=false until one is saved.
type fakeTemplates struct {
	saved *service.BirthdayTemplate
}

func (f *fakeTemplates) BirthdayTemplate(context.Context) (service.BirthdayTemplate, bool, error) {
	if f.saved == nil {
		return service.BirthdayTemplate{}, false, nil
	}
	return *f.saved, true, nil
}

func (f *fakeTemplates) SaveBirthdayTemplate(_ context.Context, t service.BirthdayTemplate) (service.BirthdayTemplate, error) {
	f.saved = &t
	return t, nil
}

// failingGuardians fails for one pupil only.
type failingGuardians struct {
	*fakeGuardians
	failFor string
}

func (f failingGuardians) GuardiansForStudent(ctx context.Context, id string) ([]string, error) {
	if id == f.failFor {
		return nil, errors.New("students is down")
	}
	return f.fakeGuardians.GuardiansForStudent(ctx, id)
}

// --- helpers -----------------------------------------------------------------

func mexicoCity(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("load tz: %v", err)
	}
	return loc
}

type birthdayFixture struct {
	svc       *service.CommsService
	repo      *fakeRepo
	dir       *fakeDirectory
	templates *fakeTemplates
	sender    *spySender
}

func newBirthdayService(t *testing.T, now time.Time, guardians service.GuardianReader, born map[[2]int][]service.Pupil) birthdayFixture {
	t.Helper()
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	dir := &fakeDirectory{born: born}
	templates := &fakeTemplates{}
	sender := &spySender{}
	ids := 0
	svc := service.NewCommsService(repo, guardians, sender, tenantID,
		func() string { ids++; return "id-" + string(rune('0'+ids)) },
		func() time.Time { return now },
		slog.New(slog.DiscardHandler),
		service.WithBirthdays(dir, templates, mexicoCity(t)))
	return birthdayFixture{svc: svc, repo: repo, dir: dir, templates: templates, sender: sender}
}

var anaOct1 = map[[2]int][]service.Pupil{{10, 1}: {{ID: studentID, FirstName: "Ana Sofía"}}}

// --- tests -------------------------------------------------------------------

// The family of a pupil born today gets the greeting in the app: a feed entry
// of its own kind, and a push to the guardians' phones.
func TestRunBirthdaysGreetsTheFamily(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), anaOct1)

	run, err := f.svc.RunBirthdays(context.Background(), "2026-10-01")
	if err != nil {
		t.Fatalf("RunBirthdays: %v", err)
	}
	if run.Date != "2026-10-01" || run.Greeted != 1 || run.AlreadyGreeted != 0 || run.WithoutGuardians != 0 {
		t.Errorf("run = %+v", run)
	}
	if len(f.repo.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(f.repo.notifications))
	}
	n := f.repo.notifications[0]
	if n.Type != "birthday" || n.StudentID != studentID {
		t.Errorf("notification = %+v", n)
	}
	if !strings.Contains(n.Body, "Ana Sofía") || strings.Contains(n.Body, "{nombre}") {
		t.Errorf("body = %q: the default text must name the pupil", n.Body)
	}
	if n.Title == "" {
		t.Error("the greeting has no title")
	}
	if !f.repo.claimed["birthday:"+studentID+":2026"] {
		t.Errorf("ledger keys = %v, want birthday:<student>:<year>", f.repo.claimed)
	}
	if f.sender.calls != 1 || f.sender.tokens[0] != "tok-1" || f.sender.data["type"] != "birthday" || f.sender.data["studentId"] != studentID {
		t.Errorf("push = %d %v %v", f.sender.calls, f.sender.tokens, f.sender.data)
	}
}

// The scheduler retries, and anybody can curl the run again: a family is
// greeted once a year, however many times the job runs that day.
func TestRunBirthdaysIsIdempotent(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), anaOct1)
	ctx := context.Background()
	if _, err := f.svc.RunBirthdays(ctx, "2026-10-01"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	run, err := f.svc.RunBirthdays(ctx, "2026-10-01")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run.Greeted != 0 || run.AlreadyGreeted != 1 {
		t.Errorf("second run = %+v, want everybody already greeted", run)
	}
	if len(f.repo.notifications) != 1 || f.sender.calls != 1 {
		t.Errorf("notifications = %d, pushes = %d, want 1 and 1", len(f.repo.notifications), f.sender.calls)
	}

	// Next year is a new birthday.
	next, err := f.svc.RunBirthdays(ctx, "2027-10-01")
	if err != nil || next.Greeted != 1 {
		t.Errorf("next year = %+v %v, want greeted again", next, err)
	}
}

// A pupil whose family has no account yet is skipped and counted — and NOT
// marked greeted, so linking the family that same morning and re-running still
// reaches them.
func TestRunBirthdaysSkipsPupilsWithoutGuardians(t *testing.T) {
	guardians := &fakeGuardians{links: map[string][]string{}}
	f := newBirthdayService(t, fixedNow, guardians, anaOct1)
	ctx := context.Background()

	run, err := f.svc.RunBirthdays(ctx, "2026-10-01")
	if err != nil {
		t.Fatalf("RunBirthdays: %v", err)
	}
	if run.Greeted != 0 || run.WithoutGuardians != 1 || len(f.repo.notifications) != 0 || f.sender.calls != 0 {
		t.Errorf("run = %+v, notifications = %d, pushes = %d", run, len(f.repo.notifications), f.sender.calls)
	}

	guardians.links[studentID] = []string{userID}
	again, err := f.svc.RunBirthdays(ctx, "2026-10-01")
	if err != nil || again.Greeted != 1 {
		t.Errorf("after linking = %+v %v, want greeted", again, err)
	}
}

// No date means TODAY IN THE SCHOOL'S ZONE. At 21:00 in Mexico City it is
// already tomorrow in UTC; the greeting must not go out a day early.
func TestRunBirthdaysUsesTheSchoolsLocalDate(t *testing.T) {
	ninePMOct1 := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC) // 2026-10-01 21:00 -06:00
	f := newBirthdayService(t, ninePMOct1, newFakeGuardians(), anaOct1)

	run, err := f.svc.RunBirthdays(context.Background(), "")
	if err != nil {
		t.Fatalf("RunBirthdays: %v", err)
	}
	if run.Date != "2026-10-01" || run.Greeted != 1 {
		t.Errorf("run = %+v, want 2026-10-01 greeted", run)
	}
	if got := f.dir.queries[0]; got.month != 10 || got.day != 1 {
		t.Errorf("asked for %+v, want October 1", got)
	}
}

// February 29 pupils are greeted on February 28 in a year without a 29th —
// still in their own month — and on the 29th when there is one.
func TestRunBirthdaysLeapDay(t *testing.T) {
	cases := []struct {
		date string
		want birthdayQuery
	}{
		{"2027-02-28", birthdayQuery{2, 28, true}},  // not a leap year
		{"2028-02-28", birthdayQuery{2, 28, false}}, // leap year: the 29th comes
		{"2028-02-29", birthdayQuery{2, 29, false}},
		{"2100-02-28", birthdayQuery{2, 28, true}}, // divisible by 100: not leap
		{"2026-03-01", birthdayQuery{3, 1, false}},
	}
	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			f := newBirthdayService(t, fixedNow, newFakeGuardians(), nil)
			if _, err := f.svc.RunBirthdays(context.Background(), tc.date); err != nil {
				t.Fatalf("RunBirthdays: %v", err)
			}
			if got := f.dir.queries[0]; got != tc.want {
				t.Errorf("asked for %+v, want %+v", got, tc.want)
			}
		})
	}

	leapling := map[[2]int][]service.Pupil{{2, 29}: {{ID: studentID, FirstName: "Bisi"}}}
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), leapling)
	run, err := f.svc.RunBirthdays(context.Background(), "2027-02-28")
	if err != nil || run.Greeted != 1 {
		t.Errorf("leap-day pupil on 2027-02-28 = %+v %v, want greeted", run, err)
	}
}

func TestRunBirthdaysRejectsAMalformedDate(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), anaOct1)
	for _, raw := range []string{"01/10/2026", "2026-02-30", "mañana"} {
		if _, err := f.svc.RunBirthdays(context.Background(), raw); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%q: want KindInvalid, got %v", raw, err)
		}
	}
	if len(f.dir.queries) != 0 {
		t.Error("a malformed date reached the directory")
	}
}

// The school's own text replaces the default, {nombre} wherever it appears.
func TestRunBirthdaysUsesTheSchoolsTemplate(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), anaOct1)
	f.templates.saved = &service.BirthdayTemplate{Title: "¡Felicidades, {nombre}!", Body: "{nombre}: hoy es tu día, {nombre}."}

	if _, err := f.svc.RunBirthdays(context.Background(), "2026-10-01"); err != nil {
		t.Fatalf("RunBirthdays: %v", err)
	}
	n := f.repo.notifications[0]
	if n.Title != "¡Felicidades, Ana Sofía!" || n.Body != "Ana Sofía: hoy es tu día, Ana Sofía." {
		t.Errorf("rendered = %q / %q", n.Title, n.Body)
	}
}

// students down: nothing can be greeted, and the run says so — the scheduler's
// retry is the recovery.
func TestRunBirthdaysReportsADirectoryOutage(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), anaOct1)
	f.dir.err = errors.New("students is down")
	if _, err := f.svc.RunBirthdays(context.Background(), "2026-10-01"); err == nil {
		t.Error("want an error when the directory fails")
	}
}

// One pupil whose guardians cannot be read must not stop the rest of the list;
// the run still fails, so the scheduler retries and greets the one it missed.
func TestRunBirthdaysKeepsGoingPastOneFailure(t *testing.T) {
	two := map[[2]int][]service.Pupil{{10, 1}: {
		{ID: secondStudentID, FirstName: "Luis"},
		{ID: studentID, FirstName: "Ana"},
	}}
	f := newBirthdayService(t, fixedNow, failingGuardians{newFakeGuardians(), secondStudentID}, two)

	run, err := f.svc.RunBirthdays(context.Background(), "2026-10-01")
	if err == nil {
		t.Error("want the run to report the failure")
	}
	if run.Greeted != 1 || len(f.repo.notifications) != 1 || f.repo.notifications[0].StudentID != studentID {
		t.Errorf("run = %+v, notifications = %+v", run, f.repo.notifications)
	}
}

// --- template -----------------------------------------------------------------

func TestBirthdayTemplateDefaultsUntilTheSchoolWritesOne(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), nil)
	ctx := context.Background()

	got, err := f.svc.BirthdayTemplate(ctx)
	if err != nil {
		t.Fatalf("BirthdayTemplate: %v", err)
	}
	if !got.IsDefault || !strings.Contains(got.Body, "{nombre}") || got.Title == "" {
		t.Errorf("default = %+v", got)
	}

	saved, err := f.svc.UpdateBirthdayTemplate(ctx, "  ¡Feliz día!  ", "  Felicidades, {nombre}.  ", userID)
	if err != nil {
		t.Fatalf("UpdateBirthdayTemplate: %v", err)
	}
	if saved.IsDefault || saved.Title != "¡Feliz día!" || saved.Body != "Felicidades, {nombre}." || saved.UpdatedBy != userID || !saved.UpdatedAt.Equal(fixedNow) {
		t.Errorf("saved = %+v", saved)
	}
	again, err := f.svc.BirthdayTemplate(ctx)
	if err != nil || again.IsDefault || again.Body != "Felicidades, {nombre}." {
		t.Errorf("after save = %+v %v", again, err)
	}
}

func TestUpdateBirthdayTemplateValidates(t *testing.T) {
	cases := []struct {
		name, title, body, by string
	}{
		{"empty body", "Hola", "   ", userID},
		{"empty title", "", "Felicidades", userID},
		{"body too long", "Hola", strings.Repeat("a", 501), userID},
		{"title too long", strings.Repeat("a", 81), "Felicidades", userID},
		{"nobody saved it", "Hola", "Felicidades", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBirthdayService(t, fixedNow, newFakeGuardians(), nil)
			if _, err := f.svc.UpdateBirthdayTemplate(context.Background(), tc.title, tc.body, tc.by); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
			if f.templates.saved != nil {
				t.Error("an invalid template was saved")
			}
		})
	}
}

// A 500-character greeting in Spanish counts characters, not bytes.
func TestUpdateBirthdayTemplateCountsCharacters(t *testing.T) {
	f := newBirthdayService(t, fixedNow, newFakeGuardians(), nil)
	if _, err := f.svc.UpdateBirthdayTemplate(context.Background(), "Hola", strings.Repeat("ñ", 500), userID); err != nil {
		t.Errorf("500 ñ: %v", err)
	}
}
