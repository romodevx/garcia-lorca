package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var issued = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// Task 13: the card's QR carries an opaque token and nothing else — no name, no
// CURP, no matrícula, no URL. 128 random bits, URL-safe, with a prefix that
// says what it is.
func TestACardCodeIsOpaque(t *testing.T) {
	var random [16]byte
	for i := range random {
		random[i] = byte(i * 17)
	}
	c, err := domain.NewCardToken("ct-1", "t1", "stu-1", random, issued)
	if err != nil {
		t.Fatalf("NewCardToken: %v", err)
	}
	code := c.Code()
	if !strings.HasPrefix(code, "GLCARD:") || len(strings.TrimPrefix(code, "GLCARD:")) != 22 {
		t.Errorf("code = %q, want GLCARD: and 22 url-safe characters", code)
	}
	if strings.ContainsAny(strings.TrimPrefix(code, "GLCARD:"), "+/=") {
		t.Errorf("code %q is not url-safe", code)
	}

	token, err := domain.ParseCardCode("  " + code + "\n")
	if err != nil || token != c.Token() {
		t.Errorf("ParseCardCode(code) = %q, %v; want the token back", token, err)
	}
}

func TestParseCardCodeRefusesAnythingElse(t *testing.T) {
	for _, code := range []string{"", "GLCARD:", "GLCARD:short", "OTHER:AAAAAAAAAAAAAAAAAAAAAA", "GLCARD:AAAAAAAAAAAAAAAAAAAA/+", "https://x"} {
		if _, err := domain.ParseCardCode(code); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("ParseCardCode(%q) = %v, want KindInvalid", code, err)
		}
	}
}

func TestARevokedCardIsNotActive(t *testing.T) {
	c, err := domain.NewCardToken("ct-1", "t1", "stu-1", [16]byte{1}, issued)
	if err != nil {
		t.Fatalf("NewCardToken: %v", err)
	}
	if !c.Active() {
		t.Fatal("a new card must be active")
	}
	if c.Revoke(issued.Add(time.Hour)).Active() {
		t.Error("a revoked card is still active")
	}
}
