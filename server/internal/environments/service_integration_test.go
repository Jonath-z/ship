package environments

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestEnvironmentCRUDAndCloneIntegration(t *testing.T) {
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
	if err := connection.ORM.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(connection.ORM), nil)
	source, err := service.Create(ctx, RequestContext{}, project.ID, CreateInput{Name: " Production "})
	if err != nil {
		t.Fatal(err)
	}
	if source.Name != "Production" || source.Slug != "production" {
		t.Fatalf("created environment = %#v", source)
	}
	assertCount(t, connection.ORM, &migrations.Configuration{}, "environment_id = ?", 1, source.ID)
	if _, err := service.Create(ctx, RequestContext{}, project.ID, CreateInput{Name: "Duplicate", Slug: source.Slug}); !errors.Is(err, ErrSlugExists) {
		t.Fatalf("duplicate environment error = %v", err)
	}

	fixture := createCloneFixture(t, connection.ORM, source.ID)
	impact, err := service.DeletionImpact(ctx, project.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.ServerGroups != 1 || impact.Services != 1 || impact.Accessories != 1 ||
		impact.Volumes != 1 || impact.Domains != 1 || impact.EnvironmentVariables != 2 ||
		impact.Secrets != 1 || impact.Dependencies != 1 || impact.Deployments != 1 || impact.Backups != 1 {
		t.Fatalf("source deletion impact = %#v", impact)
	}

	target, err := service.Clone(ctx, RequestContext{}, project.ID, source.ID, CloneInput{Name: "Staging"})
	if err != nil {
		t.Fatal(err)
	}
	if target.Slug != "staging" || target.ID == source.ID {
		t.Fatalf("cloned environment = %#v", target)
	}
	assertClone(t, connection.ORM, source.ID, target.ID, fixture)

	if _, err := service.Clone(ctx, RequestContext{}, project.ID, source.ID, CloneInput{
		Name: "Secrets", IncludeSecrets: true,
	}); err == nil {
		t.Fatal("expected includeSecrets to be rejected until encrypted re-scoping is available")
	}
	assertCount(t, connection.ORM, &migrations.Environment{}, "project_id = ? AND slug = ?", 0, project.ID, "secrets")
	if _, err := service.Clone(ctx, RequestContext{}, project.ID, source.ID, CloneInput{
		Name: "Duplicate staging", Slug: target.Slug,
	}); !errors.Is(err, ErrSlugExists) {
		t.Fatalf("duplicate clone error = %v", err)
	}

	pageOne, err := service.List(ctx, project.ID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageOne.Items) != 1 || pageOne.NextCursor == "" {
		t.Fatalf("first environment page = %#v", pageOne)
	}
	pageTwo, err := service.List(ctx, project.ID, pageOne.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageTwo.Items) != 1 || pageTwo.Items[0].ID == pageOne.Items[0].ID {
		t.Fatalf("second environment page = %#v", pageTwo)
	}

	renamed := "Pre-production"
	updated, err := service.Update(ctx, RequestContext{}, project.ID, target.ID, UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != renamed || updated.Slug != target.Slug {
		t.Fatalf("updated environment = %#v", updated)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, target.ID, "wrong"); !errors.Is(err, ErrConfirmationFailed) {
		t.Fatalf("wrong confirmation error = %v", err)
	}
	if err := service.Delete(ctx, RequestContext{}, project.ID, target.ID, target.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, project.ID, target.ID); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("get deleted environment error = %v", err)
	}
	if _, err := service.Get(ctx, project.ID, source.ID); err != nil {
		t.Fatalf("source environment was removed: %v", err)
	}
}

type cloneFixture struct {
	ServerID    string
	GroupID     string
	ServiceID   string
	AccessoryID string
}

