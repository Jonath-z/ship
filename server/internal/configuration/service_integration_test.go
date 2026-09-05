package configuration

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestCompileAndVersioningIntegration(t *testing.T) {
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
	group := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "web"}
	serverOne := migrations.Server{ID: uuid.NewString(), Name: "web-1", IPAddress: "203.0.113.11", SSHUser: "deploy", Status: "connected", Resources: "{}"}
	serverTwo := migrations.Server{ID: uuid.NewString(), Name: "web-2", IPAddress: "203.0.113.10", SSHUser: "deploy", Status: "connected", Resources: "{}"}
	port := 3000
	api := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &group.ID,
		Name: "api", Type: "web", Image: "acme/api:v1", Port: &port, Repository: "acme/api", Branch: "main",
	}
	postgres := migrations.Accessory{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerID: &serverOne.ID,
		Name: "postgres", Type: "postgres", Image: "postgres:16",
	}
	domain := migrations.Domain{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServiceID: api.ID,
		Hostname: "api.example.com", SSLEnabled: true,
	}
	volume := migrations.Volume{
		ID: uuid.NewString(), EnvironmentID: environment.ID, AccessoryID: &postgres.ID,
		Name: "data", Source: "postgres_data", Destination: "/var/lib/postgresql/data",
	}
	variable := migrations.EnvironmentVariable{
		ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "LOG_LEVEL", Value: "info",
	}
	secret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "DATABASE_URL"}
	edge := migrations.ServiceDependency{
		ID: uuid.NewString(), EnvironmentID: environment.ID,
		SourceServiceID: api.ID, TargetAccessoryID: &postgres.ID, Type: "runtime",
	}
	for _, value := range []any{&project, &environment, &group, &serverOne, &serverTwo, &api, &postgres, &domain, &volume, &variable, &secret, &edge} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, serverID := range []string{serverOne.ID, serverTwo.ID} {
		if err := connection.ORM.Create(&migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: serverID}).Error; err != nil {
			t.Fatal(err)
		}
	}

	repository := NewRepository(connection.ORM)

	// SH-051 acceptance: compiling the same database state twice produces
	// byte-identical output.
	firstState, facts, err := repository.Compile(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondState, _, err := repository.Compile(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstDocument, err := CanonicalJSON(firstState)
	if err != nil {
		t.Fatal(err)
	}
	secondDocument, err := CanonicalJSON(secondState)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstDocument, secondDocument) {
		t.Fatal("compiling the same state twice produced different bytes")
	}

	apiSpec := firstState.Services["api"]
	if apiSpec.Role != "web" || len(apiSpec.Hosts) != 2 || apiSpec.Hosts[0] != "203.0.113.10" {
		t.Fatalf("api spec = %#v", apiSpec)
	}
	if len(apiSpec.DependsOn) != 1 || apiSpec.DependsOn[0] != "accessory:postgres" {
		t.Fatalf("api dependsOn = %#v", apiSpec.DependsOn)
	}
	postgresSpec := firstState.Accessories["postgres"]
	if len(postgresSpec.Hosts) != 1 || postgresSpec.Hosts[0] != "203.0.113.11" || len(postgresSpec.Volumes) != 1 {
		t.Fatalf("postgres spec = %#v", postgresSpec)
	}
	if firstState.Env["LOG_LEVEL"] != "info" || len(firstState.SecretRefs) != 1 {
		t.Fatalf("environment values = %#v / %#v", firstState.Env, firstState.SecretRefs)
	}

	// The secret has no vault value yet, so validation must block on it.
	codes := violationCodes(Validate(firstState, facts))
	if codes["secret_missing_value"] != 1 {
		t.Fatalf("validation codes = %#v", codes)
	}

	// SH-053: snapshots are numbered sequentially and stay immutable.
	first, err := repository.Snapshot(ctx, environment.ID, nil, "initial configuration")
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.ChangeSummary != "initial configuration" {
		t.Fatalf("first snapshot = %#v", first)
	}
	newPort := 4000
	if err := connection.ORM.Model(&migrations.Service{}).Where("id = ?", api.ID).Update("port", newPort).Error; err != nil {
		t.Fatal(err)
	}
	second, err := repository.Snapshot(ctx, environment.ID, nil, "change api port")
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 {
		t.Fatalf("second snapshot = %#v", second)
	}
	stored, err := repository.Version(ctx, environment.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State.Services["api"].Port != 3000 {
		t.Fatalf("version 1 mutated: %#v", stored.State.Services["api"])
	}
	if err := connection.ORM.Model(&migrations.ConfigurationVersion{}).
		Where("version = ?", 1).Update("change_summary", "tampered").Error; err == nil {
		t.Fatal("expected configuration versions to be immutable")
	}
	records, err := repository.Versions(ctx, environment.ID, 10)
	if err != nil || len(records) != 2 || records[0].Version != 2 {
		t.Fatalf("versions = %#v, error = %v", records, err)
	}

	// SH-054: the diff between the two snapshots shows the port change.
	entities := Diff(stored.State, second.State)
	var apiChange EntityDiff
	for _, entity := range entities {
		if entity.Kind == "service" && entity.Name == "api" {
			apiChange = entity
		}
	}
	if apiChange.Change != ChangeChanged || len(apiChange.Fields) != 1 || apiChange.Fields[0].Field != "port" {
		t.Fatalf("api diff = %#v", apiChange)
	}
}
