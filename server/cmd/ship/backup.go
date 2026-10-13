package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

// backupFiles is the fixed archive layout, format_version 2.
var backupFiles = []string{".env", "keyring", "compose.yml", "database.db", "metadata"}

func (in installation) backupCommand(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ship backup [archive.tar.gz]")
	}
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	if target == "" {
		target = filepath.Join(in.dir, "backups", "ship-"+timestamp()+".tar.gz")
	} else if !filepath.IsAbs(target) {
		workingDir, err := os.Getwd()
		if err != nil {
			return err
		}
		target = filepath.Join(workingDir, target)
	}
	if err := in.createBackup(target); err != nil {
		return err
	}
	fmt.Println(target)
	return nil
}

// createBackup stages the archive contents in a private temporary directory,
// checksums them, and writes a 0600 tar.gz.
func (in installation) createBackup(target string) error {
	env, err := in.loadEnv()
	if err != nil {
		return err
	}
	if _, err := os.Stat(in.keyringFile); err != nil {
		return fmt.Errorf("missing encryption keyring")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Join(in.dir, "backups"), ".backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	logf("exporting database")
	if err := exportDatabase(in.databaseFile, filepath.Join(staging, "database.db")); err != nil {
		return fmt.Errorf("export database: %w", err)
	}

	if err := copyFile(in.envFile, filepath.Join(staging, ".env"), 0o600); err != nil {
		return err
	}
	if err := copyFile(in.keyringFile, filepath.Join(staging, "keyring"), 0o600); err != nil {
		return err
	}
	if err := copyFile(in.composeFile, filepath.Join(staging, "compose.yml"), 0o644); err != nil {
		return err
	}
	metadata := fmt.Sprintf("created_at=%s\nship_version=%s\nformat_version=2\n",
		timestamp(), env.get("SHIP_VERSION"))
	if err := os.WriteFile(filepath.Join(staging, "metadata"), []byte(metadata), 0o600); err != nil {
		return err
	}

	var checksums strings.Builder
	for _, name := range backupFiles {
		sum, err := sha256File(filepath.Join(staging, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&checksums, "%s  %s\n", sum, name)
	}
	if err := os.WriteFile(filepath.Join(staging, "SHA256SUMS"), []byte(checksums.String()), 0o600); err != nil {
		return err
	}

	return writeTarGz(target, staging, append(backupFiles, "SHA256SUMS"))
}

func (in installation) restore(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: ship restore ARCHIVE [--yes]")
	}
	archive := args[0]
	assumeYes := false
	if len(args) == 2 {
		if args[1] != "--yes" {
			return fmt.Errorf("usage: ship restore ARCHIVE [--yes]")
		}
		assumeYes = true
	}
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("backup does not exist: %s", archive)
	}
	if err := confirmRestore(assumeYes); err != nil {
		return err
	}

	staging, err := os.MkdirTemp(filepath.Join(in.dir, "backups"), ".restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractTarGz(archive, staging); err != nil {
		return err
	}
	for _, name := range append(backupFiles, "SHA256SUMS") {
		if _, err := os.Stat(filepath.Join(staging, name)); err != nil {
			return fmt.Errorf("backup is missing %s", name)
		}
	}
	if err := verifyChecksums(staging); err != nil {
		return err
	}

	logf("stopping application processes")
	_ = in.compose("stop", "ship-api", "ship-worker")

	if err := copyFile(filepath.Join(staging, ".env"), in.envFile, 0o600); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(in.keyringFile), 0o700); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(staging, "keyring"), in.keyringFile, 0o600); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		// 10001 is the fixed Ship service account inside the containers.
		if err := os.Chown(filepath.Dir(in.keyringFile), 10001, 10001); err != nil {
			return err
		}
		if err := os.Chown(in.keyringFile, 10001, 10001); err != nil {
			return err
		}
	}
	if err := copyFile(filepath.Join(staging, "compose.yml"), in.composeFile, 0o644); err != nil {
		return err
	}

	env, err := in.loadEnv()
	if err != nil {
		return err
	}

	logf("restoring database")
	if err := in.restoreDatabase(filepath.Join(staging, "database.db")); err != nil {
		return fmt.Errorf("restore database: %w", err)
	}

	if err := in.pullAppImages(); err != nil {
		return err
	}
	if err := in.compose("up", "-d", "--wait", "--remove-orphans"); err != nil {
		return err
	}
	logf("restore complete; dashboard: %s", in.dashboardURL(env))
	return nil
}

// exportDatabase takes an online, consistent snapshot of the SQLite control
// plane database. VACUUM INTO runs inside a read transaction, so the api and
// worker containers can keep writing while the backup is taken; the bind
// mount shares the host filesystem, so SQLite's cross-process locking holds.
func exportDatabase(databaseFile, target string) error {
	if _, err := os.Stat(databaseFile); err != nil {
		return fmt.Errorf("missing control-plane database: %s", databaseFile)
	}
	db, err := sql.Open("sqlite", "file:"+databaseFile+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec("VACUUM INTO ?", target); err != nil {
		return err
	}
	return os.Chmod(target, 0o600)
}

// restoreDatabase replaces the control-plane database file. The api and
// worker containers must be stopped; stale WAL and shared-memory files are
// removed so SQLite cannot replay journal state from the old database.
func (in installation) restoreDatabase(source string) error {
	directory := filepath.Dir(in.databaseFile)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(in.databaseFile + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := copyFile(source, in.databaseFile, 0o600); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		// 10001 is the fixed Ship service account inside the containers.
		if err := os.Chown(directory, 10001, 10001); err != nil {
			return err
		}
		if err := os.Chown(in.databaseFile, 10001, 10001); err != nil {
			return err
		}
	}
	return nil
}

func confirmRestore(assumeYes bool) error {
	if assumeYes {
		return nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("restore replaces current control-plane data; pass --yes")
	}
	fmt.Print("Restore will replace Ship database and workspace data. Type RESTORE to continue: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "RESTORE" {
		return fmt.Errorf("restore cancelled")
	}
	return nil
}

func verifyChecksums(directory string) error {
	raw, err := os.ReadFile(filepath.Join(directory, "SHA256SUMS"))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("malformed SHA256SUMS line: %q", line)
		}
		expected, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if strings.Contains(name, "/") || name == ".." {
			return fmt.Errorf("unsafe file name in SHA256SUMS: %q", name)
		}
		actual, err := sha256File(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, mode)
}

func writeTarGz(target, directory string, names []string) error {
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	compressor := gzip.NewWriter(output)
	archive := tar.NewWriter(compressor)
	for _, name := range names {
		path := filepath.Join(directory, name)
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(archive, file); err != nil {
			file.Close()
			return err
		}
		file.Close()
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := compressor.Close(); err != nil {
		return err
	}
	return output.Close()
}

// extractTarGz unpacks only regular files with safe relative names.
func extractTarGz(archive, directory string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	decompressor, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer decompressor.Close()
	reader := tar.NewReader(decompressor)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if header.Typeflag != tar.TypeReg || filepath.IsAbs(name) || strings.HasPrefix(name, "..") || strings.Contains(name, "/") {
			return fmt.Errorf("unsafe entry in backup archive: %q", header.Name)
		}
		destination, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(destination, reader); err != nil {
			destination.Close()
			return err
		}
		if err := destination.Close(); err != nil {
			return err
		}
	}
}
