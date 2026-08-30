package projects

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestProjectCRUDIntegration(t *testing.T) {
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

	service := NewService(NewRepository(connection.ORM), nil)
	first, err := service.Create(ctx, RequestContext{}, CreateInput{Name: " Acme API "})
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Acme API" || first.Slug != "acme-api" {
		t.Fatalf("created project = %#v", first)
	}
	second, err := service.Create(ctx, RequestContext{}, CreateInput{Name: "Back Office", Slug: "back-office"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, RequestContext{}, CreateInput{Name: "Duplicate", Slug: first.Slug}); !errors.Is(err, ErrSlugExists) {
		t.Fatalf("duplicate create error = %v", err)
	}

	pageOne, err := service.List(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageOne.Items) != 1 || pageOne.NextCursor == "" {
		t.Fatalf("first page = %#v", pageOne)
	}
	pageTwo, err := service.List(ctx, pageOne.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID == pageOne.Items[0].ID {
		t.Fatalf("second page = %#v", pageTwo)
	}

	renamed := "Acme Platform"
	updated, err := service.Update(ctx, RequestContext{}, first.ID, UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != renamed || updated.Slug != first.Slug {
		t.Fatalf("updated project = %#v", updated)
	}

	environment := migrations.Environment{
		ID: uuid.NewString(), ProjectID: first.ID, Name: "Production", Slug: "production",
	}
	if err := connection.ORM.Create(&environment).Error; err != nil {
		t.Fatal(err)
	}
	impact, err := service.DeletionImpact(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.Environments != 1 {
		t.Fatalf("deletion impact = %#v", impact)
	}
	if err := service.Delete(ctx, RequestContext{}, first.ID, "wrong"); !errors.Is(err, ErrConfirmationFailed) {
		t.Fatalf("wrong confirmation error = %v", err)
	}
	if err := service.Delete(ctx, RequestContext{}, first.ID, first.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, first.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("get deleted project error = %v", err)
	}
	var environmentCount int64
	if err := connection.ORM.Model(&migrations.Environment{}).Where("project_id = ?", first.ID).Count(&environmentCount).Error; err != nil {
		t.Fatal(err)
	}
	if environmentCount != 0 {
		t.Fatalf("cascaded environment count = %d", environmentCount)
	}
	if _, err := service.Get(ctx, second.ID); err != nil {
		t.Fatalf("unrelated project was removed: %v", err)
	}
}
