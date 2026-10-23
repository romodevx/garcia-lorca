package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const secondStudentID = "aaaaaaaa-0000-0000-0000-000000000002"

// The school writes once and every family in the group hears it. Until now the
// only way to reach a group was to send a message per pupil, which is how a
// school ends up sending it to twenty-eight of thirty.
func TestAnnounceReachesEveryStudentGiven(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{studentID: {userID}}}, nil)

	created, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID},
		Title:      "Junta de padres",
		Body:       "Jueves 8:00 en el auditorio.",
		Key:        "ann-1",
	})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if created != 2 {
		t.Errorf("created = %d, want 2", created)
	}
	if len(repo.notifications) != 2 {
		t.Fatalf("stored %d notifications, want 2", len(repo.notifications))
	}
	if repo.notifications[0].Title != "Junta de padres" {
		t.Errorf("title = %q", repo.notifications[0].Title)
	}
}

// One announcement, four hundred families. A retry after a timeout must not
// deliver it twice — the phone buzzing again for the same junta is exactly the
// thing that makes a school turn notifications off.
func TestAnnounceIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{}}, nil)
	announcement := service.Announcement{
		StudentIDs: []string{studentID, secondStudentID},
		Title:      "Junta de padres",
		Body:       "Jueves 8:00.",
		Key:        "ann-1",
	}

	if _, err := svc.Announce(context.Background(), announcement); err != nil {
		t.Fatalf("first Announce: %v", err)
	}
	created, err := svc.Announce(context.Background(), announcement)
	if err != nil {
		t.Fatalf("second Announce: %v", err)
	}
	if created != 0 {
		t.Errorf("the replay created %d notifications, want 0", created)
	}
	if len(repo.notifications) != 2 {
		t.Errorf("stored %d notifications after a replay, want 2", len(repo.notifications))
	}
}

// An announcement to nobody is a mistake at the call site — a group with no
// pupils, a filter that matched nothing — and silence would hide it.
func TestAnnounceToNobodyIsRefused(t *testing.T) {
	svc, _ := newService(t, newFakeRepo(), &fakeGuardians{links: map[string][]string{}}, nil)

	_, err := svc.Announce(context.Background(), service.Announcement{
		Title: "Junta", Body: "Jueves", Key: "ann-1",
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("empty audience: want KindInvalid, got %v", err)
	}
}

func TestAnnounceNeedsWordsToSay(t *testing.T) {
	svc, _ := newService(t, newFakeRepo(), &fakeGuardians{links: map[string][]string{}}, nil)

	_, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID}, Title: "  ", Body: "Jueves", Key: "ann-1",
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank title: want KindInvalid, got %v", err)
	}
}

// The families reachable on a phone get the push; the feed entry is durable for
// everyone regardless. A school still onboarding families must still be able to
// announce.
func TestAnnouncePushesToTheGuardiansItCanReach(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1", Platform: "android"}}
	sender := &spySender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{studentID: {userID}}}, sender)

	if _, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID},
		Title:      "Junta de padres", Body: "Jueves 8:00.", Key: "ann-1",
	}); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	if len(sender.tokens) != 1 || sender.tokens[0] != "tok-1" {
		t.Errorf("pushed to %v, want the one device we know about", sender.tokens)
	}
}

// --- one push per account (ADR 0041 D4) ---------------------------------------

const (
	thirdStudentID  = "aaaaaaaa-0000-0000-0000-000000000003"
	fourthStudentID = "aaaaaaaa-0000-0000-0000-000000000004"
	secondUserID    = "bbbbbbbb-0000-0000-0000-000000000002"
)

// pushesByToken indexes the pushes by the device they went to, so an assertion reads
// "this phone buzzed with this payload" whatever order the pushes ran in.
func pushesByToken(sender *spySender) map[string][]service.PushMessage {
	out := map[string][]service.PushMessage{}
	for _, m := range sender.messages {
		for _, tok := range m.Tokens {
			out[tok] = append(out[tok], m)
		}
	}
	return out
}

// A parent with three children in the audience gets the announcement in each
// child's feed, but the phone buzzes once.
func TestAnnouncePushesOncePerGuardianAcrossSiblings(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	sender := &spySender{}
	guardians := &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID}, thirdStudentID: {userID},
	}}
	svc, _ := newService(t, repo, guardians, sender)

	created, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{secondStudentID, studentID, thirdStudentID},
		Title:      "Junta", Body: "Jueves 8:00.", Key: "ann-1",
	})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if created != 3 || len(repo.notifications) != 3 {
		t.Fatalf("created %d / stored %d, want one feed entry per child", created, len(repo.notifications))
	}
	if sender.calls != 1 {
		t.Fatalf("pushed %d times, want once for the one parent", sender.calls)
	}
	data := sender.messages[0].Data
	if data["type"] != "announcement" {
		t.Errorf("type = %q", data["type"])
	}
	// The first child in the AUDIENCE order, not in any map order.
	if data["studentId"] != secondStudentID {
		t.Errorf("studentId = %q, want the first child in audience order %q", data["studentId"], secondStudentID)
	}
	want := secondStudentID + "," + studentID + "," + thirdStudentID
	if data["studentIds"] != want {
		t.Errorf("studentIds = %q, want %q", data["studentIds"], want)
	}
	if repo.deviceCalls != 1 {
		t.Errorf("device lookups = %d, want one batched read", repo.deviceCalls)
	}
}

