// Package docker reads container state on remote hosts (SH-046). Every call
// goes through the typed SSH operation allowlist — this package never builds
// command strings of its own. Stats collection belongs to the deferred
// monitoring epic and is intentionally absent.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cryptossh "golang.org/x/crypto/ssh"

	shipssh "github.com/Jonath-z/ship/server/internal/ssh"
)

type Runner interface {
	Run(ctx context.Context, target shipssh.Target, signer cryptossh.Signer, command shipssh.Command, stream func(line string)) (shipssh.Result, error)
}

type Container struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
}

type Client struct {
	runner Runner
}

func NewClient(runner Runner) *Client {
	return &Client{runner: runner}
}

// Version reports the Docker server version, or an error when Docker is
// missing or not responding.
func (client *Client) Version(ctx context.Context, target shipssh.Target, signer cryptossh.Signer) (string, error) {
	result, err := client.run(ctx, target, signer, shipssh.Request{Operation: shipssh.OperationDockerVersion})
	if err != nil {
		return "", err
	}
	var version struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &version); err != nil {
		return "", fmt.Errorf("parse docker version: %w", err)
	}
	return version.Version, nil
}

// Containers lists all containers on the host.
func (client *Client) Containers(ctx context.Context, target shipssh.Target, signer cryptossh.Signer) ([]Container, error) {
	result, err := client.run(ctx, target, signer, shipssh.Request{Operation: shipssh.OperationContainerList})
	if err != nil {
		return nil, err
	}
	containers := []Container{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			Status string `json:"Status"`
			State  string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // tolerate unexpected lines rather than failing the listing
		}
		containers = append(containers, Container{
			Name: entry.Names, Image: entry.Image, Status: entry.Status, State: entry.State,
		})
	}
	return containers, nil
}

// Logs tails one container's log, streaming lines as they arrive when stream
// is non-nil.
func (client *Client) Logs(ctx context.Context, target shipssh.Target, signer cryptossh.Signer, container string, lines int, stream func(line string)) (string, error) {
	command, err := shipssh.Render(shipssh.Request{
		Operation: shipssh.OperationContainerLogs, Container: container, Lines: lines,
	})
	if err != nil {
		return "", err
	}
	result, err := client.runner.Run(ctx, target, signer, command, stream)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("docker logs exited with status %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

func (client *Client) run(ctx context.Context, target shipssh.Target, signer cryptossh.Signer, request shipssh.Request) (shipssh.Result, error) {
	command, err := shipssh.Render(request)
	if err != nil {
		return shipssh.Result{}, err
	}
	result, err := client.runner.Run(ctx, target, signer, command, nil)
	if err != nil {
		return shipssh.Result{}, err
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("remote docker command exited with status %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result, nil
}
