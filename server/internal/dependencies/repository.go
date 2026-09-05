package dependencies

import (
	"context"
	"fmt"

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

func (repository *Repository) AccessoryExists(ctx context.Context, environmentID, accessoryID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Accessory{}).
		Where("id = ? AND environment_id = ?", accessoryID, environmentID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) List(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.ServiceDependency, string, error) {
	query := repository.db.WithContext(ctx).Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.ServiceDependency
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list dependencies: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, environmentID, dependencyID string) (migrations.ServiceDependency, error) {
	var row migrations.ServiceDependency
	err := repository.db.WithContext(ctx).First(&row, "id = ? AND environment_id = ?", dependencyID, environmentID).Error
	return row, err
}

func (repository *Repository) Create(ctx context.Context, row *migrations.ServiceDependency) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.ServiceDependency) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

// ServiceEdges returns the service-to-service adjacency of one environment:
// source service id -> target service ids.
func (repository *Repository) ServiceEdges(ctx context.Context, environmentID string) (map[string][]string, error) {
	var rows []migrations.ServiceDependency
	err := repository.db.WithContext(ctx).
		Where("environment_id = ? AND target_service_id IS NOT NULL", environmentID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	edges := make(map[string][]string, len(rows))
	for _, row := range rows {
		edges[row.SourceServiceID] = append(edges[row.SourceServiceID], *row.TargetServiceID)
	}
	return edges, nil
}
