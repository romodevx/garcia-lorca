// Package levelarchive is the trash every data service keeps for level
// deletions (ADR 0021).
//
// A service does not hide deleted rows behind a deleted_at flag that every query
// would then have to remember. It MOVES them: inside one tenant-bound
// transaction each row is copied as JSONB into the service's own
// `<service>_deletion_archive` table and deleted from its home table, in a
// single statement (DELETE … RETURNING feeding the INSERT), so no row can be
// deleted without being filed. Restoring reinserts the JSONB with
// jsonb_populate_record, parents before children, skipping — and counting —
// what now conflicts. After the retention the archive is purged for good.
//
// Every statement runs on the caller's transaction, which must already have
// app.current_tenant bound (pkg/authctx): the archive's own tenant_id is read
// from that setting, so RLS on the archive table holds by construction.
//
// This package holds the framework-free half (counts, reports, the scope, the
// retention and the purge loop) so a service's domain and use cases can speak
// it; the GORM half that moves rows lives in levelarchive/gormarchive.
package levelarchive

import (
	"errors"
	"sort"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Retention is how long a deleted level stays restorable.
const Retention = 7 * 24 * time.Hour

// Cutoff is the instant before which an archive is past its retention.
func Cutoff(now time.Time) time.Time { return now.Add(-Retention) }

// Counts is rows per kind (e.g. "attendanceMarks": 1200).
type Counts map[string]int64

// Add merges other into c.
func (c Counts) Add(other Counts) {
	for k, v := range other {
		c[k] += v
	}
}

// KindCount is one entry of Counts, for wire formats that want an order.
type KindCount struct {
	Kind  string
	Count int64
}

// Sorted lists c by kind, so the same counts always serialize the same way.
func (c Counts) Sorted() []KindCount {
	out := make([]KindCount, 0, len(c))
	for k, v := range c {
		out = append(out, KindCount{Kind: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// Skip is rows of one kind a restore could not put back, and why.
type Skip struct {
	Kind   string `json:"kind"`
	Count  int64  `json:"count"`
	Reason string `json:"reason"`
}

// Report is what a restore put back and what it skipped.
type Report struct {
	Restored Counts `json:"restored"`
	Skipped  []Skip `json:"skipped"`
}

// NewReport is an empty report, ready to fill.
func NewReport() Report { return Report{Restored: Counts{}, Skipped: []Skip{}} }

// Record adds one kind's outcome: restored rows, and — when some were not —
// a skip with its reason.
func (r *Report) Record(kind string, restored, skipped int64, reason string) {
	r.Restored[kind] += restored
	if skipped > 0 {
		r.Skipped = append(r.Skipped, Skip{Kind: kind, Count: skipped, Reason: reason})
	}
}

// ErrBatchNotFound is a restore of a batch this service holds no archive for:
// never archived here, or already purged. The gateway turns it into
// TRASH_EXPIRED (or a partial-restore error), never a silent success.
var ErrBatchNotFound = glerrors.New(glerrors.KindNotFound, "este nivel ya no está en la papelera")

// CodeLevelBeingDeleted is the refusal code of any write that resolves a level
// while its deletion runs (the X-Error-Code the gateway forwards).
const CodeLevelBeingDeleted = "LEVEL_BEING_DELETED"

// CodedError is a typed error that also names a machine code for the screen.
// It unwraps to the *glerrors.Error, so KindOf and SafeMessage read it.
type CodedError struct {
	Code string
	err  *glerrors.Error
}

// Error implements error.
func (e *CodedError) Error() string { return e.err.Error() }

// Unwrap exposes the typed error.
func (e *CodedError) Unwrap() error { return e.err }

// LevelBeingDeleted refuses a write that resolves a level whose deletion is in
// flight (config's levels.status = deleting): a conflict, 409 at the edge.
func LevelBeingDeleted() error {
	return &CodedError{Code: CodeLevelBeingDeleted, err: glerrors.New(glerrors.KindConflict, "Este nivel se está eliminando")}
}

// CodeOf is err's machine code, or "" when it carries none.
func CodeOf(err error) string {
	var c *CodedError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}
