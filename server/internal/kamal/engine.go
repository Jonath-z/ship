// Package kamal is the only package that knows the Kamal CLI exists (spec
// §23, §44). Everything above depends on the DeploymentEngine interface.
package kamal

import "context"

// DeployRequest is one prepared engine run: the workspace already contains
// the rendered configuration, secrets, and SSH key.
type DeployRequest struct {
	Workspace Workspace
	Rollback  bool
	Version   string // for rollbacks: the container version to reactivate
}

// DeploymentEngine abstracts the engine so the pipeline and tests never shell
// out directly. The single production implementation wraps the Kamal CLI.
type DeploymentEngine interface {
	Deploy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error)
	Version(ctx context.Context) (string, error)
}

type CLIEngine struct {
	Executor *Executor
}

func NewCLIEngine() *CLIEngine {
	return &CLIEngine{Executor: NewExecutor()}
}

func (engine *CLIEngine) Deploy(ctx context.Context, request DeployRequest, stream func(line string)) (ExecResult, error) {
	if request.Rollback {
		return engine.Executor.Run(ctx, request.Workspace, []string{"rollback", request.Version}, stream)
	}
	// V1 deploys prebuilt registry images: nothing to build or push.
	return engine.Executor.Run(ctx, request.Workspace, []string{"deploy", "--skip-push"}, stream)
}

func (engine *CLIEngine) Version(ctx context.Context) (string, error) {
	return engine.Executor.Version(ctx)
}
