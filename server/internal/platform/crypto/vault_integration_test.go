package crypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestVaultRawDatabaseAndOnlineRotationIntegration(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	connection, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.ORM.Exec("TRUNCATE TABLE vault_entries, audit_logs CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE vault_entries, audit_logs CASCADE")

	oldKey := bytes.Repeat([]byte{0x11}, 32)
	newKey := bytes.Repeat([]byte{0x22}, 32)
	oldID, newID := KeyID(oldKey), KeyID(newKey)
	keyringPath := filepath.Join(t.TempDir(), "keyring")
	writeTestKeyring(t, keyringPath, oldID, map[string][]byte{oldID: oldKey})
	provider := FileKeyProvider{Path: keyringPath}
	vault := NewVault(connection.ORM, provider, audit.NewService(connection.ORM))
	scopeID, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("postgres://ship:top-secret-password@database/production")
	id, err := vault.Store(ctx, StoreInput{
		Kind: KindApplicationSecret, ScopeType: "environment", ScopeID: scopeID,
		Name: "DATABASE_URL", Plaintext: plaintext,
	})
	if err != nil {
		t.Fatal(err)
	}

	var raw migrations.VaultEntry
	if err := connection.ORM.First(&raw, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string][]byte{
		"ciphertext": raw.Ciphertext, "wrapped data key": raw.WrappedDEK,
	} {
		if bytes.Contains(value, plaintext) || bytes.Contains(value, []byte("top-secret-password")) {
			t.Fatalf("%s exposed plaintext in the database", name)
		}
	}

	writeTestKeyring(t, keyringPath, newID, map[string][]byte{oldID: oldKey, newID: newKey})
	result, err := vault.Rotate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rewrapped != 1 || result.ActiveKeyID != newID {
		t.Fatalf("unexpected rotation result: %#v", result)
	}
	writeTestKeyring(t, keyringPath, newID, map[string][]byte{newID: newKey})
	revealed, err := vault.Reveal(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(revealed, plaintext) {
		t.Fatal("rotated ciphertext did not decrypt to the original value")
	}
	if err := connection.ORM.First(&raw, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if raw.KeyID != newID {
		t.Fatalf("database retained old key ID %q", raw.KeyID)
	}
}

func writeTestKeyring(t *testing.T, path, active string, keys map[string][]byte) {
	t.Helper()
	contents := "active " + active + "\n"
	for id, key := range keys {
		contents += "key " + id + " " + hex.EncodeToString(key) + "\n"
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
