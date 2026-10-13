// Package migrations defines the database schema as Go structs.
package migrations

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Models is the schema passed to GORM's migration tools.
var Models = []any{
	&User{},
	&Project{},
	&Environment{},
	&SSHKey{},
	&Server{},
	&ServerGroup{},
	&ServerGroupMembership{},
	&Service{},
	&Accessory{},
	&Volume{},
	&Domain{},
	&EnvironmentVariable{},
	&Secret{},
	&VaultEntry{},
	&ServiceDependency{},
	&Configuration{},
	&ConfigurationVersion{},
	&Deployment{},
	&DeploymentLog{},
	&Backup{},
	&AuditLog{},
}

type User struct {
	ID           string     `gorm:"type:text;primaryKey"`
	Email        string     `gorm:"not null;uniqueIndex"`
	PasswordHash string     `gorm:"not null"`
	Role         string     `gorm:"type:varchar(16);not null;default:owner;check:chk_users_role,role IN ('owner','admin','deployer','viewer')"`
	DisabledAt   *time.Time `gorm:"index"`
	CreatedAt    time.Time  `gorm:"not null"`
	UpdatedAt    time.Time  `gorm:"not null"`
}

type Project struct {
	ID        string    `gorm:"type:text;primaryKey"`
	Name      string    `gorm:"type:varchar(100);not null;check:chk_projects_name,length(trim(name)) BETWEEN 1 AND 100"`
	Slug      string    `gorm:"type:varchar(63);not null;uniqueIndex;check:chk_projects_slug,length(slug) > 0 AND slug NOT GLOB '*[^a-z0-9-]*' AND slug NOT LIKE '-%' AND slug NOT LIKE '%-' AND slug NOT LIKE '%--%'"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

type Environment struct {
	ID        string    `gorm:"type:text;primaryKey"`
	ProjectID string    `gorm:"type:text;not null;uniqueIndex:idx_environment_project_slug"`
	Project   Project   `gorm:"constraint:OnDelete:CASCADE"`
	Name      string    `gorm:"type:varchar(100);not null;check:chk_environments_name,length(trim(name)) BETWEEN 1 AND 100"`
	Slug      string    `gorm:"type:varchar(63);not null;uniqueIndex:idx_environment_project_slug;check:chk_environments_slug,length(slug) > 0 AND slug NOT GLOB '*[^a-z0-9-]*' AND slug NOT LIKE '-%' AND slug NOT LIKE '%-' AND slug NOT LIKE '%--%'"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// VaultEntry stores encrypted sensitive material. The plaintext and data key
// are never persisted; the data key is wrapped by an external master key.
type VaultEntry struct {
	ID            string    `gorm:"type:text;primaryKey"`
	SecretID      *string   `gorm:"type:text;uniqueIndex"`
	Secret        *Secret   `gorm:"constraint:OnDelete:CASCADE"`
	Kind          string    `gorm:"type:varchar(32);not null;index:idx_vault_scope,priority:1"`
	ScopeType     string    `gorm:"type:varchar(32);not null;index:idx_vault_scope,priority:2"`
	ScopeID       string    `gorm:"type:text;not null;index:idx_vault_scope,priority:3"`
	Name          string    `gorm:"not null"`
	KeyID         string    `gorm:"not null;index"`
	FormatVersion int       `gorm:"not null;default:1"`
	Ciphertext    []byte    `gorm:"type:blob;not null"`
	DataNonce     []byte    `gorm:"type:blob;not null"`
	WrappedDEK    []byte    `gorm:"type:blob;not null"`
	WrapNonce     []byte    `gorm:"type:blob;not null"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

// AuditLog is append-only at the application boundary. It deliberately has no
// UpdatedAt field and rejects update and delete operations through GORM.
type AuditLog struct {
	ID           string    `gorm:"type:text;primaryKey"`
	ActorUserID  *string   `gorm:"type:text;index"`
	ActorUser    *User     `gorm:"constraint:OnDelete:SET NULL"`
	ActorEmail   string    `gorm:"not null;default:''"`
	Action       string    `gorm:"not null;index"`
	ResourceType string    `gorm:"not null;index:idx_audit_resource,priority:1"`
	ResourceID   string    `gorm:"not null;default:'';index:idx_audit_resource,priority:2"`
	Outcome      string    `gorm:"type:varchar(16);not null;check:chk_audit_logs_outcome,outcome IN ('success','failure')"`
	SourceIP     string    `gorm:"not null;default:''"`
	RequestID    string    `gorm:"not null;default:'';index"`
	Metadata     string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"not null;index"`
}

var ErrAuditLogImmutable = errors.New("audit log entries are immutable")

func (*AuditLog) BeforeUpdate(*gorm.DB) error {
	return ErrAuditLogImmutable
}

func (*AuditLog) BeforeDelete(*gorm.DB) error {
	return ErrAuditLogImmutable
}
