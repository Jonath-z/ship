package environments

import (
	"context"
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

func (repository *Repository) FindProject(ctx context.Context, projectID string) (migrations.Project, error) {
	var project migrations.Project
	if err := repository.db.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return migrations.Project{}, err
	}
	return project, nil
}

func (repository *Repository) List(ctx context.Context, projectID, cursor string, limit int) ([]migrations.Environment, string, error) {
	query := repository.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at ASC, id ASC")
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return nil, "", err
		}
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Environment
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list environments: %w", err)
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	return rows, nextCursor, nil
}

func (repository *Repository) Find(ctx context.Context, projectID, environmentID string) (migrations.Environment, error) {
	var row migrations.Environment
	if err := repository.db.WithContext(ctx).
		First(&row, "id = ? AND project_id = ?", environmentID, projectID).Error; err != nil {
		return migrations.Environment{}, err
	}
	return row, nil
}

func (repository *Repository) FindForUpdate(ctx context.Context, projectID, environmentID string) (migrations.Environment, error) {
	var row migrations.Environment
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row, "id = ? AND project_id = ?", environmentID, projectID).Error; err != nil {
		return migrations.Environment{}, err
	}
	return row, nil
}

func (repository *Repository) Create(ctx context.Context, row *migrations.Environment) error {
	return repository.db.WithContext(ctx).Create(row).Error
}

func (repository *Repository) CreateConfiguration(ctx context.Context, environmentID string) error {
	id, err := identity.New()
	if err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Create(&migrations.Configuration{
		ID: id, EnvironmentID: environmentID,
	}).Error
}

func (repository *Repository) Update(ctx context.Context, row *migrations.Environment, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	if err := repository.db.WithContext(ctx).Model(row).Updates(values).Error; err != nil {
		return err
	}
	return repository.db.WithContext(ctx).First(row, "id = ?", row.ID).Error
}

func (repository *Repository) Delete(ctx context.Context, row *migrations.Environment) error {
	return repository.db.WithContext(ctx).Delete(row).Error
}

func (repository *Repository) DeletionImpact(ctx context.Context, row migrations.Environment) (DeletionImpact, error) {
	impact := DeletionImpact{EnvironmentID: row.ID, Slug: row.Slug}
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
			Where("environment_id = ?", row.ID).Count(destination).Error; err != nil {
			return DeletionImpact{}, err
		}
	}
	return impact, nil
}

// CopyDesiredState duplicates configuration records but intentionally omits
// secrets, vault entries, configuration history, deployments, logs, and backups.
func (repository *Repository) CopyDesiredState(ctx context.Context, sourceID, targetID string) error {
	groupIDs, err := repository.copyServerGroups(ctx, sourceID, targetID)
	if err != nil {
		return err
	}
	serviceIDs, err := repository.copyServices(ctx, sourceID, targetID, groupIDs)
	if err != nil {
		return err
	}
	accessoryIDs, err := repository.copyAccessories(ctx, sourceID, targetID, groupIDs)
	if err != nil {
		return err
	}
	if err := repository.copyVolumes(ctx, sourceID, targetID, serviceIDs, accessoryIDs); err != nil {
		return err
	}
	if err := repository.copyDomains(ctx, sourceID, targetID, serviceIDs); err != nil {
		return err
	}
	if err := repository.copyEnvironmentVariables(ctx, sourceID, targetID, serviceIDs); err != nil {
		return err
	}
	return repository.copyDependencies(ctx, sourceID, targetID, serviceIDs, accessoryIDs)
}

func (repository *Repository) copyServerGroups(ctx context.Context, sourceID, targetID string) (map[string]string, error) {
	var rows []migrations.ServerGroup
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	identifiers := make(map[string]string, len(rows))
	for _, row := range rows {
		oldID := row.ID
		newID, err := identity.New()
		if err != nil {
			return nil, err
		}
		row.ID, row.EnvironmentID = newID, targetID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return nil, err
		}
		identifiers[oldID] = newID
	}
	if len(identifiers) == 0 {
		return identifiers, nil
	}
	var memberships []migrations.ServerGroupMembership
	oldGroupIDs := make([]string, 0, len(identifiers))
	for oldID := range identifiers {
		oldGroupIDs = append(oldGroupIDs, oldID)
	}
	if err := repository.db.WithContext(ctx).Where("server_group_id IN ?", oldGroupIDs).Find(&memberships).Error; err != nil {
		return nil, err
	}
	for _, membership := range memberships {
		membership.ServerGroupID = identifiers[membership.ServerGroupID]
		membership.CreatedAt = time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&membership).Error; err != nil {
			return nil, err
		}
	}
	return identifiers, nil
}

