// Package migrations defines the database schema as Go structs.
package migrations

import "time"

// Models is the schema passed to GORM's migration tools.
var Models = []any{
	&User{},
	&Project{},
	&Environment{},
}

type User struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	Email        string    `gorm:"not null;uniqueIndex"`
	PasswordHash string    `gorm:"not null"`
	Role         string    `gorm:"not null;default:owner"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Project struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"not null"`
	Slug      string    `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type Environment struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	ProjectID string    `gorm:"type:uuid;not null;uniqueIndex:idx_environment_project_slug"`
	Project   Project   `gorm:"constraint:OnDelete:CASCADE"`
	Name      string    `gorm:"not null"`
	Slug      string    `gorm:"not null;uniqueIndex:idx_environment_project_slug"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}
