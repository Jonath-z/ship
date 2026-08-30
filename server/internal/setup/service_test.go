package setup

import (
	"crypto/sha256"
	"encoding/hex"
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
