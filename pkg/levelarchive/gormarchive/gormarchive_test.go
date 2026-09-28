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
	sql := a.reinsertSQL("groups", []Parent{{Column: "cycle_id", Table: "cycles"}, {Column: "grade_level_id", Table: "grade_levels", Nullable: true}})
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

func TestStepRefusesUnsafeTableNames(t *testing.T) {
	a := New("x_deletion_archive")
	defer func() {
		if recover() == nil {
			t.Fatal("moveSQL accepted an unsafe table name")
		}
	}()
	_ = a.moveSQL(Step{Kind: "k", Table: "t; DROP", Where: "true"})
}
