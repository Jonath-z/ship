package accessories

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestAccessoryCRUDIntegration(t *testing.T) {
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
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, servers CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, servers CASCADE")

	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	otherEnvironment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Staging", Slug: "staging"}
	server := migrations.Server{
		ID: uuid.NewString(), Name: "production-1", Hostname: "production-1.example.com", SSHUser: "deploy", Resources: "{}",
	}
	group := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "database"}
	otherGroup := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: otherEnvironment.ID, Name: "database"}
	for _, value := range []any{&project, &environment, &otherEnvironment, &server, &group, &otherGroup} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(NewRepository(connection.ORM), nil)
	postgres, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "Primary PostgreSQL", Type: "postgres", Image: "postgres:16",
	})
	if err != nil {
		t.Fatal(err)
	}
	if postgres.Port == nil || *postgres.Port != 5432 || postgres.SuggestedVolume == nil ||
		postgres.SuggestedVolume.Source != "primary_postgresql_data" || postgres.SuggestedConnectionSecret != "DATABASE_URL" {
		t.Fatalf("created postgres accessory = %#v", postgres)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: postgres.Name, Type: "postgres", Image: "postgres:16",
	}); !errors.Is(err, ErrNameExists) {
		t.Fatalf("duplicate accessory error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "MySQL", Type: "mysql", Image: "mysql:8",
	}); err == nil {
		t.Fatal("expected unsupported accessory type to be rejected")
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "Wrong group", Type: "redis", Image: "redis:7", ServerGroupID: &otherGroup.ID,
	}); !errors.Is(err, ErrPlacementNotFound) {
		t.Fatalf("cross-environment placement error = %v", err)
	}

	redis, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "Redis", Type: "redis", Image: "redis:7", ServerGroupID: &group.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Get(ctx, project.ID, environment.ID, redis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Role != group.Name || loaded.ServerGroupID == nil || *loaded.ServerGroupID != group.ID {
		t.Fatalf("loaded accessory = %#v", loaded)
	}
	updated, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, redis.ID, UpdateInput{
		PlacementSet: true, ServerID: &server.ID, PortSet: true, Port: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ServerID == nil || *updated.ServerID != server.ID || updated.ServerGroupID != nil || updated.Port != nil {
		t.Fatalf("updated accessory = %#v", updated)
	}
	cleared, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, redis.ID, UpdateInput{PlacementSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ServerID != nil || cleared.ServerGroupID != nil {
		t.Fatalf("cleared placement = %#v", cleared)
	}

	page, err := service.List(ctx, project.ID, environment.ID, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("accessory page = %#v, error = %v", page, err)
	}
	volume := migrations.Volume{
		ID: uuid.NewString(), EnvironmentID: environment.ID, AccessoryID: &postgres.ID,
		Name: "Postgres data", Source: "postgres_data", Destination: "/var/lib/postgresql/data",
	}
	if err := connection.ORM.Create(&volume).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, postgres.ID); err != nil {
		t.Fatal(err)
	}
	var volumeCount int64
	if err := connection.ORM.Model(&migrations.Volume{}).Where("id = ?", volume.ID).Count(&volumeCount).Error; err != nil {
		t.Fatal(err)
	}
	if volumeCount != 0 {
		t.Fatalf("accessory volume count after delete = %d", volumeCount)
	}
}
