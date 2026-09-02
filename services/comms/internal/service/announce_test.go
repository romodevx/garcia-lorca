package service_test

import (
	"context"
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
