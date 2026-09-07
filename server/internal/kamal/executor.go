package kamal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Executor runs the Kamal binary against a prepared workspace (SH-061).
// Output streams incrementally; the environment is minimal and controlled.
type Executor struct {
	Binary  string        // defaults to "kamal"
	Timeout time.Duration // per invocation; defaults to 30 minutes
}

func NewExecutor() *Executor {
	return &Executor{Binary: "kamal", Timeout: 30 * time.Minute}
}

type ExecResult struct {
	ExitCode int
}

// Run invokes kamal with the given arguments inside the workspace. Every
// output line is passed to stream as it arrives.
func (executor *Executor) Run(ctx context.Context, workspace Workspace, arguments []string, stream func(line string)) (ExecResult, error) {
	runContext, cancel := context.WithTimeout(ctx, executor.timeout())
	defer cancel()

	command := exec.CommandContext(runContext, executor.binary(), arguments...)
	command.Dir = workspace.Root
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"KAMAL_CONFIG_PATH=config/deploy.yml",
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		return ExecResult{ExitCode: -1}, err
	}
	command.Stderr = command.Stdout // interleave, as a terminal would
	if err := command.Start(); err != nil {
		return ExecResult{ExitCode: -1}, fmt.Errorf("start %s: %w", executor.binary(), err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if stream != nil {
			stream(scanner.Text())
		}
	}

	waitErr := command.Wait()
	if waitErr == nil {
		return ExecResult{ExitCode: 0}, nil
	}
	var exitError *exec.ExitError
	if ok := isExitError(waitErr, &exitError); ok {
		return ExecResult{ExitCode: exitError.ExitCode()}, nil
	}
	if runContext.Err() != nil {
		return ExecResult{ExitCode: -1}, fmt.Errorf("kamal timed out after %s", executor.timeout())
	}
	return ExecResult{ExitCode: -1}, waitErr
}

// Version reports the installed Kamal version (SH-062 startup assertion).
func (executor *Executor) Version(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, executor.binary(), "version").Output()
	if err != nil {
		return "", fmt.Errorf("kamal is not available: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (executor *Executor) binary() string {
	if executor.Binary == "" {
		return "kamal"
	}
	return executor.Binary
}

func (executor *Executor) timeout() time.Duration {
	if executor.Timeout == 0 {
		return 30 * time.Minute
	}
	return executor.Timeout
}

func isExitError(err error, target **exec.ExitError) bool {
	exitError, ok := err.(*exec.ExitError)
	if ok {
		*target = exitError
	}
	return ok
}
