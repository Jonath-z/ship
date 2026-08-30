package crypto

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestFileKeyProvider(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "keyring")
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	id := KeyID(key)
	contents := "active " + id + "\nkey " + id + " " + hex.EncodeToString(key) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	keyring, err := (FileKeyProvider{Path: path}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if keyring.Active != id || len(keyring.Keys[id]) != 32 {
		t.Fatalf("unexpected keyring: %#v", keyring)
	}
}

func TestEnvelopeRejectsTampering(t *testing.T) {
	key := make([]byte, 32)
	nonce, ciphertext, err := seal(key, []byte("secret"), []byte("context"))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[0] ^= 0xff
	if _, err := open(key, nonce, ciphertext, []byte("context")); err == nil {
		t.Fatal("expected tampered ciphertext to be rejected")
	}
}
