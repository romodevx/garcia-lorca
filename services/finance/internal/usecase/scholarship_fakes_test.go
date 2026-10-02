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

var _ ports.ScholarshipRepository = (*fakeScholarshipRepo)(nil)

// fakeScholarshipRepo keeps scholarships and holdings in memory with the
// database's rules — one name per school (case-insensitive), one open holding
// per pupil and scholarship — and freezes onto the ledger fake it shares.
type fakeScholarshipRepo struct {
	ledger       *fakeRepo
	scholarships map[string]domain.Scholarship
	holdings     []domain.ScholarshipHolding
}

func newFakeScholarshipRepo(ledger *fakeRepo) *fakeScholarshipRepo {
	return &fakeScholarshipRepo{ledger: ledger, scholarships: map[string]domain.Scholarship{}}
}

func (f *fakeScholarshipRepo) nameTaken(s domain.Scholarship) bool {
	for id, other := range f.scholarships {
		if id != s.ID() && strings.EqualFold(other.Name(), s.Name()) {
			return true
		}
	}
	return false
}

func (f *fakeScholarshipRepo) CreateScholarship(_ context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	if f.nameTaken(s) {
		return domain.Scholarship{}, glerrors.New(glerrors.KindConflict, "name taken")
	}
	f.scholarships[s.ID()] = s
	return s, nil
}

func (f *fakeScholarshipRepo) UpdateScholarship(_ context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	if _, ok := f.scholarships[s.ID()]; !ok {
		return domain.Scholarship{}, glerrors.ErrNotFound
	}
	if f.nameTaken(s) {
		return domain.Scholarship{}, glerrors.New(glerrors.KindConflict, "name taken")
	}
	f.scholarships[s.ID()] = s
	return s, nil
}

func (f *fakeScholarshipRepo) GetScholarship(_ context.Context, id string) (domain.Scholarship, error) {
	s, ok := f.scholarships[id]
	if !ok {
		return domain.Scholarship{}, glerrors.ErrNotFound
	}
	return s, nil
}

func (f *fakeScholarshipRepo) ListScholarships(_ context.Context, includeInactive bool) ([]domain.Scholarship, error) {
	var out []domain.Scholarship
	for _, s := range f.scholarships {
		if s.Active() || includeInactive {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (f *fakeScholarshipRepo) CountHolders(_ context.Context) (map[string]int, error) {
	out := map[string]int{}
	for _, h := range f.holdings {
		if h.EndsOn.IsZero() {
			out[h.ScholarshipID]++
		}
	}
	return out, nil
}

func (f *fakeScholarshipRepo) ListHolders(_ context.Context, scholarshipID string) ([]domain.ScholarshipHolding, error) {
	var out []domain.ScholarshipHolding
	for _, h := range f.holdings {
		if h.ScholarshipID == scholarshipID && h.EndsOn.IsZero() {
			out = append(out, h)
		}
	}
	return out, nil
}

func (f *fakeScholarshipRepo) HeldBy(_ context.Context, studentIDs []string) (map[string][]domain.Scholarship, error) {
	wanted := map[string]bool{}
	for _, id := range studentIDs {
		wanted[id] = true
	}
	out := map[string][]domain.Scholarship{}
	for _, h := range f.holdings {
		if !h.EndsOn.IsZero() || (studentIDs != nil && !wanted[h.StudentID]) {
			continue
		}
		out[h.StudentID] = append(out[h.StudentID], f.scholarships[h.ScholarshipID])
	}
	return out, nil
}

func (f *fakeScholarshipRepo) Assign(_ context.Context, hs []domain.ScholarshipHolding) ([]domain.ScholarshipHolding, error) {
	var written []domain.ScholarshipHolding
	for _, h := range hs {
		held := false
		for _, existing := range f.holdings {
			if existing.EndsOn.IsZero() && existing.ScholarshipID == h.ScholarshipID && existing.StudentID == h.StudentID {
				held = true
			}
		}
		if !held {
			f.holdings = append(f.holdings, h)
			written = append(written, h)
		}
	}
	return written, nil
}

func (f *fakeScholarshipRepo) Unassign(_ context.Context, scholarshipID string, studentIDs []string, on time.Time) (int, error) {
	n := 0
	for i, h := range f.holdings {
		if h.ScholarshipID != scholarshipID || !h.EndsOn.IsZero() {
			continue
		}
		for _, id := range studentIDs {
			if h.StudentID == id {
				f.holdings[i].EndsOn = on
				n++
			}
		}
	}
	return n, nil
}

func (f *fakeScholarshipRepo) FreezeScholarships(_ context.Context, charges []domain.Charge) (int, error) {
	n := 0
	for _, c := range charges {
		stored, ok := f.ledger.charges[c.ID()]
		if !ok || stored.Status() == domain.StatusCancelled || stored.Scholarship() > 0 {
			continue
		}
		frozen, err := stored.WithScholarship(c.ScholarshipID(), c.Scholarship().Int64(), f.ledger.paidFor(c.ID()))
		if err != nil {
			return n, err
		}
		f.ledger.charges[c.ID()] = frozen
		n++
	}
	return n, nil
}
