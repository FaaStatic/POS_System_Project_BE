package database

import (
	"fmt"
	"strings"

	envconfig "shop_project_be/config/env_config"
	zaplogger "shop_project_be/infrastructure/logger"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func InitDB(config envconfig.DBConfig, log *zap.Logger, env string) (*gorm.DB, error) {
	dsn := "host=" + dsnValue(config.Host) + " user=" + dsnValue(config.User) + " password=" + dsnValue(config.Password) +
		" dbname=" + dsnValue(config.DBName) + " port=" + dsnValue(config.Port) + " sslmode=" + dsnValue(config.SSLMode) +
		" TimeZone=" + dsnValue(config.TimeZone) + " statement_timeout=30000 lock_timeout=10000"

	gormLog := zaplogger.NewGormZapLogger(log)
	if env == "production" {
		gormLog.LogLevel = gormlogger.Warn
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormLog,
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		TranslateError:         true,
	})

	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	maxOpen := 10
	if config.MaxOpenConns > 0 {
		maxOpen = config.MaxOpenConns
	}
	maxIdle := 5
	if config.MaxIdleConns > 0 {
		maxIdle = config.MaxIdleConns
	}
	connLifetime := time.Hour
	if config.ConnMaxLifetimeMinutes > 0 {
		connLifetime = time.Duration(config.ConnMaxLifetimeMinutes) * time.Minute
	}
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(connLifetime)
	log.Info("Database PostgreSQL Connected",
		zap.String("host", config.Host),
		zap.Int("max_open_conns", maxOpen),
		zap.Int("max_idle_conns", maxIdle),
	)

	return db, nil
}

func dsnValue(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}
