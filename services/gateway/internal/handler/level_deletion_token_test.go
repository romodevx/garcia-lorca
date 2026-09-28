package handler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

func TestDeletionTokens(t *testing.T) {
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	now := at
	tokens := handler.NewDeletionTokens([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return now })
	counts := client.LevelCounts{"groups": 2, "students": 30}
	tok, exp := tokens.Issue("t1", "lvl-1", counts)
	assert.Equal(t, at.Add(10*time.Minute), exp, "a preview is good for ten minutes")

	cases := []struct {
		name   string
		token  string
		tenant string
		level  string
		counts client.LevelCounts
		after  time.Duration
		ok     bool
	}{
		{"same preview", tok, "t1", "lvl-1", client.LevelCounts{"students": 30, "groups": 2}, 0, true},
		{"last second of the window", tok, "t1", "lvl-1", counts, 10 * time.Minute, true},
		{"expired", tok, "t1", "lvl-1", counts, 10*time.Minute + time.Second, false},
		{"a count changed", tok, "t1", "lvl-1", client.LevelCounts{"groups": 2, "students": 31}, 0, false},
		{"a kind appeared", tok, "t1", "lvl-1", client.LevelCounts{"groups": 2, "students": 30, "reportCards": 1}, 0, false},
		{"zero-count kinds do not matter", tok, "t1", "lvl-1", client.LevelCounts{"groups": 2, "students": 30, "reportCards": 0}, 0, true},
		{"another level", tok, "t1", "lvl-2", counts, 0, false},
		{"another school", tok, "t2", "lvl-1", counts, 0, false},
		{"tampered", tok + "x", "t1", "lvl-1", counts, 0, false},
		{"garbage", "nope", "t1", "lvl-1", counts, 0, false},
		{"empty", "", "t1", "lvl-1", counts, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now = at.Add(tc.after)
			assert.Equal(t, tc.ok, tokens.Verify(tc.token, tc.tenant, tc.level, tc.counts))
		})
	}
}

func TestDeletionTokensFromAnotherKeyAreRefused(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC) }
	a := handler.NewDeletionTokens([]byte("0123456789abcdef0123456789abcdef"), now)
	b := handler.NewDeletionTokens([]byte("fedcba9876543210fedcba9876543210"), now)
	tok, _ := a.Issue("t1", "lvl-1", client.LevelCounts{"groups": 1})
	assert.False(t, b.Verify(tok, "t1", "lvl-1", client.LevelCounts{"groups": 1}))
}

// Review finding 5: a token opens one saga. Two previews of the same data in
// the same second must still be two tokens, or a used token would stale an
// honest new preview (config stores the hash and refuses a reuse).
func TestDeletionTokensAreUniquePerPreview(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC) }
	tokens := handler.NewDeletionTokens([]byte("0123456789abcdef0123456789abcdef"), now)
	counts := client.LevelCounts{"groups": 1}
	a, _ := tokens.Issue("t1", "lvl-1", counts)
	b, _ := tokens.Issue("t1", "lvl-1", counts)
	assert.NotEqual(t, a, b)
	assert.True(t, tokens.Verify(a, "t1", "lvl-1", counts))
	assert.True(t, tokens.Verify(b, "t1", "lvl-1", counts))
	assert.Regexp(t, `^[0-9a-f]{64}$`, handler.TokenHash(a))
	assert.NotEqual(t, handler.TokenHash(a), handler.TokenHash(b))

	stamp, rest, _ := strings.Cut(a, ".")
	_, sig, _ := strings.Cut(rest, ".")
	assert.False(t, tokens.Verify(stamp+".AAAAAAAAAAAAAAAAAAAAAA."+sig, "t1", "lvl-1", counts), "the nonce is signed")
}