func createCloneFixture(t *testing.T, db *gorm.DB, environmentID string) cloneFixture {
	t.Helper()
	fixture := cloneFixture{
		ServerID: uuid.NewString(), GroupID: uuid.NewString(),
		ServiceID: uuid.NewString(), AccessoryID: uuid.NewString(),
	}
	mustCreate(t, db, &migrations.Server{
		ID: fixture.ServerID, Name: "production-1", Hostname: "production-1.example.com",
		SSHUser: "deploy", Resources: "{}",
	})
	mustCreate(t, db, &migrations.ServerGroup{ID: fixture.GroupID, EnvironmentID: environmentID, Name: "web"})
	mustCreate(t, db, &migrations.ServerGroupMembership{ServerGroupID: fixture.GroupID, ServerID: fixture.ServerID})
	port := 3000
	mustCreate(t, db, &migrations.Service{
		ID: fixture.ServiceID, EnvironmentID: environmentID, ServerGroupID: &fixture.GroupID,
		Name: "api", Type: "web", Repository: "https://example.com/acme/api.git", Port: &port,
	})
	mustCreate(t, db, &migrations.Accessory{
		ID: fixture.AccessoryID, EnvironmentID: environmentID, ServerGroupID: &fixture.GroupID,
		Name: "postgres", Type: "postgres", Image: "postgres:16",
	})
	mustCreate(t, db, &migrations.Volume{
		ID: uuid.NewString(), EnvironmentID: environmentID, AccessoryID: &fixture.AccessoryID,
		Name: "Postgres data", Source: "postgres_data", Destination: "/var/lib/postgresql/data",
	})
	mustCreate(t, db, &migrations.Domain{
		ID: uuid.NewString(), EnvironmentID: environmentID, ServiceID: fixture.ServiceID,
		Hostname: "api.example.com", SSLEnabled: true,
	})
	mustCreate(t, db, &migrations.EnvironmentVariable{
		ID: uuid.NewString(), EnvironmentID: environmentID, Name: "NODE_ENV", Value: "production",
	})
	mustCreate(t, db, &migrations.EnvironmentVariable{
		ID: uuid.NewString(), EnvironmentID: environmentID, ServiceID: &fixture.ServiceID,
		Name: "PORT", Value: "3000",
	})
	secret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environmentID, Name: "DATABASE_URL"}
	mustCreate(t, db, &secret)
	mustCreate(t, db, &migrations.VaultEntry{
		ID: uuid.NewString(), SecretID: &secret.ID, Kind: "application_secret",
		ScopeType: "environment", ScopeID: environmentID, Name: secret.Name,
		KeyID: "test-key", FormatVersion: 1, Ciphertext: []byte("ciphertext"),
		DataNonce: []byte("data-nonce"), WrappedDEK: []byte("wrapped-key"), WrapNonce: []byte("wrap-nonce"),
	})
	mustCreate(t, db, &migrations.ServiceDependency{
		ID: uuid.NewString(), EnvironmentID: environmentID, SourceServiceID: fixture.ServiceID,
		TargetAccessoryID: &fixture.AccessoryID, Type: "runtime",
	})

	var configuration migrations.Configuration
	if err := db.First(&configuration, "environment_id = ?", environmentID).Error; err != nil {
		t.Fatal(err)
	}
	version := migrations.ConfigurationVersion{
		ID: uuid.NewString(), ConfigurationID: configuration.ID, Version: 1, Document: "{}",
	}
	mustCreate(t, db, &version)
	deployment := migrations.Deployment{
		ID: uuid.NewString(), EnvironmentID: environmentID, ServiceID: fixture.ServiceID,
		ConfigurationVersionID: version.ID, Status: "SUCCESS",
	}
	mustCreate(t, db, &deployment)
	mustCreate(t, db, &migrations.DeploymentLog{
		ID: uuid.NewString(), DeploymentID: deployment.ID, Sequence: 1, Stream: "system", Message: "done",
	})
	mustCreate(t, db, &migrations.Backup{
		ID: uuid.NewString(), Kind: "environment", EnvironmentID: &environmentID,
		StoragePath: "/backups/production.tar", Status: "completed",
	})
	return fixture
}

func assertClone(t *testing.T, db *gorm.DB, sourceID, targetID string, source cloneFixture) {
	t.Helper()
	assertCount(t, db, &migrations.ServerGroup{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Service{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Accessory{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Volume{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Domain{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.EnvironmentVariable{}, "environment_id = ?", 2, targetID)
	assertCount(t, db, &migrations.Secret{}, "environment_id = ?", 0, targetID)
	assertCount(t, db, &migrations.ServiceDependency{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Configuration{}, "environment_id = ?", 1, targetID)
	assertCount(t, db, &migrations.Deployment{}, "environment_id = ?", 0, targetID)
	assertCount(t, db, &migrations.Backup{}, "environment_id = ?", 0, targetID)

	var group migrations.ServerGroup
	if err := db.First(&group, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if group.ID == source.GroupID {
		t.Fatal("cloned server group reused the source ID")
	}
	var membership migrations.ServerGroupMembership
	if err := db.First(&membership, "server_group_id = ?", group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if membership.ServerID != source.ServerID {
		t.Fatalf("membership server = %s, want %s", membership.ServerID, source.ServerID)
	}
	var clonedService migrations.Service
	if err := db.First(&clonedService, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if clonedService.ID == source.ServiceID || clonedService.ServerGroupID == nil || *clonedService.ServerGroupID != group.ID {
		t.Fatalf("cloned service = %#v", clonedService)
	}
	var accessory migrations.Accessory
	if err := db.First(&accessory, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if accessory.ID == source.AccessoryID || accessory.ServerGroupID == nil || *accessory.ServerGroupID != group.ID {
		t.Fatalf("cloned accessory = %#v", accessory)
	}
	var volume migrations.Volume
	if err := db.First(&volume, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if volume.AccessoryID == nil || *volume.AccessoryID != accessory.ID {
		t.Fatalf("cloned volume = %#v", volume)
	}
	var dependency migrations.ServiceDependency
	if err := db.First(&dependency, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if dependency.SourceServiceID != clonedService.ID || dependency.TargetAccessoryID == nil || *dependency.TargetAccessoryID != accessory.ID {
		t.Fatalf("cloned dependency = %#v", dependency)
	}
	var configuration migrations.Configuration
	if err := db.First(&configuration, "environment_id = ?", targetID).Error; err != nil {
		t.Fatal(err)
	}
	if configuration.CurrentVersion != 0 {
		t.Fatalf("cloned configuration current version = %d", configuration.CurrentVersion)
	}
	assertCount(t, db, &migrations.ConfigurationVersion{}, "configuration_id = ?", 0, configuration.ID)
	assertCount(t, db, &migrations.Secret{}, "environment_id = ?", 1, sourceID)
}

func mustCreate(t *testing.T, db *gorm.DB, value any) {
	t.Helper()
	if err := db.Create(value).Error; err != nil {
		t.Fatalf("create %T: %v", value, err)
	}
}

func assertCount(t *testing.T, db *gorm.DB, model any, query string, expected int64, arguments ...any) {
	t.Helper()
	var count int64
	if err := db.Model(model).Where(query, arguments...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("%T count = %d, want %d", model, count, expected)
	}
}
