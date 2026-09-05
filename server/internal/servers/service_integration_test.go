package servers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	cryptossh "golang.org/x/crypto/ssh"

	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	shipssh "github.com/Jonath-z/ship/server/internal/ssh"
	"github.com/Jonath-z/ship/server/internal/sshkeys"
	"github.com/Jonath-z/ship/server/migrations"
)

// fakeRunner answers allowlisted operations with canned results, so the whole
// registration -> check -> prepare flow runs without a VPS.
type fakeRunner struct {
	failSSH  bool
	noDocker bool
	prepared bool
}

func (runner *fakeRunner) Run(_ context.Context, _ shipssh.Target, _ cryptossh.Signer, command shipssh.Command, stream func(line string)) (shipssh.Result, error) {
	if runner.failSSH {
		return shipssh.Result{ExitCode: -1}, errors.New("dial 203.0.113.9:22: connection refused")
	}
	line := command.CommandLine()
	respond := func(stdout string, exit int) (shipssh.Result, error) {
		if stream != nil {
			for _, streamLine := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
				stream(streamLine)
			}
		}
		return shipssh.Result{Stdout: stdout, ExitCode: exit, HostKey: "ssh-ed25519 AAAATESTKEY"}, nil
	}
	switch {
	case strings.HasPrefix(line, "docker 'version'"):
		if runner.noDocker && !runner.prepared {
			return respond("", 127)
		}
		return respond(`{"Version":"27.0.1"}`+"\n", 0)
	case strings.HasPrefix(line, "cat '/etc/os-release'"):
		return respond("ID=ubuntu\nVERSION_ID=\"24.04\"\n", 0)
	case strings.HasPrefix(line, "uname '-m'"):
		return respond("x86_64\n", 0)
	case strings.Contains(line, "nproc"):
		return respond("cpu 4\nmemory 8589934592\ndisk 107374182400\n", 0)
	case strings.Contains(line, "get.docker.com"):
		runner.prepared = true
		return respond("installing docker\ndocker installed\n", 0)
	default:
		return respond("ok\n", 0)
	}
}

func TestServerLifecycleIntegration(t *testing.T) {
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
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, servers, ssh_keys, vault_entries CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, servers, ssh_keys, vault_entries CASCADE")

	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("11", 32)}, nil)
	keyService := sshkeys.NewService(connection.ORM, vault, nil)
	runner := &fakeRunner{noDocker: true}
	service := NewService(connection.ORM, keyService, runner, nil)

	// SH-041: generate a key; the private key is vaulted, never returned.
	key, err := keyService.Create(ctx, sshkeys.RequestContext{}, "default")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q", key.PublicKey)
	}
	if _, err := keyService.Signer(ctx, key.ID); err != nil {
		t.Fatalf("signer from vault: %v", err)
	}

	// SH-042: register a server.
	server, err := service.Create(ctx, RequestContext{}, CreateInput{
		Name: "web-1", IPAddress: "203.0.113.9", SSHKeyID: key.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.Status != "pending" || server.SSHUser != "root" || server.SSHPort != 22 {
		t.Fatalf("server = %#v", server)
	}
	if err := keyService.Delete(ctx, sshkeys.RequestContext{}, key.ID); !errors.Is(err, sshkeys.ErrKeyInUse) {
		t.Fatalf("deleting assigned key error = %v", err)
	}

	// SH-043: docker is missing -> degraded, per-check detail, host key saved.
	report, err := service.RunChecks(ctx, RequestContext{}, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "degraded" || len(report.Checks) != 5 {
		t.Fatalf("report = %#v", report)
	}
	if report.Checks[1].Name != "docker" || report.Checks[1].OK {
		t.Fatalf("docker check = %#v", report.Checks[1])
	}
	refreshed, _ := service.Get(ctx, server.ID)
	if refreshed.OS != "ubuntu 24.04" || refreshed.Architecture != "amd64" ||
		refreshed.Resources.CPUCores != 4 || !refreshed.HostKeySaved {
		t.Fatalf("refreshed server = %#v", refreshed)
	}

	// SH-044: preparation installs Docker; the re-check turns connected.
	prepared, err := service.Prepare(ctx, RequestContext{}, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Log) == 0 || prepared.Report.Status != "connected" {
		t.Fatalf("prepared = %#v", prepared)
	}

	// SH-045: group membership with last-member-in-use validation.
	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	group := migrations.ServerGroup{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "web"}
	for _, value := range []any{&project, &environment, &group} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := service.AddMember(ctx, RequestContext{}, project.ID, environment.ID, group.ID, server.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.AddMember(ctx, RequestContext{}, project.ID, environment.ID, group.ID, server.ID); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("duplicate membership error = %v", err)
	}
	groups, err := service.Groups(ctx, project.ID, environment.ID)
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 1 {
		t.Fatalf("groups = %#v, error = %v", groups, err)
	}

	apiService := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID, ServerGroupID: &group.ID,
		Name: "api", Type: "web", Image: "acme/api:v1",
	}
	if err := connection.ORM.Create(&apiService).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RemoveMember(ctx, RequestContext{}, project.ID, environment.ID, group.ID, server.ID); !errors.Is(err, ErrLastMemberInUse) {
		t.Fatalf("last member removal error = %v", err)
	}

	// SH-042 acceptance: removal is blocked while services are assigned.
	err = service.Delete(ctx, RequestContext{}, server.ID)
	var dependents *DependentsError
	if !errors.As(err, &dependents) || len(dependents.Names) != 1 || dependents.Names[0] != "service api" {
		t.Fatalf("delete in-use server error = %v", err)
	}

	// Freeing the group releases both the membership and the server.
	if err := connection.ORM.Delete(&apiService).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RemoveMember(ctx, RequestContext{}, project.ID, environment.ID, group.ID, server.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, RequestContext{}, server.ID); err != nil {
		t.Fatal(err)
	}
	if err := keyService.Delete(ctx, sshkeys.RequestContext{}, key.ID); err != nil {
		t.Fatalf("deleting released key error = %v", err)
	}
}

func TestRunChecksReportsUnreachableSSH(t *testing.T) {
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
	if err := connection.ORM.Exec("TRUNCATE TABLE servers, ssh_keys, vault_entries CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE servers, ssh_keys, vault_entries CASCADE")

	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("11", 32)}, nil)
	keyService := sshkeys.NewService(connection.ORM, vault, nil)
	service := NewService(connection.ORM, keyService, &fakeRunner{failSSH: true}, nil)

	key, err := keyService.Create(ctx, sshkeys.RequestContext{}, "default")
	if err != nil {
		t.Fatal(err)
	}
	server, err := service.Create(ctx, RequestContext{}, CreateInput{
		Name: "web-1", IPAddress: "203.0.113.9", SSHKeyID: key.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.RunChecks(ctx, RequestContext{}, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "disconnected" || len(report.Checks) != 1 || report.Checks[0].OK {
		t.Fatalf("report = %#v", report)
	}
	if !strings.Contains(report.Checks[0].Detail, "connection refused") {
		t.Fatalf("ssh detail = %q", report.Checks[0].Detail)
	}
}
