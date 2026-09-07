// Package deployments implements the deployment state machine, records,
// history, and orchestration pipeline (SH-070…SH-077).
package deployments

import "fmt"

// Status values follow spec §25. BUILDING and PUSHING are part of the enum
// for the post-V1 build pipeline; V1 registry-image deploys skip them.
type Status string

const (
	StatusQueued      Status = "QUEUED"
	StatusValidating  Status = "VALIDATING"
	StatusBuilding    Status = "BUILDING"
	StatusPushing     Status = "PUSHING"
	StatusDeploying   Status = "DEPLOYING"
	StatusVerifying   Status = "VERIFYING"
	StatusSuccess     Status = "SUCCESS"
	StatusFailed      Status = "FAILED"
	StatusRollingBack Status = "ROLLING_BACK"
	StatusRolledBack  Status = "ROLLED_BACK"
)

// transitions is the complete legal-transition table; anything absent is
// rejected at the persistence layer.
var transitions = map[Status][]Status{
	StatusQueued:      {StatusValidating, StatusFailed},
	StatusValidating:  {StatusBuilding, StatusPushing, StatusDeploying, StatusRollingBack, StatusFailed},
	StatusBuilding:    {StatusPushing, StatusFailed},
	StatusPushing:     {StatusDeploying, StatusFailed},
	StatusDeploying:   {StatusVerifying, StatusSuccess, StatusFailed},
	StatusVerifying:   {StatusSuccess, StatusFailed},
	StatusRollingBack: {StatusRolledBack, StatusFailed},
	StatusSuccess:     {},
	StatusFailed:      {},
	StatusRolledBack:  {},
}

func (status Status) Terminal() bool {
	return status == StatusSuccess || status == StatusFailed || status == StatusRolledBack
}

func CanTransition(from, to Status) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ErrIllegalTransition is returned when a status change violates the table.
type ErrIllegalTransition struct {
	From, To Status
}

func (err *ErrIllegalTransition) Error() string {
	return fmt.Sprintf("illegal deployment transition %s -> %s", err.From, err.To)
}
