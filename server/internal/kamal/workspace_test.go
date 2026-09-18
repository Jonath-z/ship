package kamal

import (
	"os"
	"path/filepath"
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

	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.Root); !os.IsNotExist(err) {
		t.Fatal("workspace survived cleanup")
	}
}
