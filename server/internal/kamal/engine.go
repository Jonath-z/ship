package kamal

import (
	"context"

	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/domain"
)

// DeploymentEngine is the abstraction from spec §23. Everything above this
// interface — API handlers, job handlers, the UI — is Kamal-agnostic.
type DeploymentEngine interface {
	Validate(ctx context.Context, cfg configuration.DesiredState) error
	Render(ctx context.Context, cfg configuration.DesiredState) (workspacePath string, err error)
	Deploy(ctx context.Context, cfg configuration.DesiredState) (<-chan Event, error)
	Rollback(ctx context.Context, d domain.Deployment) (<-chan Event, error)
	Logs(ctx context.Context, s domain.Service) (<-chan LogLine, error)
}

// Event is a structured progress update parsed out of engine output (SH-063).
type Event struct {
	Phase   domain.DeploymentStatus
	Message string
}

type LogLine struct {
	Server string
	Text   string
}