// Two parents, each with two children: two phones, one buzz each, and each push
// names only that parent's children.
func TestAnnouncePushesOncePerEachGuardian(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{
		{ID: "dev-1", UserID: userID, FCMToken: "tok-1"},
		{ID: "dev-2", UserID: secondUserID, FCMToken: "tok-2"},
	}
	sender := &spySender{}
	guardians := &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID},
		thirdStudentID: {secondUserID}, fourthStudentID: {secondUserID},
	}}
	svc, _ := newService(t, repo, guardians, sender)

	if _, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, thirdStudentID, secondStudentID, fourthStudentID},
		Title:      "Junta", Body: "Jueves 8:00.", Key: "ann-1",
	}); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if sender.calls != 2 {
		t.Fatalf("pushed %d times, want 2 (one per parent)", sender.calls)
	}
	byToken := pushesByToken(sender)
	if got := byToken["tok-1"]; len(got) != 1 || got[0].Data["studentIds"] != studentID+","+secondStudentID {
		t.Errorf("parent 1 pushes = %+v", got)
	}
	if got := byToken["tok-2"]; len(got) != 1 || got[0].Data["studentIds"] != thirdStudentID+","+fourthStudentID {
		t.Errorf("parent 2 pushes = %+v", got)
	}
}

// A retry of an announcement already delivered writes nothing and buzzes nobody.
func TestAnnounceRetryPushesNothing(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	sender := &spySender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID},
	}}, sender)
	a := service.Announcement{StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1"}

	if _, err := svc.Announce(context.Background(), a); err != nil {
		t.Fatalf("first Announce: %v", err)
	}
	if _, err := svc.Announce(context.Background(), a); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if sender.calls != 1 {
		t.Errorf("pushed %d times over send + retry, want 1", sender.calls)
	}
}

// A send cut off halfway and resumed pushes only for the children it had not
// reached — which may buzz a parent a second time (accepted by ADR 0041).
func TestAnnounceResumedSendPushesOnlyForTheChildrenNotReached(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	repo.claimed["announcement:ann-1:"+studentID] = true // reached by the first attempt
	sender := &spySender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID},
	}}, sender)

	created, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if created != 1 || sender.calls != 1 {
		t.Fatalf("created %d, pushed %d; want 1 and 1", created, sender.calls)
	}
	if got := sender.messages[0].Data["studentIds"]; got != secondStudentID {
		t.Errorf("studentIds = %q, want only the child not reached before", got)
	}
}

// One parent's push failing does not stop the next parent's, nor the
// announcement itself.
func TestAnnouncePushFailureIsIsolated(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{
		{ID: "dev-1", UserID: userID, FCMToken: "tok-1"},
		{ID: "dev-2", UserID: secondUserID, FCMToken: "tok-2"},
	}
	sender := &spySender{failFor: "tok-1"}
	guardians := &fakeGuardians{links: map[string][]string{studentID: {userID}, secondStudentID: {secondUserID}}}
	svc, logs := newService(t, repo, guardians, sender)

	created, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	})
	if err != nil || created != 2 {
		t.Fatalf("Announce = %d, %v; a push failure must never fail the announcement", created, err)
	}
	if len(pushesByToken(sender)["tok-2"]) != 1 {
		t.Errorf("the second parent was not reached after the first push failed")
	}
	if !strings.Contains(logs.String(), "comms push not delivered") {
		t.Errorf("the failed push was not logged:\n%s", logs.String())
	}
}

// A guardian lookup that fails for one child skips that child's push loudly; the
// rest of the audience is still reached, and nobody else is pushed instead.
func TestAnnounceGuardianLookupFailureSkipsOnlyThatChild(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "dev-2", UserID: secondUserID, FCMToken: "tok-2"}}
	sender := &spySender{}
	guardians := &failingFor{fakeGuardians: fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {secondUserID},
	}}, fail: studentID}
	svc, logs := newService(t, repo, guardians, sender)

	if _, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	}); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if sender.calls != 1 || sender.messages[0].Data["studentIds"] != secondStudentID {
		t.Errorf("pushes = %+v, want only the second child's parent", sender.messages)
	}
	if !strings.Contains(logs.String(), "could not resolve the student's guardians") {
		t.Errorf("the failed lookup was not logged:\n%s", logs.String())
	}
}

