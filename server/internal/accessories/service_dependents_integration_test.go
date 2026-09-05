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

func TestAccessoryDeleteBlockedByDependents(t *testing.T) {
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
	api := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	postgres := migrations.Accessory{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "postgres", Type: "postgres", Image: "postgres:16"}
	edge := migrations.ServiceDependency{
		ID: uuid.NewString(), EnvironmentID: environment.ID,
		SourceServiceID: api.ID, TargetAccessoryID: &postgres.ID, Type: "runtime",
	}
	for _, value := range []any{&project, &environment, &api, &postgres, &edge} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(NewRepository(connection.ORM), nil, nil)
	err = service.Delete(ctx, RequestContext{}, project.ID, environment.ID, postgres.ID)
	var dependentsError *DependentsError
	if !errors.As(err, &dependentsError) || len(dependentsError.Names) != 1 || dependentsError.Names[0] != "api" {
		t.Fatalf("delete with dependents error = %v", err)
	}

	if err := connection.ORM.Delete(&edge).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, environment.ID, postgres.ID); err != nil {
		t.Fatalf("delete after removing edge error = %v", err)
	}
}
