package kamal

import "strings"

// Phase is the coarse progress classification of SH-063 (V1 simplification:
// build/deploy/done, no fine-grained sub-phases). Unrecognized output degrades
// to a plain log line with PhaseUnknown.
type Phase string

const (
	PhaseUnknown   Phase = ""
	PhaseBuilding  Phase = "BUILDING"
	PhasePushing   Phase = "PUSHING"
	PhaseDeploying Phase = "DEPLOYING"
	PhaseVerifying Phase = "VERIFYING"
)

// ClassifyLine maps a Kamal output line onto a coarse phase transition.
func ClassifyLine(line string) Phase {
	lowered := strings.ToLower(line)
	switch {
	case strings.Contains(lowered, "building") && strings.Contains(lowered, "docker"):
		return PhaseBuilding
	case strings.Contains(lowered, "pushing") || strings.Contains(lowered, "docker push"):
		return PhasePushing
	case strings.Contains(lowered, "running docker") && strings.Contains(lowered, "run"):
		return PhaseDeploying
	case strings.Contains(lowered, "container is healthy") || strings.Contains(lowered, "readiness"):
		return PhaseVerifying
	default:
		return PhaseUnknown
	}
}
