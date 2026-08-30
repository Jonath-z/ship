package environmentvariables

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	maxImportBytes   = 256 * 1024
	maxImportEntries = 500
)

type dotenvEntry struct {
	Name  string
	Value string
}

func parseDotenv(content string) ([]dotenvEntry, error) {
	if len(content) > maxImportBytes {
		return nil, errors.New(".env content exceeds 256 KiB")
	}
	entries := make([]dotenvEntry, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 4096), maxImportBytes)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		separator := strings.IndexByte(line, '=')
		if separator < 1 {
			return nil, fmt.Errorf("line %d must use NAME=value", lineNumber)
		}
		name := strings.TrimSpace(line[:separator])
		value, err := parseDotenvValue(strings.TrimSpace(line[separator+1:]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("line %d repeats %s", lineNumber, name)
		}
		seen[name] = struct{}{}
		entries = append(entries, dotenvEntry{Name: name, Value: value})
		if len(entries) > maxImportEntries {
			return nil, fmt.Errorf(".env content exceeds %d entries", maxImportEntries)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read .env content: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New(".env content contains no values")
	}
	return entries, nil
}

func parseDotenvValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("single-quoted value is not closed")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != '"' {
			return "", errors.New("double-quoted value is not closed")
		}
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("double-quoted value contains an invalid escape")
		}
		return decoded, nil
	}
	return value, nil
}
