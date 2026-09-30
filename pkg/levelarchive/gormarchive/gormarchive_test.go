package gormarchive

import (
	"strings"
	"testing"
)

func TestNewRefusesAnythingButAPlainIdentifier(t *testing.T) {
	cases := []struct {
		name  string
		table string
		ok    bool
	}{
		{"plain", "config_deletion_archive", true},
		{"digits", "t1", true},
		{"empty", "", false},
		{"quote", `x"; DROP TABLE y; --`, false},
		{"space", "a b", false},
		{"upper", "Archive", false},
		{"leading digit", "1abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tc.ok && r != nil {
					t.Fatalf("New(%q) panicked: %v", tc.table, r)
				}
				if !tc.ok && r == nil {
					t.Fatalf("New(%q) accepted an unsafe identifier", tc.table)
				}
			}()
			_ = New(tc.table)
		})
	}
}

func TestMoveSQLDeletesAndFilesInOneStatement(t *testing.T) {
	a := New("x_deletion_archive")
	sql := a.moveSQL(Step{Kind: "marks", Table: "attendance", Where: "group_id IN ?"})
	for _, want := range []string{
		"WITH moved AS (DELETE FROM attendance WHERE group_id IN ? RETURNING *)",
		"INSERT INTO x_deletion_archive",
		"to_jsonb(moved)",
		"current_setting('app.current_tenant', true)::uuid",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("moveSQL missing %q:\n%s", want, sql)
		}
	}
}

func TestReinsertSQLGuardsEveryParent(t *testing.T) {
	a := New("x_deletion_archive")
	sql := a.reinsertSQL("groups", []string{"id"}, Exclude{}, []Parent{{Column: "cycle_id", Table: "cycles"}, {Column: "grade_level_id", Table: "grade_levels", Nullable: true}})
	for _, want := range []string{
		"INSERT INTO groups SELECT r.* FROM x_deletion_archive a",
		"jsonb_populate_record(NULL::groups, a.\"row\") r",
		"EXISTS (SELECT 1 FROM cycles p WHERE p.id = r.cycle_id)",
		"(r.grade_level_id IS NULL OR EXISTS (SELECT 1 FROM grade_levels p WHERE p.id = r.grade_level_id))",
		"ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("reinsertSQL missing %q:\n%s", want, sql)
		}
	}
}

// Review finding 3: the archive loses only what went back — the DELETE is
// joined to the INSERT's RETURNING on the primary key, in the same statement.
func TestReinsertSQLClearsOnlyTheRowsItInserted(t *testing.T) {
	a := New("x_deletion_archive")
	sql := a.reinsertSQL("period_closures", []string{"tenant_id", "term_id"}, Exclude{}, nil)
	for _, want := range []string{
		"ON CONFLICT DO NOTHING RETURNING tenant_id, term_id)",
		"cleared AS (DELETE FROM x_deletion_archive a USING ins WHERE a.batch_id = ? AND a.table_name = ?",
		`a."row"->'tenant_id' = to_jsonb(ins.tenant_id)`,
		`a."row"->'term_id' = to_jsonb(ins.term_id)`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("reinsertSQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "table_name <>") {
		t.Errorf("reinsertSQL must never clear the whole batch:\n%s", sql)
	}
}

// Review finding 6: rows of excluded enrollments never enter the INSERT.
func TestReinsertSQLExcludesNamedRows(t *testing.T) {
	a := New("x_deletion_archive")
	sql := a.reinsertSQL("attendance", []string{"id"}, Exclude{Column: "enrollment_id", Values: []string{"e"}}, nil)
	if want := "(r.enrollment_id::text IS NULL OR r.enrollment_id::text NOT IN ?)"; !strings.Contains(sql, want) {
		t.Errorf("reinsertSQL missing %q:\n%s", want, sql)
	}
	js := a.reinsertSQL("grades_outbox", []string{"id"}, Exclude{Column: "payload", JSONKey: "enrollmentId", Values: []string{"e"}}, nil)
	if want := "((r.payload->>'enrollmentId') IS NULL OR (r.payload->>'enrollmentId') NOT IN ?)"; !strings.Contains(js, want) {
		t.Errorf("reinsertSQL missing %q:\n%s", want, js)
	}
	none := a.reinsertSQL("attendance", []string{"id"}, Exclude{Column: "enrollment_id"}, nil)
	if strings.Contains(none, "NOT IN") {
		t.Errorf("an empty exclusion must exclude nothing (NOT IN (NULL) would drop every row):\n%s", none)
	}
}

func TestStepRefusesUnsafeTableNames(t *testing.T) {
	a := New("x_deletion_archive")
	defer func() {
		if recover() == nil {
			t.Fatal("moveSQL accepted an unsafe table name")
		}
	}()
	_ = a.moveSQL(Step{Kind: "k", Table: "t; DROP", Where: "true"})
}
