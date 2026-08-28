package dbpool

import (
	"database/sql"
	"testing"
)

func TestSettingsFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantOpen int
		wantIdle int
	}{
		{
			name:     "defaults are small on purpose",
			env:      nil,
			wantOpen: defaultMaxOpenConns,
			wantIdle: defaultMaxIdleConns,
		},
		{
			name:     "prod can raise the ceiling",
			env:      map[string]string{envMaxOpen: "25", envMaxIdle: "5"},
			wantOpen: 25,
			wantIdle: 5,
		},
		{
			name:     "garbage falls back to the default instead of zero",
			env:      map[string]string{envMaxOpen: "not-a-number"},
			wantOpen: defaultMaxOpenConns,
			wantIdle: defaultMaxIdleConns,
		},
		{
			name:     "a non-positive value is refused: 0 means UNLIMITED to database/sql",
			env:      map[string]string{envMaxOpen: "0"},
			wantOpen: defaultMaxOpenConns,
			wantIdle: defaultMaxIdleConns,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := settingsFromEnv()
			if got.maxOpen != tt.wantOpen {
				t.Errorf("maxOpen = %d, want %d", got.maxOpen, tt.wantOpen)
			}
			if got.maxIdle != tt.wantIdle {
				t.Errorf("maxIdle = %d, want %d", got.maxIdle, tt.wantIdle)
			}
		})
	}
}

// TestTuneBoundsThePool is the regression that matters: Go's database/sql
// defaults to UNLIMITED open connections. Nine services on Cloud Run, each
// allowed up to three instances, against a small managed Postgres is how you get
// intermittent "remaining connection slots are reserved" under the seed and then
// again under a director clicking around.
func TestTuneBoundsThePool(t *testing.T) {
	db, err := sql.Open("nonexistent-driver-is-fine", "unused")
	if err != nil {
		// sql.Open only validates the driver lazily on some versions; if it does
		// error here the test has nothing to assert against.
		t.Skipf("sql.Open unavailable in this environment: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	Tune(db)

	if got := db.Stats().MaxOpenConnections; got != defaultMaxOpenConns {
		t.Errorf("MaxOpenConnections = %d, want %d (unlimited is the bug)", got, defaultMaxOpenConns)
	}
}
