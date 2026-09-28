package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// deletionTokenTTL is how long a deletion preview may be confirmed.
const deletionTokenTTL = 10 * time.Minute

// DeletionTokens signs a level-deletion preview (ADR 0021): what the director
// saw — this school, this level, these counts — until when. Confirming checks
// the signature against a FRESH preview, so anything that changed in between
// (a grade captured, a pupil enrolled) makes the confirmation stale.
//
// The counts are not carried in the token, only signed: the server recomputes
// them and compares by signature, so a client can neither read nor edit what
// it is confirming. Kinds with a zero count are left out of the signature, so a
// service that starts reporting a new, empty kind does not stale every token.
type DeletionTokens struct {
	key []byte
	now func() time.Time
}

// NewDeletionTokens builds a signer over key (HMAC-SHA256).
func NewDeletionTokens(key []byte, now func() time.Time) *DeletionTokens {
	return &DeletionTokens{key: key, now: now}
}

// Issue signs a preview; it answers the token and when it expires.
func (t *DeletionTokens) Issue(tenantID, levelID string, counts client.LevelCounts) (string, time.Time) {
	exp := t.now().UTC().Add(deletionTokenTTL).Truncate(time.Second)
	stamp := strconv.FormatInt(exp.Unix(), 10)
	mac := t.sign(tenantID, levelID, counts, stamp)
	return stamp + "." + base64.RawURLEncoding.EncodeToString(mac), exp
}

// Verify reports whether token signs exactly this tenant, level and counts and
// has not expired. The last second of the window still counts.
func (t *DeletionTokens) Verify(token, tenantID, levelID string, counts client.LevelCounts) bool {
	stamp, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || t.now().UTC().After(time.Unix(exp, 0)) {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	return hmac.Equal(got, t.sign(tenantID, levelID, counts, stamp))
}

func (t *DeletionTokens) sign(tenantID, levelID string, counts client.LevelCounts, stamp string) []byte {
	kinds := make([]string, 0, len(counts))
	for k, v := range counts {
		if v != 0 {
			kinds = append(kinds, k)
		}
	}
	sort.Strings(kinds)
	var b strings.Builder
	b.WriteString("level-deletion/v1\n" + tenantID + "\n" + levelID + "\n" + stamp + "\n")
	for _, k := range kinds {
		b.WriteString(k + "=" + strconv.FormatInt(counts[k], 10) + "\n")
	}
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(b.String()))
	return mac.Sum(nil)
}
