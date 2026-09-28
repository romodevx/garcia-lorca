// Package gormarchive is the GORM half of gormarchive: it moves a level's rows
// into a service's `<service>_deletion_archive` table and back (ADR 0021).
//
// Table and column names are interpolated into SQL, so they are checked against
// a plain-identifier shape and panic otherwise: they are compile-time constants
// of the calling service, never input.
package gormarchive

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// markerKind is the archive row that records a batch's lifecycle (its counts
// and, once restored, its report). One per batch; it makes every call
// idempotent per batch_id.
const markerKind = "_batch"

// markerPosition sorts the marker before every data row.
const markerPosition = -1

var identifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func mustIdent(name string) string {
	if !identifier.MatchString(name) {
		panic(fmt.Sprintf("gormarchive: %q is not a plain SQL identifier", name))
	}
	return name
}

// Step is one table's slice of a deletion: the rows of Table matching Where.
// Where is a predicate over Table's own columns with ? placeholders for Args.
type Step struct {
	Kind  string
	Table string
	Where string
	Args  []any
}

// Parent is a row a restored row must still find: Column of the restored row
// must name an existing Table.id. Nullable columns pass when NULL.
type Parent struct {
	Column   string
	Table    string
	Nullable bool
}

// Archive is one service's deletion archive table.
type Archive struct{ table string }

// New names a service's archive table (e.g. "config_deletion_archive").
func New(table string) Archive { return Archive{table: mustIdent(table)} }

// Table is the archive's table name, for a service's own restore SQL.
func (a Archive) Table() string { return a.table }

// Count counts the rows a step would move, changing nothing.
func Count(tx *gorm.DB, s Step) (int64, error) {
	var n int64
	sql := "SELECT count(*) FROM " + mustIdent(s.Table) + " WHERE " + s.Where
	if err := tx.Raw(sql, s.Args...).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("gormarchive: counting %s: %w", s.Table, err)
	}
	return n, nil
}

func (a Archive) moveSQL(s Step) string {
	return "WITH moved AS (DELETE FROM " + mustIdent(s.Table) + " WHERE " + s.Where + " RETURNING *) " +
		"INSERT INTO " + a.table + ` (tenant_id, batch_id, table_name, "row", "position") ` +
		"SELECT current_setting('app.current_tenant', true)::uuid, ?, ?, to_jsonb(moved), ? FROM moved"
}

// Move files the rows of a step in the archive under batchID and deletes them,
// in one statement. It answers how many rows moved.
func (a Archive) Move(tx *gorm.DB, batchID string, position int, s Step) (int64, error) {
	args := append(append([]any{}, s.Args...), batchID, s.Table, position)
	res := tx.Exec(a.moveSQL(s), args...)
	if res.Error != nil {
		return 0, fmt.Errorf("gormarchive: moving %s: %w", s.Table, res.Error)
	}
	return res.RowsAffected, nil
}

// Snapshot files the current value of some columns of the rows a query selects
// WITHOUT deleting them — for a deletion that only clears a reference (a pupil's
// level). The query must select the row's id and the columns to restore; kind
// names the snapshot in the archive.
func (a Archive) Snapshot(tx *gorm.DB, batchID string, position int, kind, query string, args ...any) (int64, error) {
	sql := "INSERT INTO " + a.table + ` (tenant_id, batch_id, table_name, "row", "position") ` +
		"SELECT current_setting('app.current_tenant', true)::uuid, ?, ?, to_jsonb(s), ? FROM (" + query + ") s"
	all := append([]any{batchID, kind, position}, args...)
	// The query's own placeholders come AFTER the three in the SELECT list.
	res := tx.Exec(sql, all...)
	if res.Error != nil {
		return 0, fmt.Errorf("gormarchive: snapshot %s: %w", kind, res.Error)
	}
	return res.RowsAffected, nil
}

func (a Archive) reinsertSQL(table string, parents []Parent) string {
	var b strings.Builder
	t := mustIdent(table)
	b.WriteString("INSERT INTO " + t + " SELECT r.* FROM " + a.table + " a ")
	b.WriteString(`CROSS JOIN LATERAL jsonb_populate_record(NULL::` + t + `, a."row") r `)
	b.WriteString("WHERE a.batch_id = ? AND a.table_name = ?")
	for _, p := range parents {
		exists := "EXISTS (SELECT 1 FROM " + mustIdent(p.Table) + " p WHERE p.id = r." + mustIdent(p.Column) + ")"
		if p.Nullable {
			exists = "(r." + p.Column + " IS NULL OR " + exists + ")"
		}
		b.WriteString(" AND " + exists)
	}
	b.WriteString(` ORDER BY a."position", a.id ON CONFLICT DO NOTHING`)
	return b.String()
}

// Archived counts the rows of one kind a batch holds.
func (a Archive) Archived(tx *gorm.DB, batchID, kind string) (int64, error) {
	var n int64
	err := tx.Raw("SELECT count(*) FROM "+a.table+" WHERE batch_id = ? AND table_name = ?", batchID, kind).Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("gormarchive: counting archived %s: %w", kind, err)
	}
	return n, nil
}

