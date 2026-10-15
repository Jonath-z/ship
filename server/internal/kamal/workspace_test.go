package kamal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeAndCleanup(t *testing.T) {
	workspace := NewWorkspace(WorkspaceInput{
		DataDir: t.TempDir(), ProjectSlug: "acme", EnvironmentSlug: "production",
		DeploymentID: "d-1", DeployYAML: []byte("service: acme\n"),
		Secrets:   map[string]string{"DATABASE_URL": "postgres://x", "API_KEY": "k"},
		SSHKeyPEM: []byte("PRIVATE"),
	})
	// A clone may populate the workspace first — its own deploy.yml included;
	// materialization overlays Ship's rendered one.
	if err := os.MkdirAll(filepath.Join(workspace.Root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "config", "deploy.yml"), []byte("service: theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Materialize(); err != nil {
		t.Fatal(err)
	}

	deployYAML, err := os.ReadFile(filepath.Join(workspace.Root, "config", "deploy.yml"))
	if err != nil || string(deployYAML) != "service: acme\n" {
		t.Fatalf("deploy.yml = %q, error = %v", deployYAML, err)
	}
	// SH-056 acceptance: secrets land only as 0600 files.
	for _, name := range []string{"secrets", "ssh_key"} {
		info, err := os.Stat(filepath.Join(workspace.Root, ".kamal", name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	secrets, _ := os.ReadFile(filepath.Join(workspace.Root, ".kamal", "secrets"))
	if string(secrets) != "API_KEY=k\nDATABASE_URL=postgres://x\n" {
		t.Fatalf("secrets file = %q", secrets)
	}

	// The ssh wrapper must be executable and force the workspace config;
	// OpenSSH ignores $HOME, so -F is the only way the config applies.
	wrapperPath := filepath.Join(workspace.BinDir(), "ssh")
	wrapperInfo, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	if wrapperInfo.Mode().Perm() != 0o700 {
		t.Fatalf("ssh wrapper mode = %v, want 0700", wrapperInfo.Mode().Perm())
	}
	wrapper, _ := os.ReadFile(wrapperPath)
	configPath := filepath.Join(workspace.HomeDir(), ".ssh", "config")
	if !strings.Contains(string(wrapper), " -F "+configPath+" ") {
		t.Fatalf("ssh wrapper does not force the workspace config: %q", wrapper)
	}
	sshConfig, _ := os.ReadFile(configPath)
	for _, directive := range []string{"IdentityFile ", "BatchMode yes", "StrictHostKeyChecking accept-new", "UserKnownHostsFile "} {
		if !strings.Contains(string(sshConfig), directive) {
			t.Fatalf("ssh config missing %q: %q", directive, sshConfig)
		}
	}

	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.Root); !os.IsNotExist(err) {
		t.Fatal("workspace survived cleanup")
	}
}
