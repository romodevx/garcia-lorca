package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
)

const (
	batch = "aaaaaaaa-0000-0000-0000-00000000000b"
	level = "aaaaaaaa-0000-0000-0000-00000000000c"
	user  = "aaaaaaaa-0000-0000-0000-00000000000d"
)

func TestNewLevelDeletion(t *testing.T) {
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		batch   string
		level   string
		lname   string
		by      string
		byName  string
		wantErr bool
	}{
		{"valid", batch, level, "Primaria", user, "Laura", false},
		{"blank level name", batch, level, "  ", user, "Laura", true},
		{"blank deleter name", batch, level, "Primaria", user, " ", true},
		{"bad batch", "x", level, "Primaria", user, "Laura", true},
		{"bad level", batch, "x", "Primaria", user, "Laura", true},
		{"bad deleter", batch, level, "Primaria", "x", "Laura", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := domain.NewLevelDeletion(tc.batch, tc.level, tc.lname, tc.by, tc.byName, levelarchive.Counts{"groups": 2}, at)
			if tc.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, domain.LevelDeletionDeleted, d.Status)
			assert.Equal(t, at.Add(7*24*time.Hour), d.RestorableUntil, "restorable for exactly the retention")
			assert.Equal(t, int64(2), d.Counts["groups"])
		})
	}
}

func TestLevelDeletionRestorableUntilTheWindowCloses(t *testing.T) {
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	d, err := domain.NewLevelDeletion(batch, level, "Primaria", user, "Laura", nil, at)
	require.NoError(t, err)

	assert.NoError(t, d.CheckRestorable(at.Add(time.Hour)))
	assert.NoError(t, d.CheckRestorable(d.RestorableUntil), "the last instant still counts")

	err = d.CheckRestorable(d.RestorableUntil.Add(time.Second))
	assert.Equal(t, domain.CodeTrashExpired, domain.RefusalCodeOf(err))

	d.Status = domain.LevelDeletionRestored
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(d.CheckRestorable(at)), "a restored batch is no longer in the trash")
}

func TestLevelNameTakenIsACodedConflict(t *testing.T) {
	err := domain.LevelNameTaken()
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeLevelNameTaken, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ya existe un nivel con ese nombre; renómbralo antes de restaurar", glerrors.SafeMessage(err))
}

func TestLevelDeletionRestorableStatuses(t *testing.T) {
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		status     domain.LevelDeletionStatus
		restorable bool
		inTrash    bool
	}{
		{domain.LevelDeletionDeleting, true, false}, // compensation undoes a running saga
		{domain.LevelDeletionDeleted, true, true},
		{domain.LevelDeletionFailedPartial, true, true},
		{domain.LevelDeletionRestoredPartial, true, true}, // a retry finishes it
		{domain.LevelDeletionRestored, false, false},
		{domain.LevelDeletionCancelled, false, false},
		{domain.LevelDeletionPurged, false, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			d, err := domain.NewLevelDeletion(batch, level, "Primaria", user, "Laura", nil, at)
			require.NoError(t, err)
			d.Status = tc.status
			err = d.CheckRestorable(at)
			if tc.restorable {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
			}
			assert.Equal(t, tc.inTrash, d.Status.InTrash())
		})
	}
}

func TestLevelDeletionRefusalsAreCodedConflicts(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code domain.RefusalCode
	}{
		{domain.DeletionInProgress(), domain.CodeDeletionInProgress},
		{domain.PreviewTokenUsed(), domain.CodePreviewStale},
	} {
		assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(tc.err))
		assert.Equal(t, tc.code, domain.RefusalCodeOf(tc.err))
		assert.NotEmpty(t, glerrors.SafeMessage(tc.err))
	}
}

func TestLevelDeletionStepsAreEverySagaStep(t *testing.T) {
	assert.Equal(t, []string{"grades", "attendance", "finance", "uniforms", "students", "config"}, domain.LevelDeletionSteps)
}
