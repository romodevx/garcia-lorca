package handler

import (
	"context"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// family_attendance_periods.go places the family attendance view's group
// windows: which group's lists were the child's on which days. (Not named
// *_windows.go: Go would read that suffix as a GOOS build constraint.)
//
// How the students context records a school career, and why each rule below
// exists:
//
//   - one enrollment row per (student, cycle); enrolled_at is when the ROW was
//     created — a re-enrollment for next year can be created in June;
//   - a transfer rewrites group_id on the SAME row, and the move lives in
//     enrollment_transfers (from_group, to_group, moved_at);
//   - a withdrawn enrollment says when it ended (ended_at, its status change).

// enrollmentWindow is the span of school days one group's lists were the
// child's, through one enrollment. Days are ISO dates, both ends inclusive.
type enrollmentWindow struct {
	enrollmentID string
	groupID      string
	from, to     string
}

func (w enrollmentWindow) covers(groupID, date string) bool {
	return w.groupID == groupID && date >= w.from && date <= w.to
}

// buildGroupWindows derives every group window of the child's enrollments:
//
//  1. the enrollment's active period is its cycle's [starts_on, ends_on]
//     intersected with [enrolled_at, today] — so an early re-enrollment for
//     next year never cuts this year short, and a mid-year newcomer does not
//     claim the lists taken before they arrived;
//  2. a withdrawn/transferred-out enrollment ends the day BEFORE its
//     status-change date; with no date recorded it keeps its lists up to the
//     cycle's end (never after today) rather than losing its history;
//  3. the period is split by the enrollment's transfers: from_group until the
//     day before moved_at, to_group from moved_at on (the move day is the new
//     group's).
//
// An enrollment whose cycle is unknown is bounded by enrolled_at and today only.
func buildGroupWindows(enrollments []client.Enrollment, transfers []client.Transfer, cycles map[string]client.CycleResult, today time.Time) []enrollmentWindow {
	moves := movesByEnrollment(transfers)
	out := make([]enrollmentWindow, 0, len(enrollments))
	for _, e := range enrollments {
		from, to, ok := activePeriod(e, cycles, today)
		if !ok || e.GroupID == "" {
			continue
		}
		out = append(out, splitByMoves(e, moves[e.ID], from, to)...)
	}
	return out
}

// movesByEnrollment groups the transfers by enrollment, oldest move first.
func movesByEnrollment(transfers []client.Transfer) map[string][]client.Transfer {
	out := map[string][]client.Transfer{}
	for _, t := range transfers {
		out[t.EnrollmentID] = append(out[t.EnrollmentID], t)
	}
	for _, ts := range out {
		sort.SliceStable(ts, func(i, j int) bool { return ts[i].MovedAt.Before(ts[j].MovedAt) })
	}
	return out
}

// activePeriod is rules 1 and 2 of buildGroupWindows, as UTC days.
func activePeriod(e client.Enrollment, cycles map[string]client.CycleResult, today time.Time) (from, to time.Time, ok bool) {
	from, to = firstServedMonth, today
	from = later(from, utcDay(e.EnrolledAt))
	if cycle, known := cycles[e.CycleID]; known {
		from = later(from, utcDay(cycle.StartsOn))
		if !cycle.EndsOn.IsZero() {
			to = earlier(to, utcDay(cycle.EndsOn))
		}
	}
	if e.Status != "active" && !e.EndedAt.IsZero() {
		to = earlier(to, utcDay(e.EndedAt).AddDate(0, 0, -1))
	}
	return from, to, !to.Before(from)
}

// splitByMoves is rule 3 of buildGroupWindows: one window per group the
// enrollment was in, clipped to its active period.
func splitByMoves(e client.Enrollment, moves []client.Transfer, from, to time.Time) []enrollmentWindow {
	group := e.GroupID
	if len(moves) > 0 {
		group = moves[0].FromGroupID
	}
	out := make([]enrollmentWindow, 0, len(moves)+1)
	start := from
	for _, m := range moves {
		moved := utcDay(m.MovedAt)
		out = appendWindow(out, e.ID, group, later(start, from), earlier(moved.AddDate(0, 0, -1), to))
		group, start = m.ToGroupID, moved
	}
	return appendWindow(out, e.ID, group, later(start, from), to)
}

// appendWindow adds [from, to] for groupID unless it is empty.
func appendWindow(out []enrollmentWindow, enrollmentID, groupID string, from, to time.Time) []enrollmentWindow {
	if to.Before(from) {
		return out
	}
	return append(out, enrollmentWindow{
		enrollmentID: enrollmentID,
		groupID:      groupID,
		from:         from.Format(attendanceDateLayout),
		to:           to.Format(attendanceDateLayout),
	})
}

// overlapping keeps the windows that share a day with [from, to] (ISO days).
func overlapping(windows []enrollmentWindow, from, to string) []enrollmentWindow {
	out := make([]enrollmentWindow, 0, len(windows))
	for _, w := range windows {
		if w.from <= to && w.to >= from {
			out = append(out, w)
		}
	}
	return out
}

// distinct is the distinct keys of the windows, in first-seen order.
func distinct(windows []enrollmentWindow, key func(enrollmentWindow) string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(windows))
	for _, w := range windows {
		if k := key(w); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// chunk splits ids into slices of at most size.
func chunk(ids []string, size int) [][]string {
	out := make([][]string, 0, (len(ids)+size-1)/size)
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

// fanOut runs read(ctx, 0…n-1) concurrently and concatenates the results in
// index order (deterministic). The first failure cancels the others' context
// and is returned; every goroutine has finished when fanOut returns.
func fanOut[T any](ctx context.Context, n int, read func(ctx context.Context, i int) ([]T, error)) ([]T, error) {
	results := make([][]T, n)
	g, gctx := errgroup.WithContext(ctx)
	for i := range n {
		g.Go(func() error {
			var err error
			results[i], err = read(gctx, i)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	var out []T
	for _, r := range results {
		out = append(out, r...)
	}
	return out, nil
}

// utcDay is t's UTC calendar day at midnight; the zero time stays zero.
func utcDay(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
