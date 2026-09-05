package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// envFile is the ordered contents of an installation .env file. Writing back
// preserves line order and any lines that are not KEY=VALUE pairs.
type envFile struct {
	lines []string
	index map[string]int
}

func loadEnvFile(path string) (*envFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file := &envFile{index: map[string]int{}}
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		file.lines = append(file.lines, line)
		if key, _, found := strings.Cut(line, "="); found {
			if _, seen := file.index[key]; !seen {
				file.index[key] = len(file.lines) - 1
			}
		}
	}
	return file, nil
}

func (file *envFile) get(key string) string {
	position, present := file.index[key]
	if !present {
		return ""
	}
	_, value, _ := strings.Cut(file.lines[position], "=")
	return value
}

func (file *envFile) set(key, value string) {
	line := key + "=" + value
	if position, present := file.index[key]; present {
		file.lines[position] = line
		return
	}
	file.lines = append(file.lines, line)
	file.index[key] = len(file.lines) - 1
}

// writeTo replaces path atomically and keeps the 0600 mode the installer set.
func (file *envFile) writeTo(path string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	content := strings.Join(file.lines, "\n") + "\n"
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func (in installation) loadEnv() (*envFile, error) {
	file, err := loadEnvFile(in.envFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", in.envFile, err)
	}
	return file, nil
}
