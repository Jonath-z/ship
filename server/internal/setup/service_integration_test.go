package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestSetupSingleUseIntegration(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL is not set")
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
	if err := connection.ORM.WithContext(ctx).Where("1 = 1").Delete(&migrations.User{}).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = connection.ORM.WithContext(context.Background()).Where("1 = 1").Delete(&migrations.User{}).Error
	}()

	token := "64f9a1a2f39836f3fc72384f6cb286f555e9fc46c9cab98b"
	sum := sha256.Sum256([]byte(token))
	service := NewService(connection.ORM, hex.EncodeToString(sum[:]), "ship.example.com")

	status, err := service.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Required {
		t.Fatal("expected setup to be required before an owner exists")
	}
	owner, err := service.CreateOwner(ctx, token, "OWNER@Example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if owner.Email != "owner@example.com" || owner.Role != "owner" {
		t.Fatalf("unexpected owner: %#v", owner)
	}
	if _, err := service.CreateOwner(ctx, token, "second@example.com", "a sufficiently long password"); !errors.Is(err, ErrAlreadyComplete) {
		t.Fatalf("expected setup to be single-use, got %v", err)
	}
}
