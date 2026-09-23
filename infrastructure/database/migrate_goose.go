package database

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationsDir = "migrations"

type gooseZapLogger struct {
	log *zap.Logger
}

func (l *gooseZapLogger) Printf(format string, v ...interface{}) {
	l.log.Sugar().Infof(format, v...)
}

func (l *gooseZapLogger) Fatalf(format string, v ...interface{}) {
	l.log.Sugar().Fatalf(format, v...)
}

func setupGoose(log *zap.Logger) error {
	goose.SetBaseFS(migrationFS)
	goose.SetLogger(&gooseZapLogger{log: log})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}
	return nil
}

func RunMigrations(db *sql.DB, log *zap.Logger) error {
	if err := setupGoose(log); err != nil {
		return err
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run migration: %w", err)
	}
	return nil
}

func ResetMigrations(db *sql.DB, log *zap.Logger) error {
	if err := setupGoose(log); err != nil {
		return err
	}
	if err := goose.DownTo(db, migrationsDir, 0); err != nil {
		return fmt.Errorf("failed to reset migration: %w", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to re-migrate: %w", err)
	}
	return nil
}
