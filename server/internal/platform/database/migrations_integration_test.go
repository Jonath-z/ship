package database

import (
	"context"
	"os"
	"testing"
)

func TestMigrationsUpAndDown(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := MigrateDown(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	version, err := MigrationVersion(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("expected migration version 1, got %d", version)
	}
	if err := MigrateDown(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
}
