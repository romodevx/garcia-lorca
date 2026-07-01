package bcrypt_test

import (
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
)

func TestHashAndVerifyRoundTrip(t *testing.T) {
	h := bcrypt.NewHasher(0) // 0 → default cost

	hash, err := h.Hash("s3cret-pass")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "s3cret-pass" {
		t.Fatal("hash must not equal the plaintext")
	}
	if err := h.Verify(hash, "s3cret-pass"); err != nil {
		t.Errorf("Verify(correct) = %v, want nil", err)
	}
}

func TestVerifyWrongPasswordIsUnauthorized(t *testing.T) {
	h := bcrypt.NewHasher(0)
	hash, _ := h.Hash("correct")
	err := h.Verify(hash, "wrong")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("Verify(wrong) kind = %v, want KindUnauthorized", err)
	}
}

func TestHashIsSaltedUnique(t *testing.T) {
	h := bcrypt.NewHasher(0)
	a, _ := h.Hash("same")
	b, _ := h.Hash("same")
	if a == b {
		t.Error("two hashes of the same password should differ (random salt)")
	}
}

func TestHashRejectsTooLongInput(t *testing.T) {
	h := bcrypt.NewHasher(0)
	// bcrypt refuses inputs longer than 72 bytes.
	_, err := h.Hash(strings.Repeat("x", 100))
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("Hash(too long) kind = %v, want KindInvalid", err)
	}
}
