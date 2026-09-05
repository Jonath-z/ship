package migrations

import "time"

// SSHKey is a named keypair. The public key is stored here for display and
// installation on servers; the private key lives only in the encrypted vault
// (kind ssh_private_key) and is never returned by the API.
type SSHKey struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	Name         string    `gorm:"type:varchar(100);not null;uniqueIndex;check:chk_ssh_keys_name,length(btrim(name)) BETWEEN 1 AND 100"`
	PublicKey    string    `gorm:"not null;check:chk_ssh_keys_public_key,length(btrim(public_key)) > 0"`
	VaultEntryID string    `gorm:"type:uuid;not null"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// Server is intentionally independent of an environment. Server groups place
// reusable machines into an environment-specific Kamal role.
type Server struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	Name         string    `gorm:"type:varchar(100);not null;uniqueIndex;check:chk_servers_name,length(btrim(name)) BETWEEN 1 AND 100"`
	Hostname     string    `gorm:"not null;default:''"`
	IPAddress    string    `gorm:"not null;default:'';check:chk_servers_address,length(btrim(hostname)) > 0 OR length(btrim(ip_address)) > 0"`
	SSHUser      string    `gorm:"not null;default:root;check:chk_servers_ssh_user,length(btrim(ssh_user)) > 0"`
	SSHPort      int       `gorm:"not null;default:22;check:chk_servers_ssh_port,ssh_port BETWEEN 1 AND 65535"`
	SSHKeyID     *string   `gorm:"type:uuid;index"`
	SSHKey       *SSHKey   `gorm:"constraint:OnDelete:RESTRICT"`
	HostKey      string    `gorm:"not null;default:''"`
	Architecture string    `gorm:"not null;default:''"`
	OS           string    `gorm:"not null;default:''"`
	Status       string    `gorm:"type:varchar(16);not null;default:pending;index;check:chk_servers_status,status IN ('pending','connected','disconnected','degraded')"`
	Resources    string    `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type ServerGroup struct {
	ID            string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID string      `gorm:"type:uuid;not null;uniqueIndex:idx_server_group_environment_name"`
	Environment   Environment `gorm:"constraint:OnDelete:CASCADE"`
	Name          string      `gorm:"type:varchar(63);not null;uniqueIndex:idx_server_group_environment_name;check:chk_server_groups_name,name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type ServerGroupMembership struct {
	ServerGroupID string      `gorm:"type:uuid;primaryKey"`
	ServerGroup   ServerGroup `gorm:"constraint:OnDelete:CASCADE"`
	ServerID      string      `gorm:"type:uuid;primaryKey"`
	Server        Server      `gorm:"constraint:OnDelete:CASCADE"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Service struct {
	ID            string       `gorm:"type:uuid;primaryKey"`
	EnvironmentID string       `gorm:"type:uuid;not null;uniqueIndex:idx_service_environment_name"`
	Environment   Environment  `gorm:"constraint:OnDelete:CASCADE"`
	ServerGroupID *string      `gorm:"type:uuid;index"`
	ServerGroup   *ServerGroup `gorm:"constraint:OnDelete:SET NULL"`
	Name          string       `gorm:"type:varchar(100);not null;uniqueIndex:idx_service_environment_name;check:chk_services_name,length(btrim(name)) BETWEEN 1 AND 100"`
	Type          string       `gorm:"type:varchar(32);not null;check:chk_services_type,length(btrim(type)) > 0"`
	Repository    string       `gorm:"not null;default:''"`
	Branch        string       `gorm:"not null;default:''"`
	Image         string       `gorm:"not null;default:'';check:chk_services_source,length(btrim(repository)) > 0 OR length(btrim(image)) > 0"`
	Port          *int         `gorm:"check:chk_services_port,port IS NULL OR port BETWEEN 1 AND 65535"`
	Command       string       `gorm:"not null;default:''"`
	CreatedAt     time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Accessory struct {
	ID            string       `gorm:"type:uuid;primaryKey"`
	EnvironmentID string       `gorm:"type:uuid;not null;uniqueIndex:idx_accessory_environment_name"`
	Environment   Environment  `gorm:"constraint:OnDelete:CASCADE"`
	ServerID      *string      `gorm:"type:uuid;index"`
	Server        *Server      `gorm:"constraint:OnDelete:SET NULL"`
	ServerGroupID *string      `gorm:"type:uuid;index;check:chk_accessories_placement,server_id IS NULL OR server_group_id IS NULL"`
	ServerGroup   *ServerGroup `gorm:"constraint:OnDelete:SET NULL"`
	Name          string       `gorm:"type:varchar(100);not null;uniqueIndex:idx_accessory_environment_name;check:chk_accessories_name,length(btrim(name)) BETWEEN 1 AND 100"`
	Type          string       `gorm:"type:varchar(16);not null;check:chk_accessories_type,type IN ('postgres','redis')"`
	Image         string       `gorm:"not null;check:chk_accessories_image,length(btrim(image)) > 0"`
	Port          *int         `gorm:"check:chk_accessories_port,port IS NULL OR port BETWEEN 1 AND 65535"`
	CreatedAt     time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time    `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Volume struct {
	ID            string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID string      `gorm:"type:uuid;not null;uniqueIndex:idx_volume_environment_source"`
	Environment   Environment `gorm:"constraint:OnDelete:CASCADE"`
	ServiceID     *string     `gorm:"type:uuid;index"`
	Service       *Service    `gorm:"constraint:OnDelete:CASCADE"`
	AccessoryID   *string     `gorm:"type:uuid;index;check:chk_volumes_owner,(service_id IS NULL) <> (accessory_id IS NULL)"`
	Accessory     *Accessory  `gorm:"constraint:OnDelete:CASCADE"`
	Name          string      `gorm:"type:varchar(100);not null;check:chk_volumes_name,length(btrim(name)) BETWEEN 1 AND 100"`
	Source        string      `gorm:"not null;uniqueIndex:idx_volume_environment_source;check:chk_volumes_source,length(btrim(source)) > 0"`
	Destination   string      `gorm:"not null;check:chk_volumes_destination,destination LIKE '/%'"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Domain struct {
	ID            string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID string      `gorm:"type:uuid;not null;uniqueIndex:idx_domain_environment_hostname"`
	Environment   Environment `gorm:"constraint:OnDelete:CASCADE"`
	ServiceID     string      `gorm:"type:uuid;not null;index"`
	Service       Service     `gorm:"constraint:OnDelete:CASCADE"`
	Hostname      string      `gorm:"type:varchar(253);not null;uniqueIndex:idx_domain_environment_hostname;check:chk_domains_hostname,hostname = lower(hostname) AND hostname ~ '^[a-z0-9.-]+$' AND hostname NOT LIKE '.%' AND hostname NOT LIKE '%.' AND hostname NOT LIKE '%..%'"`
	SSLEnabled    bool        `gorm:"not null;default:false"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type EnvironmentVariable struct {
	ID            string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID string      `gorm:"type:uuid;not null;uniqueIndex:idx_environment_variable_global_name,where:service_id IS NULL,priority:1;uniqueIndex:idx_environment_variable_service_name,where:service_id IS NOT NULL,priority:1"`
	Environment   Environment `gorm:"constraint:OnDelete:CASCADE"`
	ServiceID     *string     `gorm:"type:uuid;index;uniqueIndex:idx_environment_variable_service_name,where:service_id IS NOT NULL,priority:2"`
	Service       *Service    `gorm:"constraint:OnDelete:CASCADE"`
	Name          string      `gorm:"type:varchar(128);not null;uniqueIndex:idx_environment_variable_global_name,where:service_id IS NULL,priority:2;uniqueIndex:idx_environment_variable_service_name,where:service_id IS NOT NULL,priority:3;check:chk_environment_variables_name,name ~ '^[A-Z_][A-Z0-9_]*$'"`
	Value         string      `gorm:"not null"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Secret struct {
	ID            string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID string      `gorm:"type:uuid;not null;uniqueIndex:idx_secret_global_name,where:service_id IS NULL,priority:1;uniqueIndex:idx_secret_service_name,where:service_id IS NOT NULL,priority:1"`
	Environment   Environment `gorm:"constraint:OnDelete:CASCADE"`
	ServiceID     *string     `gorm:"type:uuid;index;uniqueIndex:idx_secret_service_name,where:service_id IS NOT NULL,priority:2"`
	Service       *Service    `gorm:"constraint:OnDelete:CASCADE"`
	Name          string      `gorm:"type:varchar(128);not null;uniqueIndex:idx_secret_global_name,where:service_id IS NULL,priority:2;uniqueIndex:idx_secret_service_name,where:service_id IS NOT NULL,priority:3;check:chk_secrets_name,name ~ '^[A-Z_][A-Z0-9_]*$'"`
	CreatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type ServiceDependency struct {
	ID                string      `gorm:"type:uuid;primaryKey"`
	EnvironmentID     string      `gorm:"type:uuid;not null;index"`
	Environment       Environment `gorm:"constraint:OnDelete:CASCADE"`
	SourceServiceID   string      `gorm:"type:uuid;not null;uniqueIndex:idx_service_dependency_service_target,where:target_service_id IS NOT NULL,priority:1;uniqueIndex:idx_service_dependency_accessory_target,where:target_accessory_id IS NOT NULL,priority:1"`
	SourceService     Service     `gorm:"foreignKey:SourceServiceID;constraint:OnDelete:CASCADE"`
	TargetServiceID   *string     `gorm:"type:uuid;uniqueIndex:idx_service_dependency_service_target,where:target_service_id IS NOT NULL,priority:2"`
	TargetService     *Service    `gorm:"foreignKey:TargetServiceID;constraint:OnDelete:CASCADE"`
	TargetAccessoryID *string     `gorm:"type:uuid;uniqueIndex:idx_service_dependency_accessory_target,where:target_accessory_id IS NOT NULL,priority:2;check:chk_service_dependencies_target,(target_service_id IS NULL) <> (target_accessory_id IS NULL)"`
	TargetAccessory   *Accessory  `gorm:"constraint:OnDelete:CASCADE"`
	Type              string      `gorm:"type:varchar(32);not null;default:runtime;uniqueIndex:idx_service_dependency_service_target,where:target_service_id IS NOT NULL,priority:3;uniqueIndex:idx_service_dependency_accessory_target,where:target_accessory_id IS NOT NULL,priority:3;check:chk_service_dependencies_type,length(btrim(type)) > 0"`
	CreatedAt         time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP"`
}
