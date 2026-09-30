package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
)

// LevelDeletionStatus is where a deleted level stands in the trash.
type LevelDeletionStatus string

// A batch starts deleting (BeginLevelDeletion, the saga's lock) and becomes
// deleted when every step archived. A failed saga ends cancelled (fully undone)
// or failed_partial (not fully: in the trash, restorable). A restore ends
// restored, or restored_partial when some rows did not fit (still in the trash,
// a retry finishes it). A restore in flight is restoring (it remembers where
// it came from, and falls back there if it fails). The retention sweep ends
// everything purged — never a restoring batch.
const (
	LevelDeletionDeleting        LevelDeletionStatus = "deleting"
	LevelDeletionDeleted         LevelDeletionStatus = "deleted"
	LevelDeletionFailedPartial   LevelDeletionStatus = "failed_partial"
	LevelDeletionRestoredPartial LevelDeletionStatus = "restored_partial"
	LevelDeletionRestoring       LevelDeletionStatus = "restoring"
	LevelDeletionRestored        LevelDeletionStatus = "restored"
	LevelDeletionCancelled       LevelDeletionStatus = "cancelled"
	LevelDeletionPurged          LevelDeletionStatus = "purged"
)

// InTrash reports whether the director sees the batch in the trash.
func (s LevelDeletionStatus) InTrash() bool {
	return s == LevelDeletionDeleted || s == LevelDeletionFailedPartial || s == LevelDeletionRestoredPartial
}

// restorable reports whether a restore (or a compensation) may run on it.
func (s LevelDeletionStatus) restorable() bool {
	return s == LevelDeletionDeleting || s == LevelDeletionRestoring || s.InTrash()
}

// LevelDeletionSteps are the saga's steps, in deletion order. A saga that died
// mid-way leaves every one of them pending.
var LevelDeletionSteps = []string{"grades", "attendance", "finance", "uniforms", "students", "config"}

// SagaStallAfter is how long a batch may sit in `deleting` before the sweep
// takes the saga for dead (a gateway that crashed mid-way): far beyond the
// saga's and its compensation's timeouts.
const SagaStallAfter = 30 * time.Minute

// RestoreStallAfter is how long a batch may sit in `restoring` before the
// sweep takes the restore for dead and puts the batch back where it was: far
// beyond a restore's own timeout.
const RestoreStallAfter = 30 * time.Minute

// The refusal codes of the level trash (ADR 0021).
const (
	CodeLevelNameTaken     RefusalCode = "LEVEL_NAME_TAKEN"
	CodeTrashExpired       RefusalCode = "TRASH_EXPIRED"
	CodeDeletionInProgress RefusalCode = "DELETION_IN_PROGRESS"
	CodePreviewStale       RefusalCode = "PREVIEW_STALE"
	CodeRestoreInProgress  RefusalCode = "RESTORE_IN_PROGRESS"
)

// RestoreInProgress refuses a second restore of a batch whose restore runs.
func RestoreInProgress() error {
	return refusal(glerrors.KindConflict, CodeRestoreInProgress,
		"Ya se está restaurando este nivel; espera un momento y revisa la Papelera.")
}

// DeletionInProgress refuses a second deletion of a level whose deletion runs
// (or failed half-way and waits in the trash).
func DeletionInProgress() error {
	return refusal(glerrors.KindConflict, CodeDeletionInProgress,
		"Ya se está eliminando este nivel; espera un momento y revisa la Papelera.")
}

// PreviewTokenUsed refuses a confirm token that already opened a saga.
func PreviewTokenUsed() error {
	return refusal(glerrors.KindConflict, CodePreviewStale, "Los datos cambiaron; revisa el resumen de nuevo")
}

// maxDeletionNameRunes bounds the names a batch records (the DDL CHECK).
const maxDeletionNameRunes = 200

// LevelDeletion is one deleted level in the trash: what it was, what it held,
// who deleted it and until when it can come back.
type LevelDeletion struct {
	BatchID         string
	LevelID         string
	LevelName       string
	Counts          levelarchive.Counts
	DeletedBy       string
	DeletedByName   string
	DeletedAt       time.Time
	RestorableUntil time.Time
	Status          LevelDeletionStatus
	// PendingSteps (failed_partial) are the steps whose archives may still hold
	// rows of the batch; a restore asks exactly these.
	PendingSteps []string
	// Skipped is what a compensation or a restore could not put back.
	Skipped []levelarchive.Skip
	// RestoringFrom (restoring only) is the status the batch had when its
	// restore began, and falls back to.
	RestoringFrom LevelDeletionStatus
}

// NewLevelDeletion records a fresh deletion. It stays restorable for exactly
// levelarchive.Retention from the instant it happened.
func NewLevelDeletion(batchID, levelID, levelName, deletedBy, deletedByName string, counts levelarchive.Counts, at time.Time) (LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return LevelDeletion{}, err
	}
	lvl, err := levelarchive.NormalizeID(levelID, "level id")
	if err != nil {
		return LevelDeletion{}, err
	}
	by, err := levelarchive.NormalizeID(deletedBy, "deleted by")
	if err != nil {
		return LevelDeletion{}, err
	}
	name, byName := strings.TrimSpace(levelName), strings.TrimSpace(deletedByName)
	if name == "" || byName == "" {
		return LevelDeletion{}, glerrors.New(glerrors.KindInvalid, "a deletion needs the level's and the deleter's names")
	}
	if counts == nil {
		counts = levelarchive.Counts{}
	}
	return LevelDeletion{
		BatchID: batch, LevelID: lvl, LevelName: clampRunes(name, maxDeletionNameRunes),
		Counts: counts, DeletedBy: by, DeletedByName: clampRunes(byName, maxDeletionNameRunes),
		DeletedAt: at, RestorableUntil: at.Add(levelarchive.Retention), Status: LevelDeletionDeleted,
	}, nil
}

// CheckRestorable refuses a batch that is no longer restorable (NotFound) or
// whose window has closed (TRASH_EXPIRED). The last instant of the window
// still counts. now is the database's clock (the one restorable_until was
// written with).
func (d LevelDeletion) CheckRestorable(now time.Time) error {
	if !d.Status.restorable() {
		return glerrors.ErrNotFound
	}
	if d.Expired(now) {
		return TrashExpired()
	}
	return nil
}

// Expired reports whether the batch's window has closed at now (the database's
// clock). The last instant of the window still counts.
func (d LevelDeletion) Expired(now time.Time) bool { return now.After(d.RestorableUntil) }

// TrashExpired refuses a restore past the 7-day window.
func TrashExpired() error {
	return refusal(glerrors.KindForbidden, CodeTrashExpired,
		"Pasaron más de 7 días; este nivel ya no se puede restaurar")
}

// CheckRestoreStart refuses to start a restore: a batch whose restore already
// runs (RESTORE_IN_PROGRESS), one not in the trash (NotFound) or past its
// window (TRASH_EXPIRED).
func (d LevelDeletion) CheckRestoreStart(now time.Time) error {
	switch {
	case d.Status == LevelDeletionRestoring:
		return RestoreInProgress()
	case !d.Status.InTrash():
		return glerrors.ErrNotFound
	case d.Expired(now):
		return TrashExpired()
	}
	return nil
}

// LevelNameTaken refuses a restore whose level name another level now uses.
func LevelNameTaken() error {
	return refusal(glerrors.KindConflict, CodeLevelNameTaken,
		"Ya existe un nivel con ese nombre; renómbralo antes de restaurar")
}

func clampRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
