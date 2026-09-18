// Package gitsource fetches application source for server-side builds. V1
// clones over HTTPS only: public repositories work with no credentials, and
// private GitHub repositories authenticate with a personal access token
// stored as an environment secret. Full Git provider integration (OAuth,
// repository browsing) stays in the V1.1 epic. The clone is shallow and
// ephemeral: it lands inside the deployment workspace and is removed with it.
package gitsource

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Input describes one source checkout.
type Input struct {
	URL    string // repository URL; a bare host path like github.com/acme/api is accepted
	Branch string // optional; the repository's default branch when empty
	Dir    string // clone target; git creates it, parents included
	Token  string // optional access token for private repositories
}

// Result reports what was checked out.
type Result struct {
	CommitSHA string // full SHA of HEAD after the clone
}

// Cloner is the pipeline's seam: the deploy runner depends on this interface
// so tests substitute a fake instead of shelling out to git.
type Cloner interface {
	Clone(ctx context.Context, input Input, stream func(line string)) (Result, error)
}

// CLI clones with the git binary present in the worker image.
type CLI struct {
	Binary  string        // defaults to "git"
	Timeout time.Duration // per clone; defaults to 10 minutes
}

// Clone performs a shallow single-branch clone and resolves the commit SHA.
// Progress lines stream as they arrive, so the deploy log shows the fetch
// advancing rather than a silent gap.
func (cli CLI) Clone(ctx context.Context, input Input, stream func(line string)) (Result, error) {
	repositoryURL, err := NormalizeURL(input.URL)
	if err != nil {
		return Result{}, err
	}
	return cli.clone(ctx, repositoryURL, input, stream)
}

func (cli CLI) clone(ctx context.Context, repositoryURL string, input Input, stream func(line string)) (Result, error) {
	cloneContext, cancel := context.WithTimeout(ctx, cli.timeout())
	defer cancel()

	arguments := []string{"clone", "--depth", "1", "--single-branch", "--progress"}
	if input.Branch != "" {
		arguments = append(arguments, "--branch", input.Branch)
	}
	arguments = append(arguments, repositoryURL, input.Dir)

	command := exec.CommandContext(cloneContext, cli.binary(), arguments...)
	// No terminal prompts: a missing repository or bad token fails immediately
	// instead of hanging the deployment on stdin.
	command.Env = []string{"GIT_TERMINAL_PROMPT=0"}
	if input.Token != "" {
		// The token travels only in the environment of this one git process
		// and is served through an askpass helper — it must never appear in
		// the URL, the argument list, or the streamed output.
		helper, cleanup, err := writeAskpassHelper()
		if err != nil {
			return Result{}, err
		}
		defer cleanup()
		command.Env = append(command.Env, "GIT_ASKPASS="+helper, askpassTokenVariable+"="+input.Token)
	}

	output, err := command.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	command.Stderr = command.Stdout // git writes progress to stderr; interleave
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("start git: %w", err)
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanner.Split(scanProgressLines)
	for scanner.Scan() {
		if stream != nil {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				stream(line)
			}
		}
	}
	if err := command.Wait(); err != nil {
		if cloneContext.Err() != nil {
			return Result{}, fmt.Errorf("git clone timed out after %s", cli.timeout())
		}
		return Result{}, fmt.Errorf("git clone failed: %w — check that the repository and branch exist and that the access token (if any) grants read access", err)
	}

	revision := exec.CommandContext(cloneContext, cli.binary(), "-C", input.Dir, "rev-parse", "HEAD")
	sha, err := revision.Output()
	if err != nil {
		return Result{}, fmt.Errorf("resolve cloned revision: %w", err)
	}
	return Result{CommitSHA: strings.TrimSpace(string(sha))}, nil
}

// NormalizeURL turns the stored repository reference into a cloneable HTTPS
// URL. Anything else is rejected — V1 cannot authenticate SSH remotes, and
// credentials embedded in the URL would leak into logs.
func NormalizeURL(repository string) (string, error) {
	trimmed := strings.TrimSpace(repository)
	if trimmed == "" {
		return "", fmt.Errorf("service has no repository configured")
	}
	if strings.Contains(trimmed, "@") {
		return "", fmt.Errorf("repository %q looks like an SSH or credentialed remote; use a plain HTTPS URL and store the token as the %s secret", repository, TokenSecretName)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("repository %q is not a valid URL: %w", repository, err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("repository %q uses scheme %s; V1 builds clone HTTPS URLs only", repository, parsed.Scheme)
	}
	if parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" {
		return "", fmt.Errorf("repository %q is missing a host or path", repository)
	}
	return parsed.String(), nil
}

// TokenSecretName is the conventional environment secret holding a Git access
// token (a GitHub fine-grained or classic PAT). Like the Kamal registry
// credentials, it is deploy-time-only: the renderer keeps it out of
// application env, and it reaches git through the askpass helper.
const TokenSecretName = "GIT_TOKEN"

const askpassTokenVariable = "SHIP_GIT_ASKPASS_TOKEN"

// writeAskpassHelper materializes a helper script that answers git's
// credential prompts: a fixed username (GitHub accepts any non-empty value
// alongside a PAT) and the token from the environment. The script itself
// never contains the token.
func writeAskpassHelper() (string, func(), error) {
	directory, err := os.MkdirTemp("", "ship-askpass-")
	if err != nil {
		return "", nil, fmt.Errorf("create askpass helper: %w", err)
	}
	script := "#!/bin/sh\ncase \"$1\" in\nUsername*) echo x-access-token ;;\n*) printenv " + askpassTokenVariable + " ;;\nesac\n"
	path := filepath.Join(directory, "askpass.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		os.RemoveAll(directory)
		return "", nil, fmt.Errorf("write askpass helper: %w", err)
	}
	return path, func() { os.RemoveAll(directory) }, nil
}

// scanProgressLines splits on \n and \r so git's carriage-return progress
// updates ("Receiving objects:  42%") surface as lines instead of buffering
// until the clone finishes.
func scanProgressLines(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if index := bytes.IndexAny(data, "\r\n"); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (cli CLI) binary() string {
	if cli.Binary == "" {
		return "git"
	}
	return cli.Binary
}

func (cli CLI) timeout() time.Duration {
	if cli.Timeout == 0 {
		return 10 * time.Minute
	}
	return cli.Timeout
}
