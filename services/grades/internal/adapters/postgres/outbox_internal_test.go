package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryDelayDoublesUpToTheCap(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{6, 16 * time.Minute},
		{7, 30 * time.Minute},
		{50, 30 * time.Minute},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, retryDelay(tt.attempts), "attempts=%d", tt.attempts)
	}
}
