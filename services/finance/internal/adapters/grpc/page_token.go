package grpcadapter

import (
	"encoding/base64"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// The page token is the wire form of a ports.ChargeCursor: "YYYY-MM-DD|<id>"
// in unpadded URL-safe base64, so it rides in a query string untouched. It is
// opaque to callers ON PURPOSE — they hand it back, never build or read it — so
// its shape can change without a contract change.
const (
	pageTokenDateLayout = "2006-01-02"
	pageTokenSeparator  = "|"
)

// errBadPageToken is the caller's mistake: a token this service did not mint.
var errBadPageToken = glerrors.New(glerrors.KindInvalid, "invalid page token")

// encodePageToken is the token of the page after `next`; empty when there is
// none.
func encodePageToken(next ports.ChargeCursor) string {
	if next.IsZero() {
		return ""
	}
	raw := next.DueDate.Format(pageTokenDateLayout) + pageTokenSeparator + next.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodePageToken is the cursor a token stands for; the zero cursor for an
// empty token (the first page).
func decodePageToken(token string) (ports.ChargeCursor, error) {
	if token == "" {
		return ports.ChargeCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ports.ChargeCursor{}, errBadPageToken
	}
	day, id, ok := strings.Cut(string(raw), pageTokenSeparator)
	if !ok || id == "" {
		return ports.ChargeCursor{}, errBadPageToken
	}
	due, err := time.Parse(pageTokenDateLayout, day)
	if err != nil {
		return ports.ChargeCursor{}, errBadPageToken
	}
	return ports.ChargeCursor{DueDate: due, ID: id}, nil
}
