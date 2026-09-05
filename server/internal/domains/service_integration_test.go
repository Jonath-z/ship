package domains

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestDomainCRUDIntegration(t *testing.T) {
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
	group := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "web"}
	server := migrations.Server{ID: uuid.NewString(), Name: "web-1", IPAddress: "203.0.113.10", SSHUser: "deploy", Resources: "{}"}
	serviceRow := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &group.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	otherService := migrations.Service{ID: uuid.NewString(), EnvironmentID: otherEnvironment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	for _, value := range []any{&project, &environment, &otherEnvironment, &group, &server, &serviceRow, &otherService} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := connection.ORM.Create(&migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: server.ID}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(NewRepository(connection.ORM), nil)
	created, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: serviceRow.ID, Hostname: " API.Example.COM. ", SSLEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Hostname != "api.example.com" || !created.SSLEnabled {
		t.Fatalf("created domain = %#v", created)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: serviceRow.ID, Hostname: created.Hostname,
	}); !errors.Is(err, ErrHostnameExists) {
		t.Fatalf("duplicate hostname error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: otherService.ID, Hostname: "other.example.com",
	}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("cross-environment service error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		ServiceID: serviceRow.ID, Hostname: "https://bad.example.com",
	}); err == nil {
		t.Fatal("expected malformed hostname to be rejected")
	}

	disabled := false
	updated, err := service.Update(ctx, RequestContext{}, project.ID, environment.ID, created.ID, UpdateInput{SSLEnabled: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SSLEnabled {
		t.Fatalf("updated domain = %#v", updated)
	}
	page, err := service.List(ctx, project.ID, environment.ID, "", 20)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("domain page = %#v, error = %v", page, err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, project.ID, environment.ID, created.ID); !errors.Is(err, ErrDomainNotFound) {
		t.Fatalf("deleted domain error = %v", err)
	}
}
