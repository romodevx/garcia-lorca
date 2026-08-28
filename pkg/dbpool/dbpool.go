// Package dbpool bounds a service's database connection pool.
//
// Go's database/sql opens an UNLIMITED number of connections by default. That is
// invisible on a laptop and expensive in production: nine Cloud Run services,
// each allowed several instances, all pointed at one small managed Postgres,
// will exhaust its connection slots. The failure is intermittent and ugly —
// "FATAL: remaining connection slots are reserved" — and it shows up first under
// a parallel seed and later under a user simply clicking around.
//
// Deliberately takes *sql.DB rather than a *gorm.DB: this package has no reason
// to know which ORM the caller uses, and the repo keeps GORM out of shared code.
package dbpool

import (
	"database/sql"
	"os"
	"strconv"
	"time"
)

const (
	envMaxOpen = "DB_MAX_OPEN_CONNS"
	envMaxIdle = "DB_MAX_IDLE_CONNS"

	// Small on purpose. Each service is one bounded context serving a school, not
	// a high-throughput API, and the demo environment runs on a free-tier
	// Postgres. Raise via env in an environment that has the headroom.
	defaultMaxOpenConns = 3
	defaultMaxIdleConns = 1

	// Idle connections are returned promptly so a scaled-to-zero fleet does not
	// sit on slots it is not using.
	connMaxIdleTime = time.Minute
	// Recycling connections keeps things healthy behind a pooler (Neon/pgbouncer)
	// and survives a managed instance being moved underneath us.
	connMaxLifetime = 30 * time.Minute
)

type settings struct {
	maxOpen int
	maxIdle int
}

// intFromEnv returns the positive integer in key, or fallback. A non-positive
// value is REFUSED rather than honoured: database/sql reads 0 as "unlimited",
// so a typo would silently restore the exact behaviour this package exists to
// prevent.
func intFromEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func settingsFromEnv() settings {
	return settings{
		maxOpen: intFromEnv(envMaxOpen, defaultMaxOpenConns),
		maxIdle: intFromEnv(envMaxIdle, defaultMaxIdleConns),
	}
}

// Tune applies the bounded pool configuration to db.
func Tune(db *sql.DB) {
	s := settingsFromEnv()
	db.SetMaxOpenConns(s.maxOpen)
	db.SetMaxIdleConns(s.maxIdle)
	db.SetConnMaxIdleTime(connMaxIdleTime)
	db.SetConnMaxLifetime(connMaxLifetime)
}
