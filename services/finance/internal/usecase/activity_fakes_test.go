package usecase_test

import (
	"context"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

var _ ports.ActivityRepository = (*fakeActivityRepo)(nil)

// fakeActivityRepo keeps activities and enrolments in memory, with the same
// rules the database enforces: one name per school (case-insensitive), one
// open enrolment per pupil and activity, a used option cannot be retired.
type fakeActivityRepo struct {
	activities  map[string]domain.Activity
	enrollments []domain.ActivityEnrollment
	// The day each read was judged on, as the use case handed it in.
	updateToday   time.Time
	billableToday time.Time
}

func newFakeActivityRepo() *fakeActivityRepo {
	return &fakeActivityRepo{activities: map[string]domain.Activity{}}
}

func (f *fakeActivityRepo) nameTaken(a domain.Activity) bool {
	for id, other := range f.activities {
		if id != a.ID() && strings.EqualFold(other.Name(), a.Name()) {
			return true
		}
	}
	return false
}

func (f *fakeActivityRepo) CreateActivity(_ context.Context, a domain.Activity) (domain.Activity, error) {
	if f.nameTaken(a) {
		return domain.Activity{}, glerrors.New(glerrors.KindConflict, "name taken")
	}
	f.activities[a.ID()] = a
	return a, nil
}

func (f *fakeActivityRepo) UpdateActivity(_ context.Context, a domain.Activity, today time.Time) (domain.Activity, error) {
	f.updateToday = today
	if _, ok := f.activities[a.ID()]; !ok {
		return domain.Activity{}, glerrors.ErrNotFound
	}
	if f.nameTaken(a) {
		return domain.Activity{}, glerrors.New(glerrors.KindConflict, "name taken")
	}
	for _, e := range f.enrollments {
		if e.ActivityID() != a.ID() || (!e.EndsOn().IsZero() && !e.EndsOn().After(today)) {
			continue
		}
		if _, live := a.Option(e.OptionID()); !live {
			return domain.Activity{}, glerrors.New(glerrors.KindConflict, "option in use")
		}
	}
	f.activities[a.ID()] = a
	return a, nil
}

func (f *fakeActivityRepo) GetActivity(_ context.Context, id string) (domain.Activity, error) {
	a, ok := f.activities[id]
	if !ok {
		return domain.Activity{}, glerrors.ErrNotFound
	}
	return a, nil
}

func (f *fakeActivityRepo) ListActivities(_ context.Context, includeInactive bool) ([]domain.Activity, error) {
	var out []domain.Activity
	for _, a := range f.activities {
		if a.Active() || includeInactive {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (f *fakeActivityRepo) CountActiveEnrollments(_ context.Context, on time.Time) (map[string]int, error) {
	out := map[string]int{}
	for _, e := range f.enrollments {
		if e.ActiveOn(on) {
			out[e.ActivityID()]++
		}
	}
	return out, nil
}

func (f *fakeActivityRepo) ListActivityEnrollments(_ context.Context, activityID string) ([]domain.ActivityEnrollment, error) {
	var out []domain.ActivityEnrollment
	for _, e := range f.enrollments {
		if e.ActivityID() == activityID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeActivityRepo) ListBillableEnrollments(_ context.Context, today time.Time) ([]domain.ActivityEnrollment, error) {
	f.billableToday = today
	var out []domain.ActivityEnrollment
	for _, e := range f.enrollments {
		if a, ok := f.activities[e.ActivityID()]; ok && a.Active() {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeActivityRepo) CreateEnrollments(_ context.Context, es []domain.ActivityEnrollment) ([]domain.ActivityEnrollment, error) {
	var out []domain.ActivityEnrollment
	for _, e := range es {
		if f.openIndex(e.ActivityID(), e.StudentID()) >= 0 {
			continue
		}
		f.enrollments = append(f.enrollments, e)
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeActivityRepo) openIndex(activityID, studentID string) int {
	for i, e := range f.enrollments {
		if e.ActivityID() == activityID && e.StudentID() == studentID && e.EndsOn().IsZero() {
			return i
		}
	}
	return -1
}

func (f *fakeActivityRepo) EndEnrollments(_ context.Context, activityID string, studentIDs []string, on time.Time) (int, error) {
	ended := 0
	for _, sid := range studentIDs {
		for i, e := range f.enrollments {
			if e.ActivityID() == activityID && e.StudentID() == sid && e.ActiveOn(on) {
				f.enrollments[i] = e.End(on)
				ended++
			}
		}
	}
	return ended, nil
}

func (f *fakeActivityRepo) ChangeEnrollmentOption(_ context.Context, activityID, studentID, optionID string, on time.Time) (domain.ActivityEnrollment, error) {
	for i, e := range f.enrollments {
		if e.ActivityID() == activityID && e.StudentID() == studentID && e.ActiveOn(on) {
			moved, err := e.WithOption(optionID)
			if err != nil {
				return domain.ActivityEnrollment{}, err
			}
			f.enrollments[i] = moved
			return moved, nil
		}
	}
	return domain.ActivityEnrollment{}, glerrors.ErrNotFound
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
