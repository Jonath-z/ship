// Package domain holds the shared vocabulary of Ship: the entities every other
// package agrees on. It has no database, HTTP, SSH, or Kamal dependencies.
//
// Rule: if a type is used by more than one feature package, it belongs here.
// If it is an internal detail of one package, it does not.
package domain

import "time"

type (
	ProjectID     string
	EnvironmentID string
	ServiceID     string
	ServerID      string
	AccessoryID   string
	DeploymentID  string
)

// Project is an application system (spec §8).
type Project struct {
	ID        ProjectID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Environment is a deployment boundary owning its own infrastructure config (§9).
type Environment struct {
	ID        EnvironmentID
	ProjectID ProjectID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Service is the primary deployable unit (§10).
type Service struct {
	ID            ServiceID
	EnvironmentID EnvironmentID
	Name          string
	Type          string
	Repository    string
	Branch        string
	Image         string
	Port          int
	Command       string
	Role          string // resolves to a group of servers (§12)
}

// Server is infrastructure Ship can manage over SSH (§11).
type Server struct {
	ID           ServerID
	Name         string
	Hostname     string
	IPAddress    string
	SSHUser      string
	SSHKeyID     string
	Architecture string
	OS           string
	Status       ServerStatus
	Resources    Resources
}

type ServerStatus string

const (
	ServerPending      ServerStatus = "pending"
	ServerConnected    ServerStatus = "connected"
	ServerDisconnected ServerStatus = "disconnected"
	ServerDegraded     ServerStatus = "degraded"
)

type Resources struct {
	CPUCores int
	MemoryMB int
	DiskGB   int
}

// Accessory is a supporting service such as PostgreSQL or Redis (§13).
type Accessory struct {
	ID            AccessoryID
	EnvironmentID EnvironmentID
	Name          string
	Type          string
	Image         string
	ServerID      ServerID
	Port          int
}

// DeploymentStatus is the state machine from §25.
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

// Deployment records one execution against one configuration version (§26).
type Deployment struct {
	ID                   DeploymentID
	EnvironmentID        EnvironmentID
	ServiceID            ServiceID
	CommitSHA            string
	ConfigurationVersion int
	Status               DeploymentStatus
	StartedAt            time.Time
	FinishedAt           *time.Time
}
