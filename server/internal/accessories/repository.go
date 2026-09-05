package accessories

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

func (repository *Repository) EnvironmentExists(ctx context.Context, projectID, environmentID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) ServerExists(ctx context.Context, serverID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.Server{}).Where("id = ?", serverID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) ServerGroupExists(ctx context.Context, environmentID, serverGroupID string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&migrations.ServerGroup{}).
		Where("id = ? AND environment_id = ?", serverGroupID, environmentID).Count(&count).Error
	return count == 1, err
}

func (repository *Repository) List(ctx context.Context, environmentID, cursor string, limit int) ([]migrations.Accessory, string, error) {
	query := repository.db.WithContext(ctx).Preload("Server").Preload("ServerGroup").
		Where("environment_id = ?", environmentID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Accessory
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list accessories: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, environmentID, accessoryID string) (migrations.Accessory, error) {
	var row migrations.Accessory
	if err := repository.db.WithContext(ctx).Preload("Server").Preload("ServerGroup").
		First(&row, "id = ? AND environment_id = ?", accessoryID, environmentID).Error; err != nil {
		return migrations.Accessory{}, err
	}
	return row, nil
}

func (repository *Repository) Create(ctx context.Context, row *migrations.Accessory) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) Update(ctx context.Context, row *migrations.Accessory, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Omit(clause.Associations).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Preload("Server").Preload("ServerGroup").First(row, "id = ?", row.ID).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.Accessory) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

// DependentServiceNames lists services that declare a dependency on the given
// accessory, ordered by name.
func (repository *Repository) DependentServiceNames(ctx context.Context, environmentID, accessoryID string) ([]string, error) {
	var names []string
	err := repository.db.WithContext(ctx).Model(&migrations.ServiceDependency{}).
		Distinct("services.name").
		Joins("JOIN services ON services.id = service_dependencies.source_service_id").
		Where("service_dependencies.environment_id = ? AND service_dependencies.target_accessory_id = ?", environmentID, accessoryID).
		Order("services.name ASC").Pluck("services.name", &names).Error
	return names, err
}
