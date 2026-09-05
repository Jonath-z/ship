package dependencies

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestDependencyIntegration(t *testing.T) {
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
	api := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	worker := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "worker", Type: "worker", Image: "acme/worker:latest"}
	frontend := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "frontend", Type: "web", Image: "acme/frontend:latest"}
	foreignService := migrations.Service{ID: uuid.NewString(), EnvironmentID: otherEnvironment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	postgres := migrations.Accessory{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "postgres", Type: "postgres", Image: "postgres:16"}
	for _, value := range []any{&project, &environment, &otherEnvironment, &api, &worker, &frontend, &foreignService, &postgres} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(NewRepository(connection.ORM), nil)

	// frontend -> api -> worker, plus api -> postgres.
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: frontend.ID, TargetServiceID: &api.ID,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: api.ID, TargetServiceID: &worker.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: api.ID, TargetAccessoryID: &postgres.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: api.ID, TargetServiceID: &worker.ID,
	}); !errors.Is(err, ErrDependencyExists) {
		t.Fatalf("duplicate edge error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: api.ID, TargetServiceID: &api.ID,
	}); !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("self-dependency error = %v", err)
	}
	// worker -> frontend would close frontend -> api -> worker -> frontend.
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: worker.ID, TargetServiceID: &frontend.ID,
	}); !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("transitive cycle error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: foreignService.ID, TargetServiceID: &api.ID,
	}); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("cross-environment source error = %v", err)
	}
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: api.ID, TargetServiceID: &foreignService.ID,
	}); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("cross-environment target error = %v", err)
	}

	page, err := service.List(ctx, project.ID, environment.ID, "", 2)
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("dependency page = %#v, error = %v", page, err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, created.ID); !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("delete missing dependency error = %v", err)
	}
	// After removing api -> worker the previously cyclic edge is allowed.
	if _, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, CreateInput{
		SourceServiceID: worker.ID, TargetServiceID: &frontend.ID,
	}); err != nil {
		t.Fatalf("edge after cycle removal error = %v", err)
	}
}
