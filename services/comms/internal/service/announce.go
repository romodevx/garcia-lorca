package service

import (
	"context"
	"fmt"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// typeAnnouncement is what the client switches on to draw an announcement: a
// message the school wrote, not something that happened to a pupil.
const typeAnnouncement = "announcement"

// Announcement is one message the school sends to many families at once.
//
// The audience arrives as a list of pupils, already resolved. Comms deliberately
// does not know what a group or a level is: the gateway owns that question
// because it already speaks to config and students, and teaching it to comms
// would give the notification service a second reason to change.
type Announcement struct {
	StudentIDs []string
	Title      string
	Body       string
	// Key makes the send idempotent. One announcement can reach four hundred
	// families; a retry after a timeout must not reach them twice, because a
	// phone buzzing again for the same junta is what makes a school turn
	// notifications off for good.
	Key string
}

// Announce writes one notification per pupil and pushes to the families it can
// reach.
//
// The feed entry is the durable part and is written for every pupil in the
// audience, whether or not their family has an account yet. The push is
// best-effort on top, exactly as it is for an event-driven notification: a
// school still onboarding its families must still be able to announce.
func (s *CommsService) Announce(ctx context.Context, a Announcement) (int, error) {
	title, body := strings.TrimSpace(a.Title), strings.TrimSpace(a.Body)
	if len(a.StudentIDs) == 0 {
		// A group with no pupils, or a filter that matched nothing. Silence here
		// would hide a mistake made at the call site.
		return 0, glerrors.New(glerrors.KindInvalid, "an announcement needs somebody to send it to")
	}
	if title == "" || body == "" {
		return 0, glerrors.New(glerrors.KindInvalid, "an announcement needs a title and a message")
	}
	if strings.TrimSpace(a.Key) == "" {
		return 0, glerrors.New(glerrors.KindInvalid, "an announcement needs an idempotency key")
	}

	created := 0
	for _, studentID := range a.StudentIDs {
		n := Notification{
			ID:        s.newID(),
			StudentID: studentID,
			Type:      typeAnnouncement,
			Title:     title,
			Body:      body,
			CreatedAt: s.now(),
		}
		// Keyed per pupil so a send interrupted halfway resumes rather than
		// starting over: the families already notified are skipped, and the rest
		// still hear it.
		stored, isNew, err := s.repo.RecordEvent(ctx, announcementKey(a.Key, studentID), n)
		if err != nil {
			return created, err
		}
		if !isNew {
			continue
		}
		created++
		s.push(ctx, stored, map[string]string{dataType: typeAnnouncement, dataStudentID: studentID})
	}
	return created, nil
}

// announcementKey is the ledger key for one pupil's copy of an announcement.
func announcementKey(key, studentID string) string {
	return fmt.Sprintf("announcement:%s:%s", key, studentID)
}
