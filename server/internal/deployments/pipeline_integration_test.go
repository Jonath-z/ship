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
	"github.com/Jonath-z/ship/server/internal/gitsource"
	"github.com/Jonath-z/ship/server/internal/kamal"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/internal/sshkeys"
	"github.com/Jonath-z/ship/server/migrations"
)

// fakeEngine records what a real Kamal run would see in the workspace.
type fakeEngine struct {
	deployYAML   string
	secretsFile  string
	exitCode     int
	requests     []kamal.DeployRequest
	builds       []kamal.DeployRequest
	proxyReboots []kamal.DeployRequest
}

func (engine *fakeEngine) Build(_ context.Context, request kamal.DeployRequest, stream func(line string)) (kamal.ExecResult, error) {
	engine.builds = append(engine.builds, request)
	stream("Building image with docker buildx")
	return kamal.ExecResult{ExitCode: engine.exitCode}, nil
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

func (engine *fakeEngine) RebootProxy(_ context.Context, request kamal.DeployRequest, stream func(line string)) (kamal.ExecResult, error) {
	engine.proxyReboots = append(engine.proxyReboots, request)
	stream("Rebooting kamal-proxy on 203.0.113.10")
	return kamal.ExecResult{ExitCode: 0}, nil
}

func (engine *fakeEngine) Version(context.Context) (string, error) { return "kamal-test", nil }

// fakeCloner stands in for git: it materializes a Dockerfile where the clone
// would land and reports a fixed commit.
type fakeCloner struct {
	inputs []gitsource.Input
	sha    string
}

func (cloner *fakeCloner) Clone(_ context.Context, input gitsource.Input, stream func(line string)) (gitsource.Result, error) {
	cloner.inputs = append(cloner.inputs, input)
	if err := os.MkdirAll(input.Dir, 0o700); err != nil {
		return gitsource.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(input.Dir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		return gitsource.Result{}, err
	}
	stream("Cloning into workspace")
	return gitsource.Result{CommitSHA: cloner.sha}, nil
}

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
	webPort := 4000
	webapp := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &group.ID,
		Name: "webapp", Type: "web", Repository: "github.com/acme/webapp", Branch: "main", Port: &webPort,
	}
	secret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "DATABASE_URL"}
	registrySecret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: configuration.RegistryPasswordKey}
	tokenSecret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: configuration.GitTokenKey}
	for _, value := range []any{&project, &environment, &group, &server, &api, &webapp, &secret, &registrySecret, &tokenSecret} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := connection.ORM.Create(&migrations.ServerGroupMembership{ServerGroupID: group.ID, ServerID: server.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for row, value := range map[*migrations.Secret]string{
		&secret: "postgres://prod", &registrySecret: "registry-pass", &tokenSecret: "tok-123",
	} {
		if _, err := vault.Store(ctx, shipcrypto.StoreInput{
			SecretID: &row.ID, Kind: shipcrypto.KindApplicationSecret,
			ScopeType: "environment", ScopeID: environment.ID, Name: row.Name,
			Plaintext: []byte(value),
		}); err != nil {
			t.Fatal(err)
		}
	}

	engine := &fakeEngine{}
	cloner := &fakeCloner{sha: strings.Repeat("a", 40)}
	runner := &Runner{
		DB: connection.ORM, Redis: redis, Queue: queue,
		Configuration: configuration.NewRepository(connection.ORM),
		Vault:         vault, SSHKeys: keyService, Engine: engine, Cloner: cloner,
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

	// A repository-backed service runs the build pipeline: clone into the
	// workspace, build+push tagged with the commit SHA, deploy pinned to it.
	built, err := service.Create(ctx, RequestContext{}, project.ID, environment.ID, webapp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Handle(ctx, jobs.Job{Type: JobTypeDeploy, Payload: built.ID}); err != nil {
		t.Fatal(err)
	}
	var builtRow migrations.Deployment
	if err := connection.ORM.First(&builtRow, "id = ?", built.ID).Error; err != nil {
		t.Fatal(err)
	}
	if builtRow.Status != string(StatusSuccess) || builtRow.CommitSHA != cloner.sha ||
		builtRow.Image != "acme-production-webapp:"+cloner.sha {
		t.Fatalf("built deployment = %#v", builtRow)
	}
	if len(cloner.inputs) != 1 || cloner.inputs[0].URL != "github.com/acme/webapp" ||
		cloner.inputs[0].Branch != "main" || cloner.inputs[0].Token != "tok-123" {
		t.Fatalf("clone inputs = %#v", cloner.inputs)
	}
	if len(engine.builds) != 1 || engine.builds[0].Version != cloner.sha {
		t.Fatalf("build requests = %#v", engine.builds)
	}
	lastDeploy := engine.requests[len(engine.requests)-1]
	if lastDeploy.Version != cloner.sha || lastDeploy.Rollback {
		t.Fatalf("deploy request = %#v", lastDeploy)
	}
	// The rendered config carries the untagged image name, and the Git token
	// stays out of application env (deploy-time credential).
	if !strings.Contains(engine.deployYAML, "image: acme-production-webapp") ||
		strings.Contains(engine.deployYAML, configuration.GitTokenKey) {
		t.Fatalf("webapp deploy.yml = %q", engine.deployYAML)
	}
	builtLogs, _ := service.Logs(ctx, project.ID, environment.ID, built.ID, 0, 100)
	builtText := ""
	for _, entry := range builtLogs {
		builtText += entry.Message + "\n"
	}
	for _, expected := range []string{
		"deployment is BUILDING", "Cloning into workspace",
		"checked out github.com/acme/webapp at " + cloner.sha,
		"Building image with docker buildx", "deployment is PUSHING",
		"deployment is DEPLOYING", "deployment is SUCCESS",
	} {
		if !strings.Contains(builtText, expected) {
			t.Fatalf("build logs missing %q:\n%s", expected, builtText)
		}
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
