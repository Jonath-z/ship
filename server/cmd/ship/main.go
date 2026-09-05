// Command ship operates a Ship control-plane installation on the host. It is
// the Go replacement for the previous shell CLI and is packaged into the
// release bundle next to compose.yml.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
)

const cliVersion = "0.1.0"

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$`)

const usage = `Operate a Ship control-plane installation.

Usage:
  ship status
  ship logs [service]
  ship upgrade [vX.Y.Z]
  ship backup [archive.tar.gz]
  ship restore ARCHIVE [--yes]
  ship setup-token
  ship public-url URL
  ship version
`

// installation holds the host paths every command operates on.
type installation struct {
	dir         string
	composeFile string
	envFile     string
	keyringFile string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ship: error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(usage)
		return nil
	}
	if err := rerunWithSudoIfNeeded(); err != nil {
		return err
	}

	dir := os.Getenv("SHIP_INSTALL_DIR")
	if dir == "" {
		dir = "/opt/ship"
	}
	install := installation{
		dir:         dir,
		composeFile: filepath.Join(dir, "compose.yml"),
		envFile:     filepath.Join(dir, ".env"),
		keyringFile: filepath.Join(dir, "keys", "keyring"),
	}
	if _, err := os.Stat(install.composeFile); err != nil {
		return fmt.Errorf("Ship is not installed at %s", install.dir)
	}
	if _, err := os.Stat(install.envFile); err != nil {
		return fmt.Errorf("missing %s", install.envFile)
	}

	switch args[0] {
	case "status":
		return install.status()
	case "logs":
		return install.logs(args[1:])
	case "upgrade":
		return install.upgrade(args[1:])
	case "backup":
		return install.backupCommand(args[1:])
	case "restore":
		return install.restore(args[1:])
	case "setup-token":
		return install.setupToken()
	case "public-url":
		return install.publicURL(args[1:])
	case "version", "--version", "-v":
		return install.version()
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// rerunWithSudoIfNeeded replaces the process with "sudo ship ..." when invoked
// without root, mirroring the behaviour operators expect from the installer.
func rerunWithSudoIfNeeded() error {
	if os.Geteuid() == 0 || os.Getenv("SHIP_ALLOW_NON_ROOT") == "1" {
		return nil
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("root privileges are required")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	argv := append([]string{sudo, "--preserve-env=SHIP_INSTALL_DIR", self}, os.Args[1:]...)
	return syscall.Exec(sudo, argv, os.Environ())
}

func logf(format string, args ...any) {
	fmt.Printf("ship: "+format+"\n", args...)
}
