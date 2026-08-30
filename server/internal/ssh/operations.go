// Package ssh defines the typed remote operations Ship may execute. It does
// not expose a generic shell or accept free-form command arguments.
package ssh

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

type Operation string

const (
	OperationDockerVersion Operation = "docker.version"
	OperationContainerList Operation = "docker.containers.list"
	OperationContainerLogs Operation = "docker.container.logs"
	OperationDockerJournal Operation = "system.docker.journal"
	OperationDiskUsage     Operation = "system.disk.usage"
)

type Request struct {
	Operation Operation
	Container string
	Lines     int
}

// Command is an executable plus an argument vector. It intentionally has no
// shell source field and should be passed to an SSH executor as fixed argv.
type Command struct {
	Program string
	Args    []string
}

var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func Render(request Request) (Command, error) {
	switch request.Operation {
	case OperationDockerVersion:
		return Command{Program: "docker", Args: []string{"version", "--format", "{{json .Server}}"}}, nil
	case OperationContainerList:
		return Command{Program: "docker", Args: []string{"ps", "--all", "--no-trunc", "--format", "{{json .}}"}}, nil
	case OperationContainerLogs:
		if !containerNamePattern.MatchString(request.Container) {
			return Command{}, errors.New("container name is invalid")
		}
		lines, err := validateLines(request.Lines)
		if err != nil {
			return Command{}, err
		}
		return Command{Program: "docker", Args: []string{"logs", "--tail", strconv.Itoa(lines), request.Container}}, nil
	case OperationDockerJournal:
		lines, err := validateLines(request.Lines)
		if err != nil {
			return Command{}, err
		}
		return Command{Program: "journalctl", Args: []string{"--unit", "docker", "--lines", strconv.Itoa(lines), "--no-pager"}}, nil
	case OperationDiskUsage:
		return Command{Program: "df", Args: []string{"-P"}}, nil
	default:
		return Command{}, fmt.Errorf("SSH operation %q is not allowlisted", request.Operation)
	}
}

func validateLines(lines int) (int, error) {
	if lines == 0 {
		return 200, nil
	}
	if lines < 1 || lines > 2000 {
		return 0, errors.New("line count must be between 1 and 2000")
	}
	return lines, nil
}
