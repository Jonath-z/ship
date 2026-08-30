package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestValidToken(t *testing.T) {
	token := "this-is-a-long-random-first-run-token"
	sum := sha256.Sum256([]byte(token))
	expected := hex.EncodeToString(sum[:])

	if !validToken(token, expected) {
		t.Fatal("expected the matching token to pass")
	}
	if validToken(token+"x", expected) {
		t.Fatal("expected a different token to fail")
	}
	if validToken(token, "") {
		t.Fatal("expected an unconfigured token to fail")
	}
}

func TestHashPasswordUsesArgon2idAndRandomSalt(t *testing.T) {
	first, err := hashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected password hash format: %q", first)
	}
	if first == second {
		t.Fatal("expected independently salted hashes")
	}
}
