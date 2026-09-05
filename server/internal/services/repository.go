package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
	"github.com/Jonath-z/ship/server/migrations"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

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

func (repository *Repository) List(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.Service, string, error) {
	query := repository.db.WithContext(ctx).Preload("ServerGroup").
		Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Service
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list services: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, environmentID, serviceID string) (migrations.Service, error) {
	var row migrations.Service
	if err := repository.db.WithContext(ctx).Preload("ServerGroup").
		First(&row, "id = ? AND environment_id = ?", serviceID, environmentID).Error; err != nil {
		return migrations.Service{}, err
	}
	return row, nil
}

func (repository *Repository) FindForUpdate(ctx context.Context, environmentID, serviceID string) (migrations.Service, error) {
	var row migrations.Service
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row, "id = ? AND environment_id = ?", serviceID, environmentID).Error; err != nil {
		return migrations.Service{}, err
	}
	return row, nil
}

func (repository *Repository) EnsureRole(ctx context.Context, environmentID, role string) (migrations.ServerGroup, error) {
	var group migrations.ServerGroup
	err := repository.db.WithContext(ctx).First(&group, "environment_id = ? AND name = ?", environmentID, role).Error
	if err == nil {
		return group, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.ServerGroup{}, err
	}
	id, err := identity.New()
	if err != nil {
		return migrations.ServerGroup{}, err
	}
	group = migrations.ServerGroup{ID: id, EnvironmentID: environmentID, Name: role}
	if err := repository.db.WithContext(ctx).Create(&group).Error; err != nil {
		return migrations.ServerGroup{}, err
	}
	return group, nil
}

func (repository *Repository) Create(ctx context.Context, row *migrations.Service) error {
	return repository.db.WithContext(ctx).Omit(clause.Associations).Create(row).Error
}

func (repository *Repository) Update(ctx context.Context, row *migrations.Service, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Preload("ServerGroup").First(row, "id = ?", row.ID).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.Service) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

// DependentServiceNames lists services that declare a dependency on the given
// service, ordered by name.
func (repository *Repository) DependentServiceNames(ctx context.Context, environmentID, serviceID string) ([]string, error) {
	var names []string
	err := repository.db.WithContext(ctx).Model(&migrations.ServiceDependency{}).
		Distinct("services.name").
		Joins("JOIN services ON services.id = service_dependencies.source_service_id").
		Where("service_dependencies.environment_id = ? AND service_dependencies.target_service_id = ?", environmentID, serviceID).
		Order("services.name ASC").Pluck("services.name", &names).Error
	return names, err
}
