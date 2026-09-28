package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
)

// LevelDeletionStatus is where a deleted level stands in the trash.
type LevelDeletionStatus string

// A batch starts deleted and ends restored (by the director) or purged (by the
// retention sweep). Neither end goes back.
const (
	LevelDeletionDeleted  LevelDeletionStatus = "deleted"
	LevelDeletionRestored LevelDeletionStatus = "restored"
	LevelDeletionPurged   LevelDeletionStatus = "purged"
)

// The refusal codes of the level trash (ADR 0021).
const (
	CodeLevelNameTaken RefusalCode = "LEVEL_NAME_TAKEN"
	CodeTrashExpired   RefusalCode = "TRASH_EXPIRED"
)

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

// CheckRestorable refuses a batch that is no longer in the trash (NotFound) or
// whose window has closed (TRASH_EXPIRED). The last instant of the window
// still counts.
func (d LevelDeletion) CheckRestorable(now time.Time) error {
	if d.Status != LevelDeletionDeleted {
		return glerrors.ErrNotFound
	}
	if now.After(d.RestorableUntil) {
		return refusal(glerrors.KindForbidden, CodeTrashExpired,
			"Pasaron más de 7 días; este nivel ya no se puede restaurar")
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
