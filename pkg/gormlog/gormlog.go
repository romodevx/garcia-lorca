// Package gormlog is the GORM logger every service opens its database with.
//
// GORM's default logger renders a logged query by interpolating its values, so
// a slow query over the pupils table wrote CURPs, names, phones and emails of
// minors into the logs (found by the legacy-import load test, ADR 0033). This
// logger keeps the query's shape and drops its values, and otherwise behaves
// like the one it replaces: warnings and slow queries only.
//
// Two layers, because GORM has two ways to render a query:
//   - ParamsFilter: the normal callbacks ask the logger for the values to
//     inline, and get none — the query is logged as "$1, $2…".
//   - Trace: Scan renders through an internal recorder that never asks, so the
//     SQL may arrive with the values already inlined; every string literal is
//     blanked to '?' before it is written.
package gormlog

import (
	"context"
	"regexp"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

// slowThreshold is when a query is reported as slow: GORM's own default.
const slowThreshold = 200 * time.Millisecond

// stringLiteral is a SQL string literal; a doubled quote inside it is an
// escaped quote, not its end.
var stringLiteral = regexp.MustCompile(`'(?:[^']|'')*'`)

// New returns the logger, writing to w (log.Default() in the services).
func New(w gormlogger.Writer) gormlogger.Interface {
	return redacting{gormlogger.New(w, gormlogger.Config{
		SlowThreshold:             slowThreshold,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
	})}
}

type redacting struct{ gormlogger.Interface }

// LogMode keeps the redaction when GORM derives a logger at another level.
func (r redacting) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return redacting{r.Interface.LogMode(level)}
}

// ParamsFilter hands GORM no values to inline (gorm.ParamsFilter).
func (redacting) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}

// Trace blanks every string literal of the SQL before it is written.
func (r redacting) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	r.Interface.Trace(ctx, begin, func() (string, int64) {
		sql, rows := fc()
		return stringLiteral.ReplaceAllString(sql, "'?'"), rows
	}, err)
}
