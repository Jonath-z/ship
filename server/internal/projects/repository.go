package projects

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

func (repository *Repository) List(ctx context.Context, cursor string, limit int) ([]migrations.Project, string, error) {
	query := repository.db.WithContext(ctx).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}

	var rows []migrations.Project
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list projects: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, id string) (migrations.Project, error) {
	var row migrations.Project
	if err := repository.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return migrations.Project{}, err
	}
	return row, nil
}

func (repository *Repository) FindForUpdate(ctx context.Context, id string) (migrations.Project, error) {
	var row migrations.Project
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", id).Error; err != nil {
		return migrations.Project{}, err
	}
	return row, nil
}

func (repository *Repository) Create(ctx context.Context, row *migrations.Project) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) Update(ctx context.Context, row *migrations.Project, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).First(row, "id = ?", row.ID).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.Project) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

func (repository *Repository) DeletionImpact(ctx context.Context, row migrations.Project) (DeletionImpact, error) {
	impact := DeletionImpact{ProjectID: row.ID, Slug: row.Slug}
	if err := repository.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("project_id = ?", row.ID).Count(&impact.Environments).Error; err != nil {
		return DeletionImpact{}, err
	}
	environmentIDs := repository.db.WithContext(ctx).Model(&migrations.Environment{}).
		Select("id").Where("project_id = ?", row.ID)
	for model, destination := range map[any]*int64{
		&migrations.ServerGroup{}:         &impact.ServerGroups,
		&migrations.Service{}:             &impact.Services,
		&migrations.Accessory{}:           &impact.Accessories,
		&migrations.Volume{}:              &impact.Volumes,
		&migrations.Domain{}:              &impact.Domains,
		&migrations.EnvironmentVariable{}: &impact.EnvironmentVariables,
		&migrations.Secret{}:              &impact.Secrets,
		&migrations.ServiceDependency{}:   &impact.Dependencies,
		&migrations.Configuration{}:       &impact.Configurations,
		&migrations.Deployment{}:          &impact.Deployments,
		&migrations.Backup{}:              &impact.Backups,
	} {
		if err := repository.db.WithContext(ctx).Model(model).
			Where("environment_id IN (?)", environmentIDs).Count(destination).Error; err != nil {
			return DeletionImpact{}, err
		}
	}
	return impact, nil
}
