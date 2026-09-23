package database

import (
	"fmt"
	envconfig "shop_project_be/config/env_config"

	"go.uber.org/zap"
)

func MigrateDB(log *zap.Logger) error {
	envConf, err := envconfig.InitEnvConfig(log)
	if err != nil {
		return fmt.Errorf("failed to init config: %w", err)
	}

	db, err := InitDB(envConf.DB, log, envConf.App.Env)
	if err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	sqlDb, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}
	defer sqlDb.Close()

	if err := RunMigrations(sqlDb, log); err != nil {
		return err
	}

	log.Info("migration successful")
	return nil
}
