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
	&VaultEntry{},
	&AuditLog{},
}

type User struct {
	ID           string     `gorm:"type:uuid;primaryKey"`
	Email        string     `gorm:"not null;uniqueIndex"`
	PasswordHash string     `gorm:"not null"`
	Role         string     `gorm:"type:varchar(16);not null;default:owner;check:chk_users_role,role IN ('owner','admin','deployer','viewer')"`
	DisabledAt   *time.Time `gorm:"index"`
	CreatedAt    time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Project struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"not null"`
	Slug      string    `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Environment struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	ProjectID string    `gorm:"type:uuid;not null;uniqueIndex:idx_environment_project_slug"`
	Project   Project   `gorm:"constraint:OnDelete:CASCADE"`
	Name      string    `gorm:"not null"`
	Slug      string    `gorm:"not null;uniqueIndex:idx_environment_project_slug"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// VaultEntry stores encrypted sensitive material. The plaintext and data key
// are never persisted; the data key is wrapped by an external master key.
type VaultEntry struct {
	ID            string    `gorm:"type:uuid;primaryKey"`
	Kind          string    `gorm:"type:varchar(32);not null;index:idx_vault_scope,priority:1"`
	ScopeType     string    `gorm:"type:varchar(32);not null;index:idx_vault_scope,priority:2"`
	ScopeID       string    `gorm:"type:uuid;not null;index:idx_vault_scope,priority:3"`
	Name          string    `gorm:"not null"`
	KeyID         string    `gorm:"not null;index"`
	FormatVersion int       `gorm:"not null;default:1"`
	Ciphertext    []byte    `gorm:"type:bytea;not null"`
	DataNonce     []byte    `gorm:"type:bytea;not null"`
	WrappedDEK    []byte    `gorm:"type:bytea;not null"`
	WrapNonce     []byte    `gorm:"type:bytea;not null"`
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// AuditLog is append-only at the application boundary. It deliberately has no
// UpdatedAt field and rejects update and delete operations through GORM.
type AuditLog struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	ActorUserID  *string   `gorm:"type:uuid;index"`
	ActorEmail   string    `gorm:"not null;default:''"`
	Action       string    `gorm:"not null;index"`
	ResourceType string    `gorm:"not null;index:idx_audit_resource,priority:1"`
	ResourceID   string    `gorm:"not null;default:'';index:idx_audit_resource,priority:2"`
	Outcome      string    `gorm:"type:varchar(16);not null"`
	SourceIP     string    `gorm:"not null;default:''"`
	RequestID    string    `gorm:"not null;default:'';index"`
	Metadata     string    `gorm:"type:jsonb;not null"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index"`
}

var ErrAuditLogImmutable = errors.New("audit log entries are immutable")

func (*AuditLog) BeforeUpdate(*gorm.DB) error {
	return ErrAuditLogImmutable
}

func (*AuditLog) BeforeDelete(*gorm.DB) error {
	return ErrAuditLogImmutable
}
