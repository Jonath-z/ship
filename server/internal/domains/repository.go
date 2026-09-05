package domains

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

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
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

func (repository *Repository) List(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.Domain, string, error) {
	query := repository.db.WithContext(ctx).Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Domain
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list domains: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, environmentID, domainID string) (migrations.Domain, error) {
	var row migrations.Domain
	err := repository.db.WithContext(ctx).First(&row, "id = ? AND environment_id = ?", domainID, environmentID).Error
	return row, err
}

func (repository *Repository) Create(ctx context.Context, row *migrations.Domain) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) Update(ctx context.Context, row *migrations.Domain, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).First(row, "id = ?", row.ID).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.Domain) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}