func (repository *Repository) copyServices(ctx context.Context, sourceID, targetID string, groupIDs map[string]string) (map[string]string, error) {
	var rows []migrations.Service
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	identifiers := make(map[string]string, len(rows))
	for _, row := range rows {
		oldID := row.ID
		newID, err := identity.New()
		if err != nil {
			return nil, err
		}
		groupID, err := remapOptional(row.ServerGroupID, groupIDs, "service server group")
		if err != nil {
			return nil, err
		}
		row.ID, row.EnvironmentID, row.ServerGroupID = newID, targetID, groupID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return nil, err
		}
		identifiers[oldID] = newID
	}
	return identifiers, nil
}

func (repository *Repository) copyAccessories(ctx context.Context, sourceID, targetID string, groupIDs map[string]string) (map[string]string, error) {
	var rows []migrations.Accessory
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	identifiers := make(map[string]string, len(rows))
	for _, row := range rows {
		oldID := row.ID
		newID, err := identity.New()
		if err != nil {
			return nil, err
		}
		groupID, err := remapOptional(row.ServerGroupID, groupIDs, "accessory server group")
		if err != nil {
			return nil, err
		}
		row.ID, row.EnvironmentID, row.ServerGroupID = newID, targetID, groupID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return nil, err
		}
		identifiers[oldID] = newID
	}
	return identifiers, nil
}

func (repository *Repository) copyVolumes(ctx context.Context, sourceID, targetID string, serviceIDs, accessoryIDs map[string]string) error {
	var rows []migrations.Volume
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		newID, err := identity.New()
		if err != nil {
			return err
		}
		serviceID, err := remapOptional(row.ServiceID, serviceIDs, "volume service")
		if err != nil {
			return err
		}
		accessoryID, err := remapOptional(row.AccessoryID, accessoryIDs, "volume accessory")
		if err != nil {
			return err
		}
		row.ID, row.EnvironmentID, row.ServiceID, row.AccessoryID = newID, targetID, serviceID, accessoryID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) copyDomains(ctx context.Context, sourceID, targetID string, serviceIDs map[string]string) error {
	var rows []migrations.Domain
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		newID, err := identity.New()
		if err != nil {
			return err
		}
		serviceID, ok := serviceIDs[row.ServiceID]
		if !ok {
			return fmt.Errorf("clone domain: service %s is outside the source environment", row.ServiceID)
		}
		row.ID, row.EnvironmentID, row.ServiceID = newID, targetID, serviceID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) copyEnvironmentVariables(ctx context.Context, sourceID, targetID string, serviceIDs map[string]string) error {
	var rows []migrations.EnvironmentVariable
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		newID, err := identity.New()
		if err != nil {
			return err
		}
		serviceID, err := remapOptional(row.ServiceID, serviceIDs, "environment variable service")
		if err != nil {
			return err
		}
		row.ID, row.EnvironmentID, row.ServiceID = newID, targetID, serviceID
		row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) copyDependencies(ctx context.Context, sourceID, targetID string, serviceIDs, accessoryIDs map[string]string) error {
	var rows []migrations.ServiceDependency
	if err := repository.db.WithContext(ctx).Where("environment_id = ?", sourceID).Order("created_at, id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		newID, err := identity.New()
		if err != nil {
			return err
		}
		sourceServiceID, ok := serviceIDs[row.SourceServiceID]
		if !ok {
			return fmt.Errorf("clone dependency: source service %s is outside the source environment", row.SourceServiceID)
		}
		targetServiceID, err := remapOptional(row.TargetServiceID, serviceIDs, "dependency target service")
		if err != nil {
			return err
		}
		targetAccessoryID, err := remapOptional(row.TargetAccessoryID, accessoryIDs, "dependency target accessory")
		if err != nil {
			return err
		}
		row.ID, row.EnvironmentID, row.SourceServiceID = newID, targetID, sourceServiceID
		row.TargetServiceID, row.TargetAccessoryID = targetServiceID, targetAccessoryID
		row.CreatedAt = time.Time{}
		if err := repository.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func remapOptional(value *string, identifiers map[string]string, label string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	mapped, ok := identifiers[*value]
	if !ok {
		return nil, fmt.Errorf("clone %s: %s is outside the source environment", label, *value)
	}
	return &mapped, nil
}