// failingFor is a guardian reader whose lookup fails for exactly one pupil.
type failingFor struct {
	fakeGuardians
	fail string
}

func (f *failingFor) GuardiansForStudent(ctx context.Context, studentID string) ([]string, error) {
	if studentID == f.fail {
		return nil, errors.New("students unavailable")
	}
	return f.fakeGuardians.GuardiansForStudent(ctx, studentID)
}

// A send that fails halfway still pushes for the pupils it DID record: the retry
// will skip their claimed keys, so this is their only chance to be heard.
func TestAnnounceFailingHalfwayPushesForThoseAlreadyRecorded(t *testing.T) {
	repo := &failOnNthRecord{fakeRepo: newFakeRepo(), n: 2}
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	sender := &spySender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID},
	}}, sender)

	created, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	})
	if err == nil || created != 1 {
		t.Fatalf("Announce = %d, %v; want 1 recorded and the store error", created, err)
	}
	if sender.calls != 1 || sender.messages[0].Data["studentIds"] != studentID {
		t.Errorf("pushes = %+v, want one for the pupil recorded before the failure", sender.messages)
	}
}

// failOnNthRecord is a repo whose n-th RecordEvent fails.
type failOnNthRecord struct {
	*fakeRepo
	n, calls int
}

func (f *failOnNthRecord) RecordEvent(ctx context.Context, key string, n service.Notification) (service.Notification, bool, error) {
	f.calls++
	if f.calls == f.n {
		return service.Notification{}, false, errors.New("store unavailable")
	}
	return f.fakeRepo.RecordEvent(ctx, key, n)
}

// A whole-cycle send reaches more guardians than one device read returns (the
// repo caps it at 500 rows): every guardian must still be pushed.
func TestAnnouncePushesEveryGuardianBeyondOneDeviceRead(t *testing.T) {
	const families = 600
	repo := newFakeRepo()
	links := map[string][]string{}
	pupils := make([]string, 0, families)
	for i := range families {
		pupil, guardian := fmt.Sprintf("pupil-%03d", i), fmt.Sprintf("guardian-%03d", i)
		pupils = append(pupils, pupil)
		links[pupil] = []string{guardian}
		repo.devices = append(repo.devices, service.Device{ID: "dev-" + guardian, UserID: guardian, FCMToken: "tok-" + guardian})
	}
	sender := &spySender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: links}, sender)

	if _, err := svc.Announce(context.Background(), service.Announcement{
		StudentIDs: pupils, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	}); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if sender.calls != families {
		t.Errorf("pushed %d guardians, want all %d", sender.calls, families)
	}
	if repo.maxDeviceIDs > 100 {
		t.Errorf("one device read asked for %d guardians, want chunks of at most 100", repo.maxDeviceIDs)
	}
}

// deadlineRepo fails the n-th claim the way a request past its deadline does.
type deadlineRepo struct {
	*fakeRepo
	n, calls int
	cancel   context.CancelFunc
}

func (f *deadlineRepo) RecordEvent(ctx context.Context, key string, n service.Notification) (service.Notification, bool, error) {
	f.calls++
	if f.calls == f.n {
		f.cancel()
		return service.Notification{}, false, context.DeadlineExceeded
	}
	return f.fakeRepo.RecordEvent(ctx, key, n)
}

// ctxCheckingSender refuses to push on a dead context, like a real HTTP client.
type ctxCheckingSender struct{ spySender }

func (s *ctxCheckingSender) Push(ctx context.Context, m service.PushMessage) (service.PushReport, error) {
	if err := ctx.Err(); err != nil {
		return service.PushReport{}, err
	}
	return s.spySender.Push(ctx, m)
}

// The request dying mid-send must not take the pushes of the pupils already
// recorded with it: their keys are claimed, so the retry will never push them.
func TestAnnounceDeadlineMidSendStillPushesThoseRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &deadlineRepo{fakeRepo: newFakeRepo(), n: 2, cancel: cancel}
	repo.devices = []service.Device{{ID: "dev-1", UserID: userID, FCMToken: "tok-1"}}
	sender := &ctxCheckingSender{}
	svc, _ := newService(t, repo, &fakeGuardians{links: map[string][]string{
		studentID: {userID}, secondStudentID: {userID},
	}}, sender)

	_, err := svc.Announce(ctx, service.Announcement{
		StudentIDs: []string{studentID, secondStudentID}, Title: "Junta", Body: "Jueves.", Key: "ann-1",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if sender.calls != 1 || sender.messages[0].Data["studentIds"] != studentID {
		t.Errorf("pushes = %+v, want one for the pupil recorded before the deadline", sender.messages)
	}
}
