package database

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/migrations"
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

	connection, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	assertExpectedTables(t, connection.ORM)
	assertSchemaConstraintsAndCascades(t, connection.ORM)
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	if err := MigrateDown(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
}

func assertExpectedTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{
		"users", "projects", "environments", "servers", "server_groups",
		"server_group_memberships", "services", "service_dependencies",
		"accessories", "volumes", "domains", "environment_variables",
		"secrets", "vault_entries", "configurations", "configuration_versions",
		"deployments", "deployment_logs", "backups", "audit_logs",
	} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func assertSchemaConstraintsAndCascades(t *testing.T, db *gorm.DB) {
	t.Helper()
	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	mustCreate(t, db, &project)
	environment := migrations.Environment{
		ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production",
	}
	mustCreate(t, db, &environment)
	server := migrations.Server{
		ID: uuid.NewString(), Name: "production-1", Hostname: "prod-1.example.com",
		SSHUser: "deploy", Resources: "{}",
	}
	mustCreate(t, db, &server)
	serverGroup := migrations.ServerGroup{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "web",
	}
	mustCreate(t, db, &serverGroup)
	mustCreate(t, db, &migrations.ServerGroupMembership{
		ServerGroupID: serverGroup.ID, ServerID: server.ID,
	})
	port := 3000
	service := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &serverGroup.ID,
		Name: "api", Type: "web", Repository: "https://example.com/acme/api.git", Port: &port,
	}
	mustCreate(t, db, &service)
	accessory := migrations.Accessory{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "postgres",
		Type: "postgres", Image: "postgres:16", Port: intPointer(5432),
	}
	mustCreate(t, db, &accessory)
	mustCreate(t, db, &migrations.Volume{
		ID: uuid.NewString(), EnvironmentID: environment.ID, AccessoryID: &accessory.ID,
		Name: "postgres data", Source: "postgres_data", Destination: "/var/lib/postgresql/data",
	})
	mustCreate(t, db, &migrations.Domain{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServiceID: service.ID,
		Hostname: "api.example.com", SSLEnabled: true,
	})
	mustCreate(t, db, &migrations.EnvironmentVariable{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "NODE_ENV", Value: "production",
	})
	secret := migrations.Secret{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "DATABASE_URL",
	}
	mustCreate(t, db, &secret)
	mustCreate(t, db, &migrations.VaultEntry{
		ID: uuid.NewString(), SecretID: &secret.ID, Kind: "secret", ScopeType: "environment",
		ScopeID: environment.ID, Name: secret.Name, KeyID: "test-key", FormatVersion: 1,
		Ciphertext: []byte("ciphertext"), DataNonce: []byte("data-nonce"),
		WrappedDEK: []byte("wrapped-key"), WrapNonce: []byte("wrap-nonce"),
	})
	mustCreate(t, db, &migrations.ServiceDependency{
		ID: uuid.NewString(), EnvironmentID: environment.ID, SourceServiceID: service.ID,
		TargetAccessoryID: &accessory.ID, Type: "runtime",
	})
	configuration := migrations.Configuration{ID: uuid.NewString(), EnvironmentID: environment.ID}
	mustCreate(t, db, &configuration)
	configurationVersion := migrations.ConfigurationVersion{
		ID: uuid.NewString(), ConfigurationID: configuration.ID, Version: 1, Document: "{}",
	}
	mustCreate(t, db, &configurationVersion)
	deployment := migrations.Deployment{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServiceID: service.ID,
		ConfigurationVersionID: configurationVersion.ID, Status: "QUEUED",
	}
	mustCreate(t, db, &deployment)
	mustCreate(t, db, &migrations.DeploymentLog{
		ID: uuid.NewString(), DeploymentID: deployment.ID, Sequence: 1,
		Stream: "system", Message: "queued",
	})
	mustCreate(t, db, &migrations.Backup{
		ID: uuid.NewString(), Kind: "environment", EnvironmentID: &environment.ID,
		StoragePath: "/backups/acme-production.tar", Status: "completed",
	})

	expectCreateFailure(t, db, &migrations.Project{
		ID: uuid.NewString(), Name: "Duplicate", Slug: project.Slug,
	}, "duplicate project slug")
	expectCreateFailure(t, db, &migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "empty-source", Type: "web",
	}, "service without repository or image")
	expectCreateFailure(t, db, &migrations.Accessory{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerID: &server.ID,
		ServerGroupID: &serverGroup.ID, Name: "bad-placement", Type: "redis", Image: "redis:7",
	}, "accessory with two placements")
	expectCreateFailure(t, db, &migrations.Volume{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "orphan",
		Source: "orphan_data", Destination: "/data",
	}, "volume without an owner")
	expectCreateFailure(t, db, &migrations.EnvironmentVariable{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "NODE_ENV", Value: "staging",
	}, "duplicate environment variable")
	expectCreateFailure(t, db, &migrations.ServiceDependency{
		ID: uuid.NewString(), EnvironmentID: environment.ID, SourceServiceID: service.ID,
		Type: "runtime",
	}, "dependency without a target")

	if err := db.Delete(&project).Error; err != nil {
		t.Fatalf("delete project cascade: %v", err)
	}
	for name, model := range map[string]any{
		"environments": &migrations.Environment{}, "server groups": &migrations.ServerGroup{},
		"server memberships": &migrations.ServerGroupMembership{}, "services": &migrations.Service{},
		"accessories": &migrations.Accessory{}, "volumes": &migrations.Volume{},
		"domains": &migrations.Domain{}, "environment variables": &migrations.EnvironmentVariable{},
		"secrets": &migrations.Secret{}, "vault entries": &migrations.VaultEntry{},
		"dependencies": &migrations.ServiceDependency{}, "configurations": &migrations.Configuration{},
		"configuration versions": &migrations.ConfigurationVersion{}, "deployments": &migrations.Deployment{},
		"deployment logs": &migrations.DeploymentLog{}, "backups": &migrations.Backup{},
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatalf("count %s after cascade: %v", name, err)
		}
		if count != 0 {
			t.Errorf("expected project deletion to remove %s, found %d", name, count)
		}
	}
	var serverCount int64
	if err := db.Model(&migrations.Server{}).Count(&serverCount).Error; err != nil {
		t.Fatal(err)
	}
	if serverCount != 1 {
		t.Errorf("expected global server to remain after project deletion, found %d", serverCount)
	}
}

func mustCreate(t *testing.T, db *gorm.DB, value any) {
	t.Helper()
	if err := db.Create(value).Error; err != nil {
		t.Fatalf("create %T: %v", value, err)
	}
}

func expectCreateFailure(t *testing.T, db *gorm.DB, value any, description string) {
	t.Helper()
	if err := db.Create(value).Error; err == nil {
		t.Fatalf("expected database to reject %s", description)
	}
}

func intPointer(value int) *int {
	return &value
}
