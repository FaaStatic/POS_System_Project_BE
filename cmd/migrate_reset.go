package cmd

import (
	"fmt"
	"os"
	envconfig "shop_project_be/config/env_config"
	"shop_project_be/infrastructure/database"
	zaplogger "shop_project_be/infrastructure/logger"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var migrateResetForce bool

var migrateResetCmd = &cobra.Command{
	Use:   "migrate-reset",
	Short: "Delete All table and do re-migrate",
	Run: func(cmd *cobra.Command, args []string) {
		env := os.Getenv("APP_ENV")
		zaplogger.LoggerCustom(env)
		defer zaplogger.Logger.Sync()
		if env == "production" && !migrateResetForce {
			zaplogger.Logger.Fatal("migrate-reset drops every table; refusing on APP_ENV=production without --force")
		}
		envConf, err := envconfig.InitEnvConfig(zaplogger.Logger)
		if err != nil {
			zaplogger.Logger.Fatal("Failed to initialize environment config: %v", zap.Error(err))
		}
		db, err := database.InitDB(envConf.DB, zaplogger.Logger, env)
		if err != nil {
			zaplogger.Logger.Fatal("Failed to initialize database:", zap.Error(err))
		}
		sqldb, err := db.DB()
		if err != nil {
			zaplogger.Logger.Fatal("Failed to get sql.DB:", zap.Error(err))
		}
		defer sqldb.Close()

		if err := database.ResetMigrations(sqldb, zaplogger.Logger); err != nil {
			zaplogger.Logger.Fatal("failed to reset database", zap.Error(err))
		}
		fmt.Println("Database berhasil di-reset dan di-migrasi ulang!")
	},
}

func init() {
	migrateResetCmd.Flags().BoolVar(&migrateResetForce, "force", false, "allow migrate-reset on APP_ENV=production (drops every table)")
	rootCmd.AddCommand(migrateResetCmd)
}
