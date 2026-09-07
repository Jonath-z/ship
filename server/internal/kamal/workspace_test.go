package kamal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeAndCleanup(t *testing.T) {
	workspace, err := Materialize(WorkspaceInput{
		DataDir: t.TempDir(), ProjectSlug: "acme", EnvironmentSlug: "production",
		DeploymentID: "d-1", DeployYAML: []byte("service: acme\n"),
		Secrets:   map[string]string{"DATABASE_URL": "postgres://x", "API_KEY": "k"},
		SSHKeyPEM: []byte("PRIVATE"),
	})
	if err != nil {
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
