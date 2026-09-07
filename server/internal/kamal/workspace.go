package kamal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Workspace materialization (SH-056). Each deployment gets its own directory
// under the Ship data dir:
//
//	<dataDir>/projects/<project>/<environment>/<deployment-id>/
//	├── config/deploy.yml    the rendered Kamal configuration
//	└── .kamal/
//	    ├── secrets          KEY=value lines, mode 0600
//	    └── ssh_key          private key PEM, mode 0600
//
// The whole directory is removed after the run — secrets never outlive it.
type Workspace struct {
	Root string
}

type WorkspaceInput struct {
	DataDir         string
	ProjectSlug     string
	EnvironmentSlug string
	DeploymentID    string
	DeployYAML      []byte
	Secrets         map[string]string // name -> plaintext value
	SSHKeyPEM       []byte
}

func Materialize(input WorkspaceInput) (Workspace, error) {
	root := filepath.Join(input.DataDir, "projects", input.ProjectSlug, input.EnvironmentSlug, input.DeploymentID)
	workspace := Workspace{Root: root}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		return workspace, fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".kamal"), 0o700); err != nil {
		return workspace, fmt.Errorf("create workspace: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "deploy.yml"), input.DeployYAML, 0o600); err != nil {
		return workspace, fmt.Errorf("write deploy.yml: %w", err)
	}

	names := make([]string, 0, len(input.Secrets))
	for name := range input.Secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	var secretsFile strings.Builder
	for _, name := range names {
		secretsFile.WriteString(name)
		secretsFile.WriteByte('=')
		secretsFile.WriteString(input.Secrets[name])
		secretsFile.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, ".kamal", "secrets"), []byte(secretsFile.String()), 0o600); err != nil {
		return workspace, fmt.Errorf("write secrets: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".kamal", "ssh_key"), input.SSHKeyPEM, 0o600); err != nil {
		return workspace, fmt.Errorf("write ssh key: %w", err)
	}
	return workspace, nil
}

// Cleanup removes the workspace and everything in it. It runs on every exit
// path of a deployment.
func (workspace Workspace) Cleanup() error {
	if workspace.Root == "" {
		return nil
	}
	return os.RemoveAll(workspace.Root)
}
