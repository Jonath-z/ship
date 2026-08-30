package access

import "testing"

func TestRolePermissionMatrix(t *testing.T) {
	tests := []struct {
		role       Role
		permission Permission
		allowed    bool
	}{
		{RoleOwner, EncryptionRotate, true},
		{RoleAdmin, SecretsReveal, true},
		{RoleAdmin, EncryptionRotate, false},
		{RoleDeployer, DeploymentsExecute, true},
		{RoleDeployer, SecretsReveal, false},
		{RoleViewer, SystemRead, true},
		{RoleViewer, DeploymentsExecute, false},
	}

	for _, test := range tests {
		if got := Allowed(test.role, test.permission); got != test.allowed {
			t.Fatalf("Allowed(%q, %q) = %v, want %v", test.role, test.permission, got, test.allowed)
		}
	}
}
