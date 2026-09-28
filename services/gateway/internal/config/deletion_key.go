package config

import (
	"crypto/rand"
	"log/slog"
)

// minDeletionKeyBytes is the shortest HMAC key the deletion preview accepts.
const minDeletionKeyBytes = 32

// ParseDeletionTokenKey is the key that signs level-deletion previews (ADR
// 0021), from LEVEL_DELETION_TOKEN_KEY. Unset or shorter than 32 bytes, the
// gateway makes a random one for this process and says so: previews then do
// not survive a restart or cross instances — a confirmation lands as "the data
// changed, review again", never as a pass. The gateway runs min-instances=1.
func ParseDeletionTokenKey(getenv func(string) string, log *slog.Logger) []byte {
	if key := getenv("LEVEL_DELETION_TOKEN_KEY"); len(key) >= minDeletionKeyBytes {
		return []byte(key)
	}
	key := make([]byte, minDeletionKeyBytes)
	if _, err := rand.Read(key); err != nil {
		panic("gateway: no randomness for the deletion token key: " + err.Error())
	}
	log.Warn("LEVEL_DELETION_TOKEN_KEY unset or short; level-deletion previews use a per-process key")
	return key
}
