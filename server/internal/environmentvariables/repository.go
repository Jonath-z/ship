package environmentvariables

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
	"github.com/Jonath-z/ship/server/migrations"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Transaction(ctx context.Context, operation func(*Repository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(NewRepository(tx))
	})
}

func (repository *Repository) EnvironmentExists(ctx context.Context, projectID, environmentID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) ServiceExists(ctx context.Context, environmentID, serviceID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Service{}).
		Where("id = ? AND environment_id = ?", serviceID, environmentID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) ListVariables(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.EnvironmentVariable, string, error) {
	query := repository.db.WithContext(ctx).Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.EnvironmentVariable
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list environment variables: %w", err)
	}
	return variablePage(rows, limit)
}

func variablePage(rows []migrations.EnvironmentVariable, limit int) ([]migrations.EnvironmentVariable, string, error) {
	if len(rows) <= limit {
		return rows, "", nil
	}
	rows = rows[:limit]
	last := rows[len(rows)-1]
	return rows, pagecursor.Encode(last.CreatedAt, last.ID), nil
}

func (repository *Repository) FindVariable(ctx context.Context, environmentID, variableID string) (migrations.EnvironmentVariable, error) {
	var row migrations.EnvironmentVariable
	err := repository.db.WithContext(ctx).First(&row, "id = ? AND environment_id = ?", variableID, environmentID).Error
	return row, err
}

func (repository *Repository) FindVariableByName(ctx context.Context, environmentID string, serviceID *string, name string) (migrations.EnvironmentVariable, error) {
	var row migrations.EnvironmentVariable
	query := scopeQuery(repository.db.WithContext(ctx).Model(&migrations.EnvironmentVariable{}), environmentID, serviceID)
	err := query.Where("name = ?", name).First(&row).Error
	return row, err
}

func (repository *Repository) CreateVariable(ctx context.Context, row *migrations.EnvironmentVariable) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) UpdateVariable(ctx context.Context, row *migrations.EnvironmentVariable, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).First(row, "id = ?", row.ID).Error
}

func (repository *Repository) DeleteVariable(ctx context.Context, row *migrations.EnvironmentVariable) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

func (repository *Repository) ListSecrets(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.Secret, string, error) {
	query := repository.db.WithContext(ctx).Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Secret
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list secrets: %w", err)
	}
	if len(rows) <= limit {
		return rows, "", nil
	}
	rows = rows[:limit]
	last := rows[len(rows)-1]
	return rows, pagecursor.Encode(last.CreatedAt, last.ID), nil
}

func (repository *Repository) FindSecret(ctx context.Context, environmentID, secretID string) (migrations.Secret, error) {
	var row migrations.Secret
	err := repository.db.WithContext(ctx).First(&row, "id = ? AND environment_id = ?", secretID, environmentID).Error
	return row, err
}

func (repository *Repository) FindSecretByName(ctx context.Context, environmentID string, serviceID *string, name string) (migrations.Secret, error) {
	var row migrations.Secret
	query := scopeQuery(repository.db.WithContext(ctx).Model(&migrations.Secret{}), environmentID, serviceID)
	err := query.Where("name = ?", name).First(&row).Error
	return row, err
}

func (repository *Repository) CreateSecret(ctx context.Context, row *migrations.Secret) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) UpdateSecret(ctx context.Context, row *migrations.Secret, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).First(row, "id = ?", row.ID).Error
}

func (repository *Repository) DeleteSecret(ctx context.Context, row *migrations.Secret) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

func (repository *Repository) VaultEntry(ctx context.Context, secretID string) (migrations.VaultEntry, error) {
	var row migrations.VaultEntry
	err := repository.db.WithContext(ctx).First(&row, "secret_id = ?", secretID).Error
	return row, err
}

func (repository *Repository) UpdateVaultName(ctx context.Context, vaultEntryID, name string) error {
	return repository.db.WithContext(ctx).Model(&migrations.VaultEntry{}).Where("id = ?", vaultEntryID).
		Updates(map[string]any{"name": name, "updated_at": time.Now().UTC()}).Error
}

func (repository *Repository) NameExists(ctx context.Context, environmentID string, serviceID *string, name, excludeID string) (bool, error) {
	for _, model := range []any{&migrations.EnvironmentVariable{}, &migrations.Secret{}} {
		query := scopeQuery(repository.db.WithContext(ctx).Model(model), environmentID, serviceID).Where("name = ?", name)
		if excludeID != "" {
			query = query.Where("id <> ?", excludeID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func scopeQuery(query *gorm.DB, environmentID string, serviceID *string) *gorm.DB {
	query = query.Where("environment_id = ?", environmentID)
	if serviceID == nil {
		return query.Where("service_id IS NULL")
	}
	return query.Where("service_id = ?", *serviceID)
}
