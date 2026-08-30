// Package database owns PostgreSQL connectivity and struct-driven migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Jonath-z/ship/server/migrations"
)

type Connection struct {
	ORM *gorm.DB
	SQL *sql.DB
}

func Open(ctx context.Context, databaseURL string) (*Connection, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get postgres connection: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Connection{ORM: db, SQL: sqlDB}, nil
}

func (connection *Connection) Ping(ctx context.Context) error {
	return connection.SQL.PingContext(ctx)
}

func (connection *Connection) Close() error {
	return connection.SQL.Close()
}

func MigrateUp(ctx context.Context, databaseURL string) error {
	connection, err := Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer connection.Close()

	if err := connection.ORM.WithContext(ctx).AutoMigrate(migrations.Models...); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func MigrateDown(ctx context.Context, databaseURL string) error {
	connection, err := Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer connection.Close()

	migrator := connection.ORM.WithContext(ctx).Migrator()
	models := make([]any, len(migrations.Models))
	for index := range migrations.Models {
		models[len(migrations.Models)-1-index] = migrations.Models[index]
	}
	if err := migrator.DropTable(models...); err != nil {
		return fmt.Errorf("roll back database: %w", err)
	}
	return nil
}

func MigrationVersion(ctx context.Context, databaseURL string) (int64, error) {
	connection, err := Open(ctx, databaseURL)
	if err != nil {
		return 0, err
	}
	defer connection.Close()

	migrator := connection.ORM.WithContext(ctx).Migrator()
	for _, model := range migrations.Models {
		if !migrator.HasTable(model) {
			return 0, nil
		}
	}
	return 1, nil
}
