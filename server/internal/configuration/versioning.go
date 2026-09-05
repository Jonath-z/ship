package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

var ErrVersionNotFound = errors.New("configuration version was not found")

// VersionRecord is one immutable snapshot (SH-053). Old versions are readable
// indefinitely and never mutated — the migrations model enforces immutability
// at the persistence layer.
type VersionRecord struct {
	Version       int          `json:"version"`
	State         DesiredState `json:"state"`
	ActorUserID   *string      `json:"actorUserId,omitempty"`
	ChangeSummary string       `json:"changeSummary,omitempty"`
	CreatedAt     time.Time    `json:"createdAt"`
}

// Snapshot compiles the current desired state and stores it as the next
// version for the environment, serialized with a per-environment row lock so
// concurrent snapshots cannot allocate the same number.
func (repository *Repository) Snapshot(ctx context.Context, environmentID string, actorUserID *string, changeSummary string) (VersionRecord, error) {
	state, _, err := repository.Compile(ctx, environmentID)
	if err != nil {
		return VersionRecord{}, err
	}
	document, err := CanonicalJSON(state)
	if err != nil {
		return VersionRecord{}, err
	}

	record := VersionRecord{State: state, ActorUserID: actorUserID, ChangeSummary: changeSummary}
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row migrations.Configuration
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&row, "environment_id = ?", environmentID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			id, idErr := identity.New()
			if idErr != nil {
				return idErr
			}
			row = migrations.Configuration{ID: id, EnvironmentID: environmentID}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("create configuration: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("find configuration: %w", err)
		}

		record.Version = row.CurrentVersion + 1
		versionID, err := identity.New()
		if err != nil {
			return err
		}
		version := migrations.ConfigurationVersion{
			ID: versionID, ConfigurationID: row.ID, Version: record.Version,
			Document: string(document), ActorUserID: actorUserID, ChangeSummary: changeSummary,
		}
		if err := tx.Create(&version).Error; err != nil {
			return fmt.Errorf("create configuration version: %w", err)
		}
		record.CreatedAt = version.CreatedAt.UTC()
		return tx.Model(&row).UpdateColumns(map[string]any{
			"current_version": record.Version, "updated_at": time.Now().UTC(),
		}).Error
	})
	if err != nil {
		return VersionRecord{}, err
	}
	return record, nil
}

// Versions lists snapshots newest-first, capped at limit.
func (repository *Repository) Versions(ctx context.Context, environmentID string, limit int) ([]VersionRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []migrations.ConfigurationVersion
	err := repository.db.WithContext(ctx).
		Joins("JOIN configurations ON configurations.id = configuration_versions.configuration_id").
		Where("configurations.environment_id = ?", environmentID).
		Order("configuration_versions.version DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list configuration versions: %w", err)
	}
	records := make([]VersionRecord, 0, len(rows))
	for _, row := range rows {
		record, err := toRecord(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (repository *Repository) Version(ctx context.Context, environmentID string, version int) (VersionRecord, error) {
	var row migrations.ConfigurationVersion
	err := repository.db.WithContext(ctx).
		Joins("JOIN configurations ON configurations.id = configuration_versions.configuration_id").
		Where("configurations.environment_id = ? AND configuration_versions.version = ?", environmentID, version).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return VersionRecord{}, ErrVersionNotFound
	}
	if err != nil {
		return VersionRecord{}, fmt.Errorf("get configuration version: %w", err)
	}
	return toRecord(row)
}

func toRecord(row migrations.ConfigurationVersion) (VersionRecord, error) {
	record := VersionRecord{
		Version: row.Version, ActorUserID: row.ActorUserID,
		ChangeSummary: row.ChangeSummary, CreatedAt: row.CreatedAt.UTC(),
	}
	if err := json.Unmarshal([]byte(row.Document), &record.State); err != nil {
		return VersionRecord{}, fmt.Errorf("decode configuration version %d: %w", row.Version, err)
	}
	return record, nil
}
