package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

var publicURLPattern = regexp.MustCompile(`^https?://([A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?|[0-9]+(\.[0-9]+){3})(:[0-9]{1,5})?$`)

// compose runs "docker compose" against the installation with inherited stdio.
func (in installation) compose(args ...string) error {
	command := in.composeCommand(args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func (in installation) composeCommand(args ...string) *exec.Cmd {
	full := append([]string{"compose", "--env-file", in.envFile, "-f", in.composeFile}, args...)
	return exec.Command("docker", full...)
}

func (in installation) pullAppImages() error {
	if os.Getenv("SHIP_SKIP_PULL") == "1" {
		logf("using locally available application images")
		return nil
	}
	return in.compose("pull", "ship-api", "ship-worker", "ship-web")
}

func (in installation) dashboardURL(env *envFile) string {
	if publicURL := env.get("SHIP_PUBLIC_URL"); publicURL != "" {
		return publicURL
	}
	webPort := env.get("SHIP_WEB_PORT")
	if webPort == "" {
		webPort = "3000"
	}
	return fmt.Sprintf("http://%s:%s", env.get("SHIP_HOSTNAME"), webPort)
}

func (in installation) status() error {
	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	fmt.Printf("Dashboard: %s\n", in.dashboardURL(env))
	fmt.Printf("Version:   %s\n\n", env.get("SHIP_VERSION"))
	return in.compose("ps")
}

func (in installation) logs(args []string) error {
	if len(args) > 0 {
		return in.compose("logs", "--tail", "200", "-f", args[0])
	}
	return in.compose("logs", "--tail", "200", "-f")
}

func (in installation) version() error {
	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	fmt.Printf("ship CLI %s\ninstalled Ship %s\n", cliVersion, env.get("SHIP_VERSION"))
	return nil
}

func (in installation) setupToken() error {
	token := randomHex(24)
	hash := sha256.Sum256([]byte(token))
	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	env.set("SHIP_FIRST_RUN_TOKEN_HASH", hex.EncodeToString(hash[:]))
	if err := env.writeTo(in.envFile); err != nil {
		return err
	}
	if err := in.compose("up", "-d", "--no-deps", "--force-recreate", "ship-api"); err != nil {
		return err
	}
	fmt.Printf("First-run token (shown once): %s\n", token)
	return nil
}

func (in installation) publicURL(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ship public-url URL")
	}
	value := args[0]
	if !publicURLPattern.MatchString(value) {
		return fmt.Errorf("public URL must look like https://ship.example.com or http://SERVER_IP:3000")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("public URL must look like https://ship.example.com or http://SERVER_IP:3000")
	}

	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	env.set("SHIP_PUBLIC_URL", value)
	// The hostname is only a fallback for deriving the URL; keep it in sync.
	env.set("SHIP_HOSTNAME", parsed.Hostname())
	if parsed.Scheme == "https" {
		env.set("SHIP_ALLOW_INSECURE_HTTP", "false")
		env.set("SHIP_TRUST_FORWARDED_IP", "true")
	} else {
		env.set("SHIP_ALLOW_INSECURE_HTTP", "true")
		env.set("SHIP_TRUST_FORWARDED_IP", "false")
	}
	if err := env.writeTo(in.envFile); err != nil {
		return err
	}
	if err := in.compose("up", "-d", "--no-deps", "--force-recreate", "ship-api", "ship-web"); err != nil {
		return err
	}
	logf("public URL changed to %s", value)
	if parsed.Scheme != "https" {
		logf("warning: HTTP is temporary insecure bootstrap access")
	}
	return nil
}

func (in installation) upgrade(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ship upgrade [vX.Y.Z]")
	}
	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	oldVersion := env.get("SHIP_VERSION")

	newVersion := ""
	if len(args) == 1 {
		newVersion = args[0]
	} else {
		newVersion, err = fetchLatestVersion()
		if err != nil {
			return fmt.Errorf("could not determine the latest release: %w", err)
		}
	}
	if !versionPattern.MatchString(newVersion) {
		return fmt.Errorf("version must look like v1.2.3")
	}
	if newVersion == oldVersion {
		logf("already running %s", newVersion)
		return nil
	}

	backup := filepath.Join(in.dir, "backups", "pre-upgrade-"+timestamp()+".tar.gz")
	if err := in.createBackup(backup); err != nil {
		return err
	}
	logf("backup saved to %s", backup)

	env.set("SHIP_VERSION", newVersion)
	if err := env.writeTo(in.envFile); err != nil {
		return err
	}
	if err := in.pullAppImages(); err != nil {
		env.set("SHIP_VERSION", oldVersion)
		if writeErr := env.writeTo(in.envFile); writeErr != nil {
			return fmt.Errorf("image pull failed and version rollback failed: %w", writeErr)
		}
		return fmt.Errorf("image pull failed; version restored to %s", oldVersion)
	}
	if err := in.compose("run", "--rm", "-T", "--no-deps", "ship-api", "-migrate-only"); err != nil {
		return err
	}
	if err := in.compose("up", "-d", "--wait", "--remove-orphans"); err != nil {
		return err
	}
	logf("upgraded from %s to %s", oldVersion, newVersion)
	return nil
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get("https://api.github.com/repos/Jonath-z/ship/releases/latest")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub returned status %d", response.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("release has no tag name")
	}
	return release.TagName, nil
}

func randomHex(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func timestamp() string {
	return time.Now().UTC().Format("20060102T150405Z")
}
