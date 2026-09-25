package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

type fakeCards struct {
	byToken map[string]domain.CardToken
}

func newFakeCards() *fakeCards { return &fakeCards{byToken: map[string]domain.CardToken{}} }

func (f *fakeCards) ActiveCard(_ context.Context, studentID string) (domain.CardToken, bool, error) {
	for _, c := range f.byToken {
		if c.StudentID() == studentID && c.Active() {
			return c, true, nil
		}
	}
	return domain.CardToken{}, false, nil
}

func (f *fakeCards) SaveCard(_ context.Context, c domain.CardToken) error {
	f.byToken[c.Token()] = c
	return nil
}

func (f *fakeCards) RevokeCards(_ context.Context, studentID string, at time.Time) error {
	for k, c := range f.byToken {
		if c.StudentID() == studentID && c.Active() {
			f.byToken[k] = c.Revoke(at)
		}
	}
	return nil
}

func (f *fakeCards) FindCard(_ context.Context, token string) (domain.CardToken, error) {
	c, ok := f.byToken[token]
	if !ok {
		return domain.CardToken{}, glerrors.ErrNotFound
	}
	return c, nil
}

type fakePupils map[string]*domain.Student

func (f fakePupils) FindByID(_ context.Context, id string) (*domain.Student, error) {
	s, ok := f[id]
	if !ok {
		return nil, glerrors.ErrNotFound
	}
	return s, nil
}

func pupilWith(t *testing.T, id string, status domain.AdmissionStatus) *domain.Student {
	t.Helper()
	c, err := domain.NewCURP("CARD050312MDFRNN01")
	if err != nil {
		t.Fatalf("curp: %v", err)
	}
	return domain.Rehydrate(domain.NewApplicantParams{ID: id, TenantID: "t1", CURP: c, FirstName: "Zoe", LastName: "V",
		Level: domain.RestoreLevel("lv-1", "Primaria")}, status)
}

func cardService(pupils fakePupils) (*usecase.CardService, *fakeCards) {
	cards := newFakeCards()
	n := 0
	random := func() ([16]byte, error) { n++; return [16]byte{byte(n)}, nil }
	ids := 0
	newID := func() string { ids++; return "ct-" + string(rune('0'+ids)) }
	return usecase.NewCardService(cards, pupils, "t1", newID, random, func() time.Time { return issued }), cards
}

var issued = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// Task 13: a pupil's card shows one code until it is revoked; then a new one,
// and the old one no longer verifies.
func TestACardCodeLastsUntilRevoked(t *testing.T) {
	svc, _ := cardService(fakePupils{"stu-1": pupilWith(t, "stu-1", domain.AdmissionEnrolled)})
	ctx := context.Background()

	first, err := svc.CardCode(ctx, "stu-1")
	if err != nil {
		t.Fatalf("CardCode: %v", err)
	}
	again, err := svc.CardCode(ctx, "stu-1")
	if err != nil || again != first {
		t.Fatalf("second CardCode = %q (%v), want the same %q", again, err, first)
	}

	if err := svc.RevokeCard(ctx, "stu-1"); err != nil {
		t.Fatalf("RevokeCard: %v", err)
	}
	fresh, err := svc.CardCode(ctx, "stu-1")
	if err != nil || fresh == first {
		t.Fatalf("after revoking: %q (%v), want a new code", fresh, err)
	}
	old, err := svc.VerifyCard(ctx, first)
	if err != nil || old.Valid {
		t.Errorf("the revoked code verifies as %+v (%v); want it known but not valid", old, err)
	}
}

// Valid means an active code of an enrolled pupil. A withdrawn pupil's card
// says who it was, and that it no longer counts.
func TestVerifyingACard(t *testing.T) {
	pupils := fakePupils{
		"stu-1": pupilWith(t, "stu-1", domain.AdmissionEnrolled),
		"stu-2": pupilWith(t, "stu-2", domain.AdmissionWithdrawn),
	}
	svc, _ := cardService(pupils)
	ctx := context.Background()
	enrolled, _ := svc.CardCode(ctx, "stu-1")
	gone, _ := svc.CardCode(ctx, "stu-2")

	got, err := svc.VerifyCard(ctx, enrolled)
	if err != nil || !got.Valid || got.Student.ID() != "stu-1" {
		t.Errorf("enrolled pupil: %+v (%v), want valid", got, err)
	}
	got, err = svc.VerifyCard(ctx, gone)
	if err != nil || got.Valid || got.Student.ID() != "stu-2" {
		t.Errorf("withdrawn pupil: %+v (%v), want known and not valid", got, err)
	}
	if _, err := svc.VerifyCard(ctx, "GLCARD:AAAAAAAAAAAAAAAAAAAAAA"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("an unknown code: %v, want KindNotFound", err)
	}
	if _, err := svc.VerifyCard(ctx, "Zoe Villalobos"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("not a code: %v, want KindInvalid", err)
	}
}

func TestACardForAnUnknownPupilIsNotFound(t *testing.T) {
	svc, _ := cardService(fakePupils{})
	if _, err := svc.CardCode(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("CardCode(unknown) = %v, want KindNotFound", err)
	}
}

// racingCards is a store where another request issued the pupil's card between
// this one's look and its save.
type racingCards struct {
	*fakeCards
	winner domain.CardToken
	saved  bool
}

func (r *racingCards) ActiveCard(ctx context.Context, studentID string) (domain.CardToken, bool, error) {
	if r.saved {
		return r.winner, true, nil
	}
	return domain.CardToken{}, false, nil
}

func (r *racingCards) SaveCard(_ context.Context, _ domain.CardToken) error {
	r.saved = true
	return glerrors.New(glerrors.KindConflict, "the pupil already has an active card")
}

// Two first views of a card at once: the loser shows the winner's code.
func TestTwoFirstViewsShowOneCode(t *testing.T) {
	winner, err := domain.NewCardToken("ct-w", "t1", "stu-1", [16]byte{7}, issued)
	if err != nil {
		t.Fatalf("NewCardToken: %v", err)
	}
	cards := &racingCards{fakeCards: newFakeCards(), winner: winner}
	svc := usecase.NewCardService(cards, fakePupils{"stu-1": pupilWith(t, "stu-1", domain.AdmissionEnrolled)}, "t1",
		func() string { return "ct-l" }, func() ([16]byte, error) { return [16]byte{8}, nil }, func() time.Time { return issued })

	got, err := svc.CardCode(context.Background(), "stu-1")
	if err != nil || got != winner.Code() {
		t.Errorf("CardCode = %q (%v), want the winner's %q", got, err, winner.Code())
	}
}
