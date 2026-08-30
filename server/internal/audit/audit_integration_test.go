package audit

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestAuditLogIsAppendOnlyAndFilterableIntegration(t *testing.T) {
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
	if err := connection.ORM.Exec("TRUNCATE TABLE audit_logs CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE audit_logs CASCADE")

	service := NewService(connection.ORM)
	if err := service.Record(ctx, Event{
		Action: "user.created", ResourceType: "user", ResourceID: "fixture",
		Outcome: OutcomeSuccess, Metadata: map[string]any{"role": "viewer"},
	}); err != nil {
		t.Fatal(err)
	}
	page, err := service.List(ctx, Filters{Action: "user.created", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Metadata["role"] != "viewer" {
		t.Fatalf("unexpected audit page: %#v", page)
	}

	var row migrations.AuditLog
	if err := connection.ORM.First(&row, "id = ?", page.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.ORM.Model(&row).Update("action", "tampered").Error; !errors.Is(err, migrations.ErrAuditLogImmutable) {
		t.Fatalf("expected update to be rejected, got %v", err)
	}
	if err := connection.ORM.Delete(&row).Error; !errors.Is(err, migrations.ErrAuditLogImmutable) {
		t.Fatalf("expected delete to be rejected, got %v", err)
	}
}
