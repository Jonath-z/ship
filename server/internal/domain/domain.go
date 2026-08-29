// Package domain holds Ship's shared entities without persistence or transport concerns.
package domain

import "time"

type (
	ProjectID       string
	EnvironmentID   string
	ServiceID       string
	ServerID        string
	AccessoryID     string
	DomainID        string
	VolumeID        string
	EnvVarID        string
	SecretID        string
	DeploymentID    string
	ConfigurationID string
)

type Project struct {
	ID        ProjectID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Environment struct {
	ID        EnvironmentID `json:"id"`
	ProjectID ProjectID     `json:"projectId"`
	Name      string        `json:"name"`
	Slug      string        `json:"slug"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type Service struct {
	ID            ServiceID     `json:"id"`
	EnvironmentID EnvironmentID `json:"environmentId"`
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	Repository    string        `json:"repository,omitempty"`
	Branch        string        `json:"branch,omitempty"`
	Image         string        `json:"image,omitempty"`
	Port          int           `json:"port"`
	Command       string        `json:"command,omitempty"`
	Role          string        `json:"role"`
}

type ServerStatus string

const (
	ServerPending      ServerStatus = "pending"
	ServerConnected    ServerStatus = "connected"
	ServerDisconnected ServerStatus = "disconnected"
	ServerDegraded     ServerStatus = "degraded"
)

type Resources struct {
	CPUCores int `json:"cpuCores"`
	MemoryMB int `json:"memoryMb"`
	DiskGB   int `json:"diskGb"`
}

type Server struct {
	ID           ServerID     `json:"id"`
	Name         string       `json:"name"`
	Hostname     string       `json:"hostname"`
	IPAddress    string       `json:"ipAddress"`
	SSHUser      string       `json:"sshUser"`
	SSHKeyID     string       `json:"sshKeyId"`
	Architecture string       `json:"architecture"`
	OS           string       `json:"os"`
	Status       ServerStatus `json:"status"`
	Resources    Resources    `json:"resources"`
}

type Accessory struct {
	ID            AccessoryID   `json:"id"`
	EnvironmentID EnvironmentID `json:"environmentId"`
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	Image         string        `json:"image"`
	ServerID      ServerID      `json:"serverId"`
	Port          int           `json:"port"`
}

type Domain struct {
	ID         DomainID  `json:"id"`
	ServiceID  ServiceID `json:"serviceId"`
	Hostname   string    `json:"hostname"`
	SSLEnabled bool      `json:"sslEnabled"`
}

type Volume struct {
	ID            VolumeID      `json:"id"`
	EnvironmentID EnvironmentID `json:"environmentId"`
	ServiceID     *ServiceID    `json:"serviceId,omitempty"`
	AccessoryID   *AccessoryID  `json:"accessoryId,omitempty"`
	Name          string        `json:"name"`
	Source        string        `json:"source"`
	Destination   string        `json:"destination"`
}

type EnvVar struct {
	ID            EnvVarID      `json:"id"`
	EnvironmentID EnvironmentID `json:"environmentId"`
	ServiceID     *ServiceID    `json:"serviceId,omitempty"`
	Name          string        `json:"name"`
	Value         string        `json:"value"`
}

// Secret stores ciphertext only; plaintext is deliberately absent.
type Secret struct {
	ID             SecretID      `json:"id"`
	EnvironmentID  EnvironmentID `json:"environmentId"`
	ServiceID      *ServiceID    `json:"serviceId,omitempty"`
	Name           string        `json:"name"`
	EncryptedValue []byte        `json:"-"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

// Configuration is an immutable desired-state snapshot.
type Configuration struct {
	ID            ConfigurationID `json:"id"`
	EnvironmentID EnvironmentID   `json:"environmentId"`
	Version       int             `json:"version"`
	Document      []byte          `json:"document"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type DeploymentStatus string

const (
	DeploymentQueued      DeploymentStatus = "QUEUED"
	DeploymentValidating  DeploymentStatus = "VALIDATING"
	DeploymentBuilding    DeploymentStatus = "BUILDING"
	DeploymentPushing     DeploymentStatus = "PUSHING"
	DeploymentDeploying   DeploymentStatus = "DEPLOYING"
	DeploymentVerifying   DeploymentStatus = "VERIFYING"
	DeploymentSuccess     DeploymentStatus = "SUCCESS"
	DeploymentFailed      DeploymentStatus = "FAILED"
	DeploymentRollingBack DeploymentStatus = "ROLLING_BACK"
	DeploymentRolledBack  DeploymentStatus = "ROLLED_BACK"
)

type Deployment struct {
	ID                   DeploymentID     `json:"id"`
	EnvironmentID        EnvironmentID    `json:"environmentId"`
	ServiceID            ServiceID        `json:"serviceId"`
	CommitSHA            string           `json:"commitSha"`
	ConfigurationVersion int              `json:"configurationVersion"`
	Status               DeploymentStatus `json:"status"`
	StartedAt            time.Time        `json:"startedAt"`
	FinishedAt           *time.Time       `json:"finishedAt,omitempty"`
}
