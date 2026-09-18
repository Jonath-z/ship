package gitsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		input, want, wantErr string
	}{
		{input: "github.com/acme/api", want: "https://github.com/acme/api"},
		{input: "https://github.com/acme/api.git", want: "https://github.com/acme/api.git"},
		{input: " https://gitlab.com/acme/api ", want: "https://gitlab.com/acme/api"},
		{input: "", wantErr: "no repository"},
		{input: "git@github.com:acme/api.git", wantErr: "HTTPS"},
		{input: "https://user:pass@github.com/acme/api", wantErr: "HTTPS"},
		{input: "http://github.com/acme/api", wantErr: "scheme http"},
		{input: "ssh://github.com/acme/api", wantErr: "scheme ssh"},
		{input: "file:///etc", wantErr: "scheme file"},
		{input: "https://github.com", wantErr: "missing a host or path"},
	}
	for _, testCase := range cases {
		got, err := NormalizeURL(testCase.input)
		if testCase.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("NormalizeURL(%q) error = %v, want containing %q", testCase.input, err, testCase.wantErr)
			}
			continue
		}
		if err != nil || got != testCase.want {
			t.Errorf("NormalizeURL(%q) = %q, %v, want %q", testCase.input, got, err, testCase.want)
		}
	}
}

// TestCloneResolvesCommit exercises the real git path against a local fixture
// repository. It calls the unexported clone directly: the exported entry
// point insists on HTTPS, which is exactly what production wants and exactly
// what a hermetic test cannot use.
func TestCloneResolvesCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	origin := t.TempDir()
	run := func(arguments ...string) {
		t.Helper()
		command := exec.CommandContext(ctx, "git", arguments...)
		command.Dir = origin
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	run("init", "--initial-branch", "main", ".")
	if err := os.WriteFile(filepath.Join(origin, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	var lines []string
	target := filepath.Join(t.TempDir(), "workspace", "checkout")
	result, err := CLI{}.clone(ctx, "file://"+origin, Input{URL: origin, Branch: "main", Dir: target}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CommitSHA) != 40 {
		t.Fatalf("commit SHA = %q", result.CommitSHA)
	}
	if _, err := os.Stat(filepath.Join(target, "Dockerfile")); err != nil {
		t.Fatalf("clone content missing: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected streamed clone output")
	}

	// A branch that does not exist fails with the actionable message.
	_, err = CLI{}.clone(ctx, "file://"+origin, Input{URL: origin, Branch: "missing", Dir: filepath.Join(t.TempDir(), "x")}, nil)
	if err == nil || !strings.Contains(err.Error(), "git clone failed") {
		t.Fatalf("expected clone failure, got %v", err)
	}
}
