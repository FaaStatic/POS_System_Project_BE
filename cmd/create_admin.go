package cmd

import (
	"context"
	"os"
	envconfig "shop_project_be/config/env_config"
	"shop_project_be/infrastructure/database"
	loggerconfig "shop_project_be/infrastructure/logger"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	createAdminUsername string
	createAdminPassword string
)

var createAdmin = &cobra.Command{
	Use:   "create-admin",
	Short: "Create a superadmin account directly in the database",
	Run: func(cmd *cobra.Command, args []string) {
		env := os.Getenv("APP_ENV")
		loggerconfig.LoggerCustom(env)
		defer loggerconfig.Logger.Sync()

		if len(createAdminPassword) < 6 {
			loggerconfig.Logger.Fatal("password minimal 6 karakter")
		}

		envConf, err := envconfig.InitEnvConfig(loggerconfig.Logger)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init config", zap.Error(err))
		}

		db, err := database.InitDB(envConf.DB, loggerconfig.Logger, env)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init database", zap.Error(err))
		}

		userRepo := repository.NewUserRepository(db)
		ctx := context.Background()

		existing, err := userRepo.GetUserByUsername(ctx, createAdminUsername)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to check username", zap.Error(err))
		}
		if existing != nil {
			loggerconfig.Logger.Fatal("username already taken", zap.String("username", createAdminUsername))
		}

		user := &domain.Users{
			Username: createAdminUsername,
			Password: createAdminPassword,
			Role:     enum.Superadmin,
		}
		if err := user.HashPswd(); err != nil {
			loggerconfig.Logger.Fatal("failed to hash password", zap.Error(err))
		}
		if err := userRepo.RegisterUser(ctx, user); err != nil {
			loggerconfig.Logger.Fatal("failed to create user", zap.Error(err))
		}

		loggerconfig.Logger.Info("account created successfully",
			zap.String("username", createAdminUsername),
			zap.String("role", enum.Superadmin.String()),
		)
	},
}

func init() {
	createAdmin.Flags().StringVarP(&createAdminUsername, "username", "u", "", "account username (required)")
	createAdmin.Flags().StringVarP(&createAdminPassword, "password", "p", "", "account password, minimum 6 characters (required)")
	createAdmin.MarkFlagRequired("username")
	createAdmin.MarkFlagRequired("password")
	rootCmd.AddCommand(createAdmin)
}
