package migrations

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type Configuration struct {
	ID             string      `gorm:"type:text;primaryKey"`
	EnvironmentID  string      `gorm:"type:text;not null;uniqueIndex"`
	Environment    Environment `gorm:"constraint:OnDelete:CASCADE"`
	CurrentVersion int         `gorm:"not null;default:0;check:chk_configurations_current_version,current_version >= 0"`
	CreatedAt      time.Time   `gorm:"not null"`
	UpdatedAt      time.Time   `gorm:"not null"`
}

// ConfigurationVersion is an immutable desired-state snapshot.
type ConfigurationVersion struct {
	ID              string        `gorm:"type:text;primaryKey"`
	ConfigurationID string        `gorm:"type:text;not null;uniqueIndex:idx_configuration_version"`
	Configuration   Configuration `gorm:"constraint:OnDelete:CASCADE"`
	Version         int           `gorm:"not null;uniqueIndex:idx_configuration_version;check:chk_configuration_versions_version,version > 0"`
	Document        string        `gorm:"type:text;not null"`
	ActorUserID     *string       `gorm:"type:text;index"`
	ActorUser       *User         `gorm:"constraint:OnDelete:SET NULL"`
	ChangeSummary   string        `gorm:"not null;default:''"`
	CreatedAt       time.Time     `gorm:"not null"`
}

var ErrConfigurationVersionImmutable = errors.New("configuration versions are immutable")

func (*ConfigurationVersion) BeforeUpdate(*gorm.DB) error {
	return ErrConfigurationVersionImmutable
}

func (*ConfigurationVersion) BeforeDelete(*gorm.DB) error {
	return ErrConfigurationVersionImmutable
}

type Deployment struct {
	ID                     string                `gorm:"type:text;primaryKey"`
	EnvironmentID          string                `gorm:"type:text;not null;index"`
	Environment            Environment           `gorm:"constraint:OnDelete:CASCADE"`
	ServiceID              string                `gorm:"type:text;not null;index"`
	Service                Service               `gorm:"constraint:OnDelete:CASCADE"`
	ConfigurationVersionID *string               `gorm:"type:text;index"`
	ConfigurationVersion   *ConfigurationVersion `gorm:"constraint:OnDelete:CASCADE"`
	SourceDeploymentID     *string               `gorm:"type:text;index"`
	SourceDeployment       *Deployment           `gorm:"constraint:OnDelete:SET NULL"`
	CommitSHA              string                `gorm:"not null;default:''"`
	Image                  string                `gorm:"not null;default:''"`
	Status                 string                `gorm:"type:varchar(24);not null;default:QUEUED;index;check:chk_deployments_status,status IN ('QUEUED','VALIDATING','BUILDING','PUSHING','DEPLOYING','VERIFYING','SUCCESS','FAILED','ROLLING_BACK','ROLLED_BACK')"`
	StartedAt              *time.Time
	FinishedAt             *time.Time
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
}

type DeploymentLog struct {
	ID           string     `gorm:"type:text;primaryKey"`
	DeploymentID string     `gorm:"type:text;not null;uniqueIndex:idx_deployment_log_sequence"`
	Deployment   Deployment `gorm:"constraint:OnDelete:CASCADE"`
	Sequence     int64      `gorm:"not null;uniqueIndex:idx_deployment_log_sequence;check:chk_deployment_logs_sequence,sequence > 0"`
	Stream       string     `gorm:"type:varchar(16);not null;check:chk_deployment_logs_stream,stream IN ('stdout','stderr','system')"`
	Message      string     `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"not null;index"`
}

type Backup struct {
	ID              string       `gorm:"type:text;primaryKey"`
	Kind            string       `gorm:"type:varchar(24);not null;index;check:chk_backups_kind,kind IN ('control_plane','environment')"`
	EnvironmentID   *string      `gorm:"type:text;index;check:chk_backups_scope,(kind = 'control_plane' AND environment_id IS NULL) OR (kind = 'environment' AND environment_id IS NOT NULL)"`
	Environment     *Environment `gorm:"constraint:OnDelete:CASCADE"`
	StoragePath     string       `gorm:"not null;check:chk_backups_storage_path,length(trim(storage_path)) > 0"`
	Checksum        string       `gorm:"not null;default:''"`
	SizeBytes       int64        `gorm:"not null;default:0;check:chk_backups_size,size_bytes >= 0"`
	Status          string       `gorm:"type:varchar(16);not null;default:pending;index;check:chk_backups_status,status IN ('pending','completed','failed')"`
	CreatedByUserID *string      `gorm:"type:text;index"`
	CreatedByUser   *User        `gorm:"constraint:OnDelete:SET NULL"`
	CreatedAt       time.Time    `gorm:"not null"`
	CompletedAt     *time.Time
}
