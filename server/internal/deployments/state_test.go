package deployments

import "testing"

// SH-070 acceptance: the transition table is unit-tested exhaustively —
// every ordered pair of statuses is asserted as legal or illegal.
func TestTransitionTableExhaustively(t *testing.T) {
	all := []Status{
		StatusQueued, StatusValidating, StatusBuilding, StatusPushing, StatusDeploying,
		StatusVerifying, StatusSuccess, StatusFailed, StatusRollingBack, StatusRolledBack,
	}
	legal := map[[2]Status]bool{
		{StatusQueued, StatusValidating}:      true,
		{StatusQueued, StatusFailed}:          true,
		{StatusValidating, StatusBuilding}:    true,
		{StatusValidating, StatusPushing}:     true,
		{StatusValidating, StatusDeploying}:   true,
		{StatusValidating, StatusRollingBack}: true,
		{StatusValidating, StatusFailed}:      true,
		{StatusBuilding, StatusPushing}:       true,
		{StatusBuilding, StatusFailed}:        true,
		{StatusPushing, StatusDeploying}:      true,
		{StatusPushing, StatusFailed}:         true,
		{StatusDeploying, StatusVerifying}:    true,
		{StatusDeploying, StatusSuccess}:      true,
		{StatusDeploying, StatusFailed}:       true,
		{StatusVerifying, StatusSuccess}:      true,
		{StatusVerifying, StatusFailed}:       true,
		{StatusRollingBack, StatusRolledBack}: true,
		{StatusRollingBack, StatusFailed}:     true,
	}
	for _, from := range all {
		for _, to := range all {
			expected := legal[[2]Status{from, to}]
			if CanTransition(from, to) != expected {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, !expected, expected)
			}
		}
	}
	for _, status := range []Status{StatusSuccess, StatusFailed, StatusRolledBack} {
		if !status.Terminal() {
			t.Errorf("%s should be terminal", status)
		}
	}
}
