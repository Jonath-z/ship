package registry

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/configuration"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrNotConfigured       = errors.New("registry credentials are not configured")
)

// Service resolves the environment's registry credentials (the same
// KAMAL_REGISTRY_* convention the renderer uses) and browses the registry
// with them. Credential values never appear in responses.
type Service struct {
	db     *gorm.DB
	vault  *shipcrypto.Vault
	client *Client
}

func NewService(db *gorm.DB, vault *shipcrypto.Vault) *Service {
	return &Service{db: db, vault: vault, client: NewClient()}
}

func (service *Service) Repositories(ctx context.Context, projectID, environmentID string) ([]string, error) {
	credentials, err := service.credentials(ctx, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	return service.client.Repositories(ctx, credentials)
}

func (service *Service) Tags(ctx context.Context, projectID, environmentID, repository string) ([]string, error) {
	credentials, err := service.credentials(ctx, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	return service.client.Tags(ctx, credentials, repository)
}

func (service *Service) credentials(ctx context.Context, projectID, environmentID string) (Credentials, error) {
	var environments int64
	err := service.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&environments).Error
	if err != nil {
		return Credentials{}, fmt.Errorf("find environment: %w", err)
	}
	if environments == 0 {
		return Credentials{}, ErrEnvironmentNotFound
	}

	var variables []migrations.EnvironmentVariable
	err = service.db.WithContext(ctx).
		Where("environment_id = ? AND service_id IS NULL AND name IN ?", environmentID,
			[]string{configuration.RegistryServerVar, configuration.RegistryUsernameKey}).
		Find(&variables).Error
	if err != nil {
		return Credentials{}, fmt.Errorf("load registry variables: %w", err)
	}
	credentials := Credentials{}
	for _, variable := range variables {
		switch variable.Name {
		case configuration.RegistryServerVar:
			credentials.Server = variable.Value
		case configuration.RegistryUsernameKey:
			credentials.Username = variable.Value
		}
	}

	var secrets []migrations.Secret
	err = service.db.WithContext(ctx).
		Where("environment_id = ? AND service_id IS NULL AND name IN ?", environmentID,
			[]string{configuration.RegistryUsernameKey, configuration.RegistryPasswordKey}).
		Find(&secrets).Error
	if err != nil {
		return Credentials{}, fmt.Errorf("load registry secrets: %w", err)
	}
	for _, secret := range secrets {
		var entry migrations.VaultEntry
		err := service.db.WithContext(ctx).First(&entry, "secret_id = ?", secret.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return Credentials{}, fmt.Errorf("find vault entry: %w", err)
		}
		plaintext, err := service.vault.Reveal(ctx, entry.ID)
		if err != nil {
			return Credentials{}, fmt.Errorf("decrypt registry secret: %w", err)
		}
		switch secret.Name {
		case configuration.RegistryUsernameKey:
			credentials.Username = string(plaintext)
		case configuration.RegistryPasswordKey:
			credentials.Password = string(plaintext)
		}
	}

	if credentials.Password == "" {
		return Credentials{}, ErrNotConfigured
	}
	return credentials, nil
}
