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
//	├── <application source>   repository-backed services: the shallow clone
//	├── config/deploy.yml      the rendered Kamal configuration
//	└── .kamal/
//	    ├── secrets            KEY=value lines, mode 0600
//	    └── ssh_key            private key PEM, mode 0600
//
// This is Kamal's expected layout: it runs from an application root whose
// config/ holds deploy.yml, so builds find the Dockerfile with the default
// builder context. For repository-backed services the clone lands FIRST
// (git refuses a non-empty target) and Materialize overlays Ship's files —
// overwriting any config/deploy.yml or .kamal/ the repository may track.
//
// The whole directory is removed after the run — secrets never outlive it.
type Workspace struct {
	Root  string
	input WorkspaceInput
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

// NewWorkspace resolves the workspace path without touching the filesystem,
// so the pipeline can register cleanup (and clone into Root) before any file
// exists.
func NewWorkspace(input WorkspaceInput) Workspace {
	return Workspace{
		Root:  filepath.Join(input.DataDir, "projects", input.ProjectSlug, input.EnvironmentSlug, input.DeploymentID),
		input: input,
	}
}

// Materialize writes the rendered configuration, secrets, and SSH key into
// the workspace, creating it if the clone step has not already done so.
func (workspace Workspace) Materialize() error {
	root, input := workspace.Root, workspace.input
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".kamal"), 0o700); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "deploy.yml"), input.DeployYAML, 0o600); err != nil {
		return fmt.Errorf("write deploy.yml: %w", err)
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
		return fmt.Errorf("write secrets: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".kamal", "ssh_key"), input.SSHKeyPEM, 0o600); err != nil {
		return fmt.Errorf("write ssh key: %w", err)
	}
	return nil
}

// Cleanup removes the workspace and everything in it. It runs on every exit
// path of a deployment.
func (workspace Workspace) Cleanup() error {
	if workspace.Root == "" {
		return nil
	}
	return os.RemoveAll(workspace.Root)
}
