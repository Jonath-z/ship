package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	first, err := HashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected independent salts")
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected hash: %q", first)
	}
	valid, err := VerifyPassword("a sufficiently long password", first)
	if err != nil || !valid {
		t.Fatalf("expected password to verify: valid=%v err=%v", valid, err)
	}
	valid, err = VerifyPassword("wrong password", first)
	if err != nil || valid {
		t.Fatalf("expected wrong password to fail: valid=%v err=%v", valid, err)
	}
	if PasswordNeedsRehash(first) {
		t.Fatal("new password hash should not need rehashing")
	}
}

func TestPasswordHashParserRejectsExcessiveWork(t *testing.T) {
	encoded := "$argon2id$v=19$m=4294967295,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U"
	if _, err := VerifyPassword("password", encoded); err == nil {
		t.Fatal("expected unreasonable parameters to be rejected")
	}
}
