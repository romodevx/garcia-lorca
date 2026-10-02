//go:build integration

package postgres_test

import (
	"testing"

	"gorm.io/gorm"
)

const migration0016 = "0016_evaluation_periods.sql"

// The fixture of migration 0016: one cycle (24 Aug 2026 – 9 Jul 2027), a level
// with two grades and its three trimesters mapped to months, plus a second
// level whose terms do not fit cleanly.
const (
	m16Cycle    = "16000000-0000-0000-0000-000000000001"
	m16Level    = "16000000-0000-0000-0000-000000000002"
	m16Grade1   = "16000000-0000-0000-0000-000000000003"
	m16Grade2   = "16000000-0000-0000-0000-000000000004"
	m16T1       = "16000000-0000-0000-0000-000000000011"
	m16T2       = "16000000-0000-0000-0000-000000000012"
	m16T3       = "16000000-0000-0000-0000-000000000013"
	m16Odd      = "16000000-0000-0000-0000-000000000020"
	m16OddGrade = "16000000-0000-0000-0000-000000000021"
	m16OddT1    = "16000000-0000-0000-0000-000000000022"
	m16OddT2    = "16000000-0000-0000-0000-000000000023"
	m16OddT3    = "16000000-0000-0000-0000-000000000024"
)

func exec16(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func seed0016(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec16(t, db, `INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on)
		VALUES (?, ?, '2026-2027', 'active', '2026-08-24', '2027-07-09')`, m16Cycle, schoolTenant)
	for id, name := range map[string]string{m16Level: "PRIMARIA", m16Odd: "SECUNDARIA"} {
		exec16(t, db, `INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES (?, ?, ?, 'CYCLE')`, id, schoolTenant, name)
	}
	exec16(t, db, `INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES (?, ?, ?, '1°', 1), (?, ?, ?, '2°', 2), (?, ?, ?, '1° sec', 1)`,
		m16Grade1, schoolTenant, m16Level, m16Grade2, schoolTenant, m16Level, m16OddGrade, schoolTenant, m16Odd)
	term := func(id, level, name string, ordinal int, from, to string, months ...string) {
		exec16(t, db, `INSERT INTO terms (id, tenant_id, cycle_id, level_id, name, ordinal, starts_on, ends_on) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, schoolTenant, m16Cycle, level, name, ordinal, from, to)
		for _, m := range months {
			exec16(t, db, `INSERT INTO term_months (tenant_id, term_id, cycle_id, level_id, month) VALUES (?, ?, ?, ?, ?)`,
				schoolTenant, id, m16Cycle, level, m)
		}
	}
	// PRIMARIA: the local data at T14 — clean trimesters by month.
	term(m16T1, m16Level, "1.er trimestre", 1, "2026-08-24", "2026-12-08", "2026-08-01", "2026-09-01", "2026-10-01", "2026-11-01")
	term(m16T2, m16Level, "2.º trimestre", 2, "2026-12-09", "2027-03-24", "2026-12-01", "2027-01-01", "2027-02-01", "2027-03-01")
	term(m16T3, m16Level, "3.er trimestre", 3, "2027-03-25", "2027-07-09", "2027-04-01", "2027-05-01", "2027-06-01", "2027-07-01")
	// SECUNDARIA: T1 holds September and November, T2 October — their spans
	// interleave, so only the first can become a period. T3 has no months yet:
	// its own dates stand in.
	term(m16OddT1, m16Odd, "Bimestre 1", 1, "2026-09-01", "2026-11-30", "2026-09-01", "2026-11-01")
	term(m16OddT2, m16Odd, "Bimestre 2", 2, "2026-10-01", "2026-10-31", "2026-10-01")
	term(m16OddT3, m16Odd, "Bimestre 3", 3, "2027-01-04", "2027-08-30")
}

type periodRow struct {
	GradeLevelID string
	Ordinal      int
	Name         string
	StartsOn     string
	EndsOn       string
	TermID       *string
}

func periodsOf(t *testing.T, db *gorm.DB, grade string) []periodRow {
	t.Helper()
	var rows []periodRow
	if err := db.Raw(`SELECT grade_level_id, ordinal, name, to_char(starts_on, 'YYYY-MM-DD') AS starts_on,
			to_char(ends_on, 'YYYY-MM-DD') AS ends_on, term_id::text AS term_id
		FROM evaluation_periods WHERE grade_level_id = ? ORDER BY ordinal`, grade).Scan(&rows).Error; err != nil {
		t.Fatalf("read periods: %v", err)
	}
	return rows
}

// Every term of a level becomes the same period in every grade of the level:
// first day of its first month to last day of its last month, clamped to the
// cycle, linked to the term. Nothing is deleted.
func TestMigration0016DerivesEveryGradesPeriodsFromItsLevelsTerms(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_periods", "0016")
	seed0016(t, db)

	applyMigration(t, db, migration0016)

	want := []struct {
		ordinal              int
		name, from, to, term string
	}{
		{1, "1.er trimestre", "2026-08-24", "2026-11-30", m16T1}, // August clamped to the cycle's first day
		{2, "2.º trimestre", "2026-12-01", "2027-03-31", m16T2},
		{3, "3.er trimestre", "2027-04-01", "2027-07-09", m16T3}, // July clamped to the cycle's last day
	}
	for _, grade := range []string{m16Grade1, m16Grade2} {
		got := periodsOf(t, db, grade)
		if len(got) != len(want) {
			t.Fatalf("grade %s: %d periods, want %d: %+v", grade, len(got), len(want), got)
		}
		for i, w := range want {
			g := got[i]
			if g.Ordinal != w.ordinal || g.Name != w.name || g.StartsOn != w.from || g.EndsOn != w.to || g.TermID == nil || *g.TermID != w.term {
				t.Errorf("grade %s period %d = %+v, want %+v", grade, w.ordinal, g, w)
			}
		}
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM terms"); n != 6 {
		t.Errorf("terms are kept, have %d", n)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM term_months"); n != 15 {
		t.Errorf("term months are kept, have %d", n)
	}
}

// Interleaved months cannot become two periods of one grade: the earlier
// ordinal wins, the other is left for the school to enter, and the migration
// does not stop. A term with no months uses its own dates, clamped.
func TestMigration0016SkipsWhatWouldOverlapAndFallsBackToTheTermsDates(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_periods_odd", "0016")
	seed0016(t, db)

	applyMigration(t, db, migration0016)

	got := periodsOf(t, db, m16OddGrade)
	if len(got) != 2 {
		t.Fatalf("want bimestre 1 and 3 only, got %+v", got)
	}
	if got[0].Ordinal != 1 || got[0].StartsOn != "2026-09-01" || got[0].EndsOn != "2026-11-30" {
		t.Errorf("bimestre 1 = %+v", got[0])
	}
	if got[1].Ordinal != 3 || got[1].StartsOn != "2027-01-04" || got[1].EndsOn != "2027-07-09" {
		t.Errorf("bimestre 3 (no months: its own dates, clamped) = %+v", got[1])
	}
}

// The database itself keeps a grade's periods apart, also against writers that
// skip the use case, and keeps each period's dates in order.
func TestMigration0016ConstraintsRefuseOverlapsAndBackwardDates(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_periods_ck", "0016")
	seed0016(t, db)
	applyMigration(t, db, migration0016)

	insert := func(grade string, ordinal int, from, to string) error {
		return db.Exec(`INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on)
			VALUES (?, ?, ?, ?, 'X', ?, ?)`, schoolTenant, m16Cycle, grade, ordinal, from, to).Error
	}
	if err := insert(m16Grade1, 4, "2027-07-09", "2027-07-09"); err == nil {
		t.Error("sharing the last day of the 3rd trimester must be refused (inclusive ranges)")
	}
	if err := insert(m16Grade1, 3, "2026-06-01", "2026-06-30"); err == nil {
		t.Error("an ordinal twice in one grade must be refused")
	}
	if err := insert(m16Grade1, 5, "2026-06-30", "2026-06-01"); err == nil {
		t.Error("a period ending before it starts must be refused")
	}
	if err := insert(m16OddGrade, 2, "2026-12-01", "2026-12-31"); err != nil {
		t.Errorf("a free gap of another grade is fine: %v", err)
	}
	if !hasConstraint(t, db, "evaluation_periods_no_overlap") {
		t.Error("the EXCLUDE constraint must exist by name (the adapter maps it)")
	}
}
