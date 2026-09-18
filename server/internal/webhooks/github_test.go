package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestSignatureValid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	if !signatureValid(body, sign(body, "hunter2"), "hunter2") {
		t.Fatal("valid signature rejected")
	}
	for name, signature := range map[string]string{
		"wrong secret":   sign(body, "other"),
		"missing prefix": "deadbeef",
		"empty":          "",
	} {
		if signatureValid(body, signature, "hunter2") {
			t.Fatalf("%s accepted", name)
		}
	}
	if signatureValid(body, sign(body, ""), "") {
		t.Fatal("empty secret must never verify")
	}
}

func TestRepositoryMatches(t *testing.T) {
	matching := []string{
		"github.com/acme/webapp",
		"https://github.com/acme/webapp",
		"https://github.com/acme/webapp.git",
		"https://github.com/ACME/WebApp",
	}
	for _, stored := range matching {
		if !repositoryMatches(stored, "acme/webapp") {
			t.Errorf("%q should match acme/webapp", stored)
		}
	}
	nonMatching := []string{
		"gitlab.com/acme/webapp",       // wrong host
		"github.com/acme/other",        // wrong repository
		"git@github.com:acme/webapp",   // rejected by normalization
		"github.com/acme/webapp/extra", // wrong path depth
		"",
	}
	for _, stored := range nonMatching {
		if repositoryMatches(stored, "acme/webapp") {
			t.Errorf("%q should not match acme/webapp", stored)
		}
	}
}
