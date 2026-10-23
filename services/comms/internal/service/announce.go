package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

const (
	// guardianChunk is how many guardians one device read asks for. It keeps the
	// read far below the repository's 500-row cap even when a parent has a few
	// phones; a larger chunk would silently drop the guardians past the cap.
	guardianChunk = 100
	// pushBudget bounds the announcement pushes once they are detached from the
	// request that started them. The pushes run one after another, so it must
	// cover a whole-cycle send (hundreds of guardians), not one push.
	pushBudget = 2 * time.Minute
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
	var reached []Notification
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
			s.pushAnnouncement(ctx, reached)
			return created, err
		}
		if !isNew {
			continue
		}
		created++
		reached = append(reached, stored)
	}
	s.pushAnnouncement(ctx, reached)
	return created, nil
}

// pushAnnouncement buzzes each guardian of the newly notified pupils ONCE, no
// matter how many of their children the announcement reached (ADR 0041 D4): a
// parent with three children must not hear the same junta three times.
//
// It only sees the pupils THIS call recorded. A send cut off halfway and
// resumed later therefore pushes again for the children the first attempt did
// not reach, which can buzz a parent a second time; ADR 0041 accepts that,
// because losing the announcement is worse than hearing it twice.
//
// Guardians are resolved one students call per newly notified pupil (students
// exposes no batched read), so the bound is the audience size: a whole-cycle
// send is one call per enrolled pupil; only the family scope is capped at the
// edge. Devices are read in chunks of guardianChunk users, because the device
// read silently stops at the repository's row cap (500) and a whole-cycle send
// reaches more guardians than that.
//
// The pushes run on their own pushBudget, detached from the request: when the
// loop stopped because the request's deadline passed, the pupils it had
// already recorded are claimed, a retry skips them, and this is their only push.
//
// Best-effort throughout: a failed lookup skips that pupil (or chunk) loudly and
// never widens the audience, and a failed push never fails the announcement.
func (s *CommsService) pushAnnouncement(ctx context.Context, reached []Notification) {
	if s.sender == nil || len(reached) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushBudget)
	defer cancel()
	if s.guardians == nil {
		s.log.Warn("comms cannot target a push without a guardian reader",
			slog.String("notification_id", reached[0].ID))
		return
	}
	order, children := s.guardiansOf(ctx, reached)
	if len(order) == 0 {
		return
	}
	for chunk := range slices.Chunk(order, guardianChunk) {
		s.pushGuardians(ctx, chunk, children)
	}
}

// pushGuardians reads the devices of one chunk of guardians and pushes each of
// them once, naming the children of theirs the announcement reached.
func (s *CommsService) pushGuardians(ctx context.Context, guardianIDs []string, children map[string][]Notification) {
	devices, err := s.repo.DevicesForUsers(ctx, guardianIDs)
	if err != nil {
		s.log.Warn("comms could not resolve push targets",
			slog.String("notification_id", children[guardianIDs[0]][0].ID),
			slog.Int("guardian_count", len(guardianIDs)),
			slog.Any("error", err))
		return
	}
	tokens := make(map[string][]string, len(guardianIDs))
	for _, d := range devices {
		tokens[d.UserID] = append(tokens[d.UserID], d.FCMToken)
	}
	for _, guardianID := range guardianIDs {
		kids := children[guardianID]
		if len(tokens[guardianID]) == 0 {
			continue
		}
		data := map[string]string{
			dataType:       typeAnnouncement,
			dataStudentID:  kids[0].StudentID,
			dataStudentIDs: joinStudentIDs(kids),
		}
		s.deliver(ctx, kids[0], tokens[guardianID], data)
	}
}

// guardiansOf maps each guardian of the reached pupils to the notifications of
// their children, both in audience order so the push payload is deterministic.
func (s *CommsService) guardiansOf(ctx context.Context, reached []Notification) ([]string, map[string][]Notification) {
	var order []string
	children := map[string][]Notification{}
	for _, n := range reached {
		guardianIDs, err := s.guardians.GuardiansForStudent(ctx, n.StudentID)
		if err != nil {
			s.log.Warn("comms could not resolve the student's guardians",
				slog.String("notification_id", n.ID),
				slog.String("student_id", n.StudentID),
				slog.Any("error", err))
			continue
		}
		for _, g := range guardianIDs {
			if _, seen := children[g]; !seen {
				order = append(order, g)
			}
			children[g] = append(children[g], n)
		}
	}
	return order, children
}

// joinStudentIDs is the comma list of the pupils one push stands for.
func joinStudentIDs(ns []Notification) string {
	ids := make([]string, len(ns))
	for i, n := range ns {
		ids[i] = n.StudentID
	}
	return strings.Join(ids, ",")
}

// announcementKey is the ledger key for one pupil's copy of an announcement.
func announcementKey(key, studentID string) string {
	return fmt.Sprintf("announcement:%s:%s", key, studentID)
}
