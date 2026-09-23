package cmd

import (
	"os"
	"shop_project_be/infrastructure/database"
	zaplogger "shop_project_be/infrastructure/logger"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var migrateDb = &cobra.Command{
	Use:   "migrate",
	Short: "Shop Migrate Database",
	Run: func(cmd *cobra.Command, args []string) {
		env := os.Getenv("APP_ENV")

		zaplogger.LoggerCustom(env)
		defer zaplogger.Logger.Sync()
		if err := database.MigrateDB(zaplogger.Logger); err != nil {
			zaplogger.Logger.Fatal("failed to migrate database", zap.Error(err))
		}
	},
}

func init() {
	rootCmd.AddCommand(migrateDb)
}
