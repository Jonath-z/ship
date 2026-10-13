// Package database owns SQLite connectivity and struct-driven migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Jonath-z/ship/server/migrations"
)

type Connection struct {
	ORM *gorm.DB
	SQL *sql.DB
}

// DSN turns a database file path into a SQLite DSN with the pragmas Ship
// needs: WAL so the api and worker processes can read while the other writes,
// a busy timeout so cross-process write contention queues instead of failing,
// foreign keys on (SQLite leaves them off by default), and immediate
// transactions so read-modify-write transactions take the write lock up
// front instead of deadlocking on lock upgrade.
func DSN(databasePath string) string {
	if strings.HasPrefix(databasePath, "file:") {
		return databasePath // caller supplied a full DSN (tests)
	}
	return "file:" + databasePath + "?" + url.Values{
		"_pragma": []string{
			"busy_timeout(10000)",
			"journal_mode(WAL)",
			"foreign_keys(1)",
			"synchronous(NORMAL)",
		},
		"_txlock": []string{"immediate"},
	}.Encode()
}

func Open(ctx context.Context, databasePath string) (*Connection, error) {
	if !strings.HasPrefix(databasePath, "file:") {
		if dir := filepath.Dir(databasePath); dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}

	db, err := gorm.Open(sqlite.Open(DSN(databasePath)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// SQLite stores time.Time as text, so every timestamp must be written
		// in the same zone for lexicographic comparison (cursors, ordering) to
		// match chronological order.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sqlite connection: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return &Connection{ORM: db, SQL: sqlDB}, nil
}

func (connection *Connection) Ping(ctx context.Context) error {
	return connection.SQL.PingContext(ctx)
}

func (connection *Connection) Close() error {
	return connection.SQL.Close()
}

func MigrateUp(ctx context.Context, databasePath string) error {
	connection, err := Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer connection.Close()

	if err := connection.ORM.WithContext(ctx).AutoMigrate(migrations.Models...); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func MigrateDown(ctx context.Context, databasePath string) error {
	connection, err := Open(ctx, databasePath)
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

func MigrationVersion(ctx context.Context, databasePath string) (int64, error) {
	connection, err := Open(ctx, databasePath)
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
