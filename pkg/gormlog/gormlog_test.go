package gormlog_test

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/gormlog"
)

// GORM renders a logged query by interpolating its values unless the logger
// filters them out. A slow query over the pupils table would otherwise write
// CURPs, names, phones and emails of minors into Cloud Logging.
func TestTheLoggerNeverHandsOverQueryValues(t *testing.T) {
	l := gormlog.New(log.New(&bytes.Buffer{}, "", 0))

	filter, ok := l.(gorm.ParamsFilter)
	if !ok {
		t.Fatal("the logger must filter query values (gorm.ParamsFilter)")
	}
	sql, vars := filter.ParamsFilter(context.Background(), "SELECT id FROM students WHERE curp = $1", "AAMI210219MJCLLRA0")
	if len(vars) != 0 {
		t.Errorf("vars = %v, want none: values must not reach the log", vars)
	}
	if strings.Contains(sql, "AAMI") {
		t.Errorf("sql = %q carries a value", sql)
	}
}

// Slow queries are still reported — with their shape, not their data.
func TestTheLoggerStillReportsSlowQueries(t *testing.T) {
	var out bytes.Buffer
	l := gormlog.New(log.New(&out, "", 0))

	l.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		return "SELECT id FROM students WHERE curp = $1", 1
	}, nil)

	if !strings.Contains(out.String(), "SLOW SQL") || !strings.Contains(out.String(), "curp = $1") {
		t.Errorf("log = %q, want the slow query reported by its shape", out.String())
	}
}

// GORM's Scan renders the query through an internal recorder that ignores the
// params filter, so what reaches Trace may already carry the values inlined.
// Every string literal is blanked before it is written.
func TestTheLoggerBlanksValuesAlreadyInlined(t *testing.T) {
	var out bytes.Buffer
	l := gormlog.New(log.New(&out, "", 0))

	l.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		return "SELECT s.id FROM students s WHERE s.curp IN ('AAMI210219MJCLLRA0','O''BRIEN') AND e.status = 'active'", 2
	}, nil)

	for _, leaked := range []string{"AAMI210219MJCLLRA0", "BRIEN", "active"} {
		if strings.Contains(out.String(), leaked) {
			t.Errorf("log carries %q: %s", leaked, out.String())
		}
	}
	if !strings.Contains(out.String(), "s.curp IN ('?','?')") {
		t.Errorf("log = %q, want the literals blanked in place", out.String())
	}
}
