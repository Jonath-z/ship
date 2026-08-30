package services

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestServiceCRUDIntegration(t *testing.T) {
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
	if err := connection.ORM.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.ORM.Create(&environment).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(connection.ORM), nil)
	port := 3000
	api, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: " API ", Type: "web", Repository: "https://example.com/acme/api.git", Port: &port,
	})
	if err != nil {
		t.Fatal(err)
	}
	if api.Name != "API" || api.Branch != "main" || api.Role != "web" || api.ServerGroupID == "" {
		t.Fatalf("created service = %#v", api)
	}
	worker, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "Worker", Type: "worker", Image: "ghcr.io/acme/worker:latest", Role: "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if worker.Branch != "" || worker.Role != "worker" {
		t.Fatalf("image service = %#v", worker)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "API", Type: "web", Image: "ghcr.io/acme/api:latest",
	}); !errors.Is(err, ErrNameExists) {
		t.Fatalf("duplicate service error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		Name: "Invalid", Type: "web",
	}); err == nil {
		t.Fatal("expected service without repository or image to be rejected")
	}

	pageOne, err := service.List(ctx, project.ID, environment.ID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageOne.Items) != 1 || pageOne.NextCursor == "" {
		t.Fatalf("first page = %#v", pageOne)
	}
	pageTwo, err := service.List(ctx, project.ID, environment.ID, pageOne.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID == pageOne.Items[0].ID {
		t.Fatalf("second page = %#v", pageTwo)
	}

	empty := ""
	image := "ghcr.io/acme/api:v1"
	role := "backend"
	updated, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, api.ID, UpdateInput{
		Repository: &empty, Image: &image, PortSet: true, Port: nil, Role: &role,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Repository != "" || updated.Branch != "" || updated.Image != image || updated.Port != nil || updated.Role != role {
		t.Fatalf("updated service = %#v", updated)
	}
	if _, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, api.ID, UpdateInput{Image: &empty}); err == nil {
		t.Fatal("expected clearing the last service source to fail")
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, api.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, project.ID, environment.ID, api.ID); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("get deleted service error = %v", err)
	}
}
