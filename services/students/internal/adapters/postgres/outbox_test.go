package postgres

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOutboxRetryDelayExponentialAndCapped(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: 30 * time.Second},
		{attempts: 2, want: time.Minute},
		{attempts: 3, want: 2 * time.Minute},
		{attempts: 7, want: 30 * time.Minute},
		{attempts: 20, want: 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.attempts), func(t *testing.T) {
			assert.Equal(t, tt.want, outboxRetryDelay(tt.attempts))
		})
	}
}
