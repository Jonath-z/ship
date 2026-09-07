package deployments

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/kamal"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/internal/sshkeys"
	"github.com/Jonath-z/ship/server/migrations"
)

// fakeEngine records what a real Kamal run would see in the workspace.
type fakeEngine struct {
	deployYAML  string
	secretsFile string
	exitCode    int
	requests    []kamal.DeployRequest
}

func (engine *fakeEngine) Deploy(_ context.Context, request kamal.DeployRequest, stream func(line string)) (kamal.ExecResult, error) {
	engine.requests = append(engine.requests, request)
	yaml, _ := os.ReadFile(filepath.Join(request.Workspace.Root, "config", "deploy.yml"))
	secrets, _ := os.ReadFile(filepath.Join(request.Workspace.Root, ".kamal", "secrets"))
	engine.deployYAML = string(yaml)
	engine.secretsFile = string(secrets)
	stream("Running docker run on 203.0.113.10")
	stream("container is healthy")
	return kamal.ExecResult{ExitCode: engine.exitCode}, nil
}

func (engine *fakeEngine) Version(context.Context) (string, error) { return "kamal-test", nil }

func TestDeploymentPipelineIntegration(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	redisURL := os.Getenv("SHIP_TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL and SHIP_TEST_REDIS_URL are required")
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
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, servers, ssh_keys, vault_entries CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, servers, ssh_keys, vault_entries CASCADE")

	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	redis := redisclient.NewClient(options)
	defer redis.Close()
	redis.FlushDB(ctx)

	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("11", 32)}, nil)
	keyService := sshkeys.NewService(connection.ORM, vault, nil)
	queue := jobs.NewQueue(redis, slog.Default())

	// Inventory: one connected server in the web role with an SSH key.
	key, err := keyService.Create(ctx, sshkeys.RequestContext{}, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	group := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "web"}
	server := migrations.Server{
		ID: uuid.NewString(), Name: "web-1", IPAddress: "203.0.113.10", SSHUser: "root",
		SSHKeyID: &key.ID, Status: "connected", Resources: "{}",
	}
	port := 3000
	api := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &group.ID,
		Name: "api", Type: "web", Image: "acme/api:v1", Port: &port,
	}
	secret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "DATABASE_URL"}
	for _, value := range []any{&project, &environment, &group, &server, &api, &secret} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := connection.ORM.Create(&migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: server.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Store(ctx, shipcrypto.StoreInput{
		SecretID: &secret.ID, Kind: shipcrypto.KindApplicationSecret,
		ScopeType: "environment", ScopeID: environment.ID, Name: secret.Name,
		Plaintext: []byte("postgres://prod"),
	}); err != nil {
		t.Fatal(err)
	}

	engine := &fakeEngine{}
	runner := &Runner{
		DB: connection.ORM, Redis: redis, Queue: queue,
		Configuration: configuration.NewRepository(connection.ORM),
		Vault:         vault, SSHKeys: keyService, Engine: engine,
		DataDir: t.TempDir(),
	}
	service := NewService(connection.ORM, redis, queue, nil)

	// POST /deployments returns immediately with a QUEUED record (§24).
	deployment, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Status != StatusQueued {
		t.Fatalf("deployment = %#v", deployment)
	}

	// Run the worker side.
	if err := runner.Handle(ctx, jobs.Job{Type: JobTypeDeploy, Payload: deployment.ID}); err != nil {
		t.Fatal(err)
	}
	finished, err := service.Get(ctx, project.ID, environment.ID, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != StatusSuccess || finished.ConfigurationVersionID == "" || finished.FinishedAt == nil {
		t.Fatalf("finished deployment = %#v", finished)
	}
	// The engine saw the rendered config and the materialized secret value.
	if !strings.Contains(engine.deployYAML, "service: acme-production-api") ||
		!strings.Contains(engine.deployYAML, "user: root") {
		t.Fatalf("deploy.yml = %q", engine.deployYAML)
	}
	if !strings.Contains(engine.secretsFile, "DATABASE_URL=postgres://prod") {
		t.Fatalf("secrets file = %q", engine.secretsFile)
	}
	// Workspace (and its secrets) did not outlive the run.
	if _, err := os.Stat(filepath.Join(runner.DataDir, "projects")); err == nil {
		entries, _ := filepath.Glob(filepath.Join(runner.DataDir, "projects", "*", "*", "*"))
		if len(entries) != 0 {
			t.Fatalf("workspace leaked: %v", entries)
		}
	}
	// Output and status transitions are persisted for replay (SH-090).
	logs, err := service.Logs(ctx, project.ID, environment.ID, deployment.ID, 0, 100)
	if err != nil || len(logs) < 4 {
		t.Fatalf("logs = %#v, error = %v", logs, err)
	}
	assembled := ""
	for _, entry := range logs {
		assembled += entry.Message + "\n"
	}
	if !strings.Contains(assembled, "deployment is VALIDATING") ||
		!strings.Contains(assembled, "Running docker run on 203.0.113.10") ||
		!strings.Contains(assembled, "deployment is SUCCESS") {
		t.Fatalf("log content:\n%s", assembled)
	}

	// Rollback reproduces the recorded configuration version (SH-076).
	rollback, err := service.Rollback(ctx, RequestContext{}, project.ID, environment.ID, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Handle(ctx, jobs.Job{Type: JobTypeDeploy, Payload: rollback.ID}); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := service.Get(ctx, project.ID, environment.ID, rollback.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != StatusRolledBack || rolledBack.ConfigurationVersionID != finished.ConfigurationVersionID {
		t.Fatalf("rollback = %#v", rolledBack)
	}

	// A validation failure marks the run FAILED with the violations logged.
	if err := connection.ORM.Delete(&migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: server.ID}).Error; err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Handle(ctx, jobs.Job{Type: JobTypeDeploy, Payload: blocked.ID}); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(ctx, project.ID, environment.ID, blocked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("blocked deployment = %#v", failed)
	}
	failedLogs, _ := service.Logs(ctx, project.ID, environment.ID, blocked.ID, 0, 100)
	failedText := ""
	for _, entry := range failedLogs {
		failedText += entry.Message + "\n"
	}
	if !strings.Contains(failedText, "service_unplaced") {
		t.Fatalf("failed logs:\n%s", failedText)
	}
}
