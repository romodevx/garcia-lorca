package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func (f *fakeCatalog) RenameCycle(_ context.Context, c domain.Cycle) error {
	old, ok := f.cycles[c.ID()]
	if !ok {
		return glerrors.ErrNotFound
	}
	if old.Name() != c.Name() {
		if err := f.claim("cycle|" + c.Name()); err != nil {
			return err
		}
		delete(f.unique, "cycle|"+old.Name())
	}
	f.cycleRenames++
	f.cycles[c.ID()] = c
	return nil
}

// CC-103: a draft year stayed forever under the name it was born with.
func TestRenameCycle(t *testing.T) {
	cases := map[string]struct {
		id, name string
		kind     glerrors.Kind
		code     domain.RefusalCode
		writes   int
	}{
		"renames and trims":       {name: "  Ciclo 2026-2027 ", writes: 1},
		"blank name":              {name: "  ", kind: glerrors.KindInvalid, code: domain.CodeCycleNameRequired},
		"a name another has":      {name: "2027-2028", kind: glerrors.KindConflict},
		"a cycle that is not one": {id: "nope", name: "x", kind: glerrors.KindNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo := newService(t)
			c := seedNamedCycle(t, svc, "2026-2027")
			seedNamedCycle(t, svc, "2027-2028")
			id := c.ID()
			if tc.id != "" {
				id = tc.id
			}

			got, err := svc.RenameCycle(ctx(), id, tc.name)

			assert.Equal(t, tc.writes, repo.cycleRenames)
			if tc.kind != glerrors.KindUnknown {
				assert.Equal(t, tc.kind, glerrors.KindOf(err))
				assert.Equal(t, tc.code, domain.RefusalCodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "Ciclo 2026-2027", got.Name())
			assert.Equal(t, c.Status(), got.Status())
			assert.Equal(t, "Ciclo 2026-2027", repo.cycles[c.ID()].Name())
		})
	}
}

// CC-101 through the use case: the repository's refusal reaches the caller.
func TestCreateCycleRefusesAnOverlappingYear(t *testing.T) {
	svc, repo := newService(t)
	seedNamedCycle(t, svc, "2026-2027")

	_, err := svc.CreateCycle(ctx(), "otro", domain.CycleStatusDraft, someDate.AddDate(0, 3, 0), laterDate.AddDate(0, 3, 0))

	assert.Equal(t, domain.CodeCycleOverlap, domain.RefusalCodeOf(err))
	assert.Len(t, repo.cycles, 1)
}
