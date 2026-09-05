package configuration

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/migrations"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// EnvironmentRows is everything the compiler needs, loaded in one place.
type EnvironmentRows struct {
	ProjectSlug     string
	EnvironmentSlug string
	Services        []migrations.Service
	Accessories     []migrations.Accessory
	Groups          []migrations.ServerGroup
	GroupHosts      map[string][]string // group id -> host addresses
	ServerAddress   map[string]string   // server id -> host address (direct accessory placement)
	HostStatus      map[string]string   // host address -> server status
	Volumes         []migrations.Volume
	Domains         []migrations.Domain
	Variables       []migrations.EnvironmentVariable
	Secrets         []migrations.Secret
	SecretHasValue  map[string]bool // secret id -> vault entry exists
	Dependencies    []migrations.ServiceDependency
}

func (repository *Repository) EnvironmentExists(ctx context.Context, projectID, environmentID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) Load(ctx context.Context, environmentID string) (EnvironmentRows, error) {
	rows := EnvironmentRows{
		GroupHosts:     map[string][]string{},
		ServerAddress:  map[string]string{},
		HostStatus:     map[string]string{},
		SecretHasValue: map[string]bool{},
	}
	db := repository.db.WithContext(ctx)

	var environment migrations.Environment
	if err := db.Preload("Project").First(&environment, "id = ?", environmentID).Error; err != nil {
		return rows, fmt.Errorf("load environment: %w", err)
	}
	rows.ProjectSlug = environment.Project.Slug
	rows.EnvironmentSlug = environment.Slug

	byEnvironment := func(destination any, order string) error {
		return db.Where("environment_id = ?", environmentID).Order(order).Find(destination).Error
	}
	if err := byEnvironment(&rows.Services, "name ASC"); err != nil {
		return rows, fmt.Errorf("load services: %w", err)
	}
	if err := byEnvironment(&rows.Accessories, "name ASC"); err != nil {
		return rows, fmt.Errorf("load accessories: %w", err)
	}
	if err := byEnvironment(&rows.Groups, "name ASC"); err != nil {
		return rows, fmt.Errorf("load server groups: %w", err)
	}
	if err := byEnvironment(&rows.Volumes, "source ASC"); err != nil {
		return rows, fmt.Errorf("load volumes: %w", err)
	}
	if err := byEnvironment(&rows.Domains, "hostname ASC"); err != nil {
		return rows, fmt.Errorf("load domains: %w", err)
	}
	if err := byEnvironment(&rows.Variables, "name ASC"); err != nil {
		return rows, fmt.Errorf("load variables: %w", err)
	}
	if err := byEnvironment(&rows.Secrets, "name ASC"); err != nil {
		return rows, fmt.Errorf("load secrets: %w", err)
	}
	if err := byEnvironment(&rows.Dependencies, "created_at ASC, id ASC"); err != nil {
		return rows, fmt.Errorf("load dependencies: %w", err)
	}

	type membership struct {
		ServerGroupID string
		Hostname      string
		IPAddress     string
		Status        string
	}
	var memberships []membership
	err := db.Table("server_group_memberships").
		Select("server_group_memberships.server_group_id, servers.hostname, servers.ip_address, servers.status").
		Joins("JOIN servers ON servers.id = server_group_memberships.server_id").
		Joins("JOIN server_groups ON server_groups.id = server_group_memberships.server_group_id").
		Where("server_groups.environment_id = ?", environmentID).
		Order("servers.ip_address ASC, servers.hostname ASC").Scan(&memberships).Error
	if err != nil {
		return rows, fmt.Errorf("load server memberships: %w", err)
	}
	for _, member := range memberships {
		address := member.IPAddress
		if address == "" {
			address = member.Hostname
		}
		rows.GroupHosts[member.ServerGroupID] = append(rows.GroupHosts[member.ServerGroupID], address)
		rows.HostStatus[address] = member.Status
	}

	var placedServers []migrations.Server
	err = db.Where("id IN (SELECT server_id FROM accessories WHERE environment_id = ? AND server_id IS NOT NULL)", environmentID).
		Order("id ASC").Find(&placedServers).Error
	if err != nil {
		return rows, fmt.Errorf("load accessory servers: %w", err)
	}
	for _, server := range placedServers {
		address := server.IPAddress
		if address == "" {
			address = server.Hostname
		}
		rows.ServerAddress[server.ID] = address
		rows.HostStatus[address] = server.Status
	}

	var vaultSecretIDs []string
	err = db.Model(&migrations.VaultEntry{}).
		Where("secret_id IN (SELECT id FROM secrets WHERE environment_id = ?)", environmentID).
		Pluck("secret_id", &vaultSecretIDs).Error
	if err != nil {
		return rows, fmt.Errorf("load secret values: %w", err)
	}
	for _, id := range vaultSecretIDs {
		rows.SecretHasValue[id] = true
	}
	return rows, nil
}