// Reinsert puts a table's archived rows back. A row that now conflicts with a
// unique key, or whose parent no longer exists, is skipped. It answers how many
// went back and how many were skipped.
func (a Archive) Reinsert(tx *gorm.DB, batchID, table string, parents ...Parent) (restored, skipped int64, err error) {
	total, err := a.Archived(tx, batchID, table)
	if err != nil || total == 0 {
		return 0, 0, err
	}
	res := tx.Exec(a.reinsertSQL(table, parents), batchID, table)
	if res.Error != nil {
		return 0, 0, fmt.Errorf("gormarchive: restoring %s: %w", table, res.Error)
	}
	return res.RowsAffected, total - res.RowsAffected, nil
}

// marker is the batch's lifecycle row.
type marker struct {
	Counts   levelarchive.Counts  `json:"counts"`
	Restored bool                 `json:"restored"`
	Report   *levelarchive.Report `json:"report,omitempty"`
}

// lock serializes every call on one batch: two retries of the same saga step
// cannot interleave.
func (a Archive) lock(tx *gorm.DB, batchID string) error {
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", a.table+":"+batchID).Error; err != nil {
		return fmt.Errorf("gormarchive: locking batch: %w", err)
	}
	return nil
}

func (a Archive) readMarker(tx *gorm.DB, batchID string) (marker, bool, error) {
	var raw []string
	err := tx.Raw(`SELECT "row"::text FROM `+a.table+" WHERE batch_id = ? AND table_name = ?", batchID, markerKind).
		Scan(&raw).Error
	if err != nil {
		return marker{}, false, fmt.Errorf("gormarchive: reading batch: %w", err)
	}
	if len(raw) == 0 {
		return marker{}, false, nil
	}
	var m marker
	if err := json.Unmarshal([]byte(raw[0]), &m); err != nil {
		return marker{}, false, fmt.Errorf("gormarchive: decoding batch: %w", err)
	}
	return m, true, nil
}

func (a Archive) writeMarker(tx *gorm.DB, batchID string, m marker) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("gormarchive: encoding batch: %w", err)
	}
	if err := tx.Exec("DELETE FROM "+a.table+" WHERE batch_id = ? AND table_name = ?", batchID, markerKind).Error; err != nil {
		return fmt.Errorf("gormarchive: replacing batch: %w", err)
	}
	err = tx.Exec("INSERT INTO "+a.table+` (tenant_id, batch_id, table_name, "row", "position") `+
		"VALUES (current_setting('app.current_tenant', true)::uuid, ?, ?, ?::jsonb, ?)",
		batchID, markerKind, string(body), markerPosition).Error
	if err != nil {
		return fmt.Errorf("gormarchive: writing batch: %w", err)
	}
	return nil
}

// ArchiveBatch runs a service's archive once per batch. The first call runs fn
// and records its counts; a repeated call (a saga retry) answers the recorded
// counts without touching anything.
func (a Archive) ArchiveBatch(tx *gorm.DB, batchID string, fn func(tx *gorm.DB) (levelarchive.Counts, error)) (levelarchive.Counts, error) {
	if err := a.lock(tx, batchID); err != nil {
		return nil, err
	}
	m, found, err := a.readMarker(tx, batchID)
	if err != nil {
		return nil, err
	}
	if found {
		return m.Counts, nil
	}
	counts, err := fn(tx)
	if err != nil {
		return nil, err
	}
	if err := a.writeMarker(tx, batchID, marker{Counts: counts}); err != nil {
		return nil, err
	}
	return counts, nil
}

// RestoreBatch runs a service's restore once per batch. An unknown batch (its
// archive never committed, or was purged) restores nothing; a restored one
// answers its first report. After a restore the batch's data rows leave the
// archive and only the marker stays, until the purge.
func (a Archive) RestoreBatch(tx *gorm.DB, batchID string, fn func(tx *gorm.DB) (levelarchive.Report, error)) (levelarchive.Report, error) {
	if err := a.lock(tx, batchID); err != nil {
		return levelarchive.Report{}, err
	}
	m, found, err := a.readMarker(tx, batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	if !found {
		return levelarchive.NewReport(), nil
	}
	if m.Restored && m.Report != nil {
		return *m.Report, nil
	}
	rep, err := fn(tx)
	if err != nil {
		return levelarchive.Report{}, err
	}
	if err := tx.Exec("DELETE FROM "+a.table+" WHERE batch_id = ? AND table_name <> ?", batchID, markerKind).Error; err != nil {
		return levelarchive.Report{}, fmt.Errorf("gormarchive: clearing restored batch: %w", err)
	}
	m.Restored, m.Report = true, &rep
	if err := a.writeMarker(tx, batchID, m); err != nil {
		return levelarchive.Report{}, err
	}
	return rep, nil
}

// Purge hard-deletes every archive row filed before cutoff, markers included.
func (a Archive) Purge(tx *gorm.DB, cutoff time.Time) (int64, error) {
	res := tx.Exec("DELETE FROM "+a.table+" WHERE created_at < ?", cutoff)
	if res.Error != nil {
		return 0, fmt.Errorf("gormarchive: purging: %w", res.Error)
	}
	return res.RowsAffected, nil
}
