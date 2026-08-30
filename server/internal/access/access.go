// Package access defines Ship's install-scoped roles and permissions.
package access

import "github.com/gin-gonic/gin"

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleDeployer Role = "deployer"
	RoleViewer   Role = "viewer"
)

var Roles = []Role{RoleOwner, RoleAdmin, RoleDeployer, RoleViewer}

func ValidRole(role Role) bool {
	for _, candidate := range Roles {
		if role == candidate {
			return true
		}
	}
	return false
}

type Permission string

const (
	Public              Permission = "public"
	Setup               Permission = "setup"
	SystemRead          Permission = "system.read"
	ProjectsRead        Permission = "projects.read"
	ProjectsManage      Permission = "projects.manage"
	UsersRead           Permission = "users.read"
	UsersManage         Permission = "users.manage"
	ServersRead         Permission = "servers.read"
	ServersManage       Permission = "servers.manage"
	ConfigurationRead   Permission = "configuration.read"
	ConfigurationManage Permission = "configuration.manage"
	DeploymentsRead     Permission = "deployments.read"
	DeploymentsExecute  Permission = "deployments.execute"
	SecretsReadMetadata Permission = "secrets.metadata.read"
	SecretsManage       Permission = "secrets.manage"
	SecretsReveal       Permission = "secrets.reveal"
	AuditRead           Permission = "audit.read"
	SettingsManage      Permission = "settings.manage"
	EncryptionRotate    Permission = "encryption.rotate"
	Authenticated       Permission = "authenticated"
)

var grants = map[Role]map[Permission]struct{}{
	RoleOwner: allPermissions(),
	RoleAdmin: permissions(
		SystemRead, ProjectsRead, ProjectsManage, UsersRead, UsersManage, ServersRead, ServersManage,
		ConfigurationRead, ConfigurationManage, DeploymentsRead, DeploymentsExecute,
		SecretsReadMetadata, SecretsManage, SecretsReveal, AuditRead, SettingsManage,
	),
	RoleDeployer: permissions(
		SystemRead, ProjectsRead, ServersRead, ConfigurationRead, DeploymentsRead,
		DeploymentsExecute, SecretsReadMetadata,
	),
	RoleViewer: permissions(
		SystemRead, ProjectsRead, ServersRead, ConfigurationRead, DeploymentsRead,
		SecretsReadMetadata,
	),
}

func Allowed(role Role, permission Permission) bool {
	if permission == Public || permission == Setup || permission == Authenticated {
		return permission == Authenticated
	}
	_, ok := grants[role][permission]
	return ok
}

func permissions(values ...Permission) map[Permission]struct{} {
	result := make(map[Permission]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func allPermissions() map[Permission]struct{} {
	return permissions(
		SystemRead, ProjectsRead, ProjectsManage, UsersRead, UsersManage, ServersRead, ServersManage,
		ConfigurationRead, ConfigurationManage, DeploymentsRead, DeploymentsExecute,
		SecretsReadMetadata, SecretsManage, SecretsReveal, AuditRead, SettingsManage,
		EncryptionRotate,
	)
}

type Principal struct {
	UserID string
	Email  string
	Role   Role
}

const principalContextKey = "shipPrincipal"

func SetPrincipal(c *gin.Context, principal Principal) {
	c.Set(principalContextKey, principal)
}

func PrincipalFrom(c *gin.Context) (Principal, bool) {
	value, ok := c.Get(principalContextKey)
	if !ok {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}
