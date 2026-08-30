package volumes

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestVolumeCRUDIntegration(t *testing.T) {
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
	if err := connection.ORM.Exec("TRUNCATE TABLE projects CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects CASCADE")

	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	otherEnvironment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Staging", Slug: "staging"}
	for _, value := range []any{&project, &environment, &otherEnvironment} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	ownerService := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "api", Type: "web", Image: "acme/api:latest",
	}
	otherService := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: otherEnvironment.ID, Name: "api", Type: "web", Image: "acme/api:latest",
	}
	accessory := migrations.Accessory{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "postgres", Type: "postgres", Image: "postgres:16",
	}
	for _, value := range []any{&ownerService, &otherService, &accessory} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(NewRepository(connection.ORM), nil)
	created, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: &ownerService.ID, Name: " App data ", Source: "app_data", Destination: "/var/lib/app/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Destination != "/var/lib/app" || created.ServiceID == nil || *created.ServiceID != ownerService.ID {
		t.Fatalf("created volume = %#v", created)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		AccessoryID: &accessory.ID, Name: "Duplicate", Source: created.Source, Destination: "/data",
	}); !errors.Is(err, ErrSourceExists) {
		t.Fatalf("duplicate source error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: &otherService.ID, Name: "Cross environment", Source: "cross_data", Destination: "/data",
	}); !errors.Is(err, ErrOwnerNotFound) {
		t.Fatalf("cross-environment owner error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: &ownerService.ID, AccessoryID: &accessory.ID,
		Name: "Two owners", Source: "two_owners", Destination: "/data",
	}); err == nil {
		t.Fatal("expected two owners to be rejected")
	}

	second, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		AccessoryID: &accessory.ID, Name: "Database data", Source: "postgres_data", Destination: "/var/lib/postgresql/data",
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(ctx, project.ID, environment.ID, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("volume page = %#v, error = %v", page, err)
	}
	newSource := "postgres_primary_data"
	updated, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, second.ID, UpdateInput{Source: &newSource})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source != newSource {
		t.Fatalf("updated volume = %#v", updated)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, project.ID, environment.ID, created.ID); !errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("get deleted volume error = %v", err)
	}
}
