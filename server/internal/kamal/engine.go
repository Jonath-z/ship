// Package kamal is the only package that knows the Kamal CLI exists (spec
// §23, §44). Everything above depends on the DeploymentEngine interface.
package kamal

import "context"

// DeployRequest is one prepared engine run: the workspace already contains
// the rendered configuration, secrets, and SSH key — plus the application
// source when the service builds from a repository.
type DeployRequest struct {
	Workspace Workspace
	Rollback  bool
	// Version pins the image tag Kamal acts on. Ship always supplies it for
	// repository-backed services (the cloned commit SHA) — left to itself,
	// Kamal would derive a version from the workspace's git state, which the
	// config overlay marks dirty. For rollbacks it is the container version
	// to reactivate. Empty means Kamal's default (image-backed deploys).
	Version string
}

// DeploymentEngine abstracts the engine so the pipeline and tests never shell
// out directly. The single production implementation wraps the Kamal CLI.
type DeploymentEngine interface {
	// Build builds the workspace's application source and pushes the image
	// to the configured registry. With Kamal's buildx integration the push
	// happens inside the build — there is no separate push invocation.
	Build(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error)
	Deploy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error)
	// RebootProxy replaces kamal-proxy on the target hosts with the version
	// this Kamal requires. The pipeline invokes it to self-heal the
	// "kamal-proxy version ... is too old" deploy failure.
	RebootProxy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error)
	Version(ctx context.Context) (string, error)
}

type CLIEngine struct {
	Executor *Executor
}

func NewCLIEngine() *CLIEngine {
	return &CLIEngine{Executor: NewExecutor()}
}

func (engine *CLIEngine) Build(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error) {
	return engine.Executor.Run(ctx, request.Workspace, []string{"build", "push", "--version", request.Version}, stream)
}

func (engine *CLIEngine) Deploy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error) {
	if request.Rollback {
		return engine.Executor.Run(ctx, request.Workspace, []string{"rollback", request.Version}, stream)
	}
	// The image is already in the registry — either prebuilt (image-backed
	// services) or pushed by Build just before this call.
	arguments := []string{"deploy", "--skip-push"}
	if request.Version != "" {
		arguments = append(arguments, "--version", request.Version)
	}
	return engine.Executor.Run(ctx, request.Workspace, arguments, stream)
}

func (engine *CLIEngine) RebootProxy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error) {
	// -y skips the interactive confirmation; the brief proxy outage is
	// acceptable because the deploy already failed against the stale proxy.
	return engine.Executor.Run(ctx, request.Workspace, []string{"proxy", "reboot", "-y"}, stream)
}

func (engine *CLIEngine) Version(ctx context.Context) (string, error) {
	return engine.Executor.Version(ctx)
}
