package cmd

import (
	"context"
	"errors"
	"os"

	envconfig "shop_project_be/config/env_config"
	"shop_project_be/infrastructure/cache"
	"shop_project_be/infrastructure/database"
	loggerconfig "shop_project_be/infrastructure/logger"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var resetAdminTarget string

var resetAdminPassword = &cobra.Command{
	Use:   "reset-admin-password",
	Short: "Reset a superadmin password directly in the database",
	Long: "Asks for the credentials of an active superadmin (the operator), then sets a new password " +
		"for the superadmin named by --username and signs that account out everywhere.",
	Run: func(cmd *cobra.Command, args []string) {
		env := os.Getenv("APP_ENV")
		loggerconfig.LoggerCustom(env)
		defer loggerconfig.Logger.Sync()

		envConf, err := envconfig.InitEnvConfig(loggerconfig.Logger)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init config", zap.Error(err))
		}
		db, err := database.InitDB(envConf.DB, loggerconfig.Logger, env)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init database", zap.Error(err))
		}
		rdb, err := cache.InitRedis(&envConf.Redis)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init redis", zap.Error(err))
		}

		ctx := context.Background()
		userRepo := repository.NewUserRepository(db)
		sessionRepo := repository.NewSessionRepository(rdb)

		if err := runResetAdminPassword(ctx, userRepo, sessionRepo, resetAdminTarget); err != nil {
			loggerconfig.Logger.Fatal("reset failed", zap.Error(err))
		}
		loggerconfig.Logger.Info("password reset", zap.String("username", resetAdminTarget))
	},
}

func runResetAdminPassword(ctx context.Context, users domain.UserRepository, sessions domain.SessionRepository, targetName string) error {
	if err := authenticateOperator(ctx, users); err != nil {
		return err
	}

	target, err := users.GetUserByUsername(ctx, targetName)
	if err != nil {
		return err
	}
	if target == nil || target.Role != enum.Superadmin {
		return errors.New("no active superadmin with that username")
	}

	password, err := promptSecret("New password: ")
	if err != nil {
		return err
	}
	confirm, err := promptSecret("Repeat new password: ")
	if err != nil {
		return err
	}
	if err := validateNewPassword(password, confirm); err != nil {
		return err
	}

	hashed := &domain.Users{Password: password}
	if err := hashed.HashPswd(); err != nil {
		return err
	}
	if err := users.UpdateUser(ctx, target.ID, map[string]any{"password": hashed.Password}); err != nil {
		return err
	}
	// The old password must stop working on devices that are already signed in.
	return sessions.RevokeUserSessions(ctx, target.ID.String())
}

// validateNewPassword mirrors the 6-character minimum of create-admin and the API.
func validateNewPassword(password, confirm string) error {
	if password != confirm {
		return errors.New("passwords do not match")
	}
	if len(password) < 6 {
		return errors.New("password minimal 6 karakter")
	}
	return nil
}

func init() {
	resetAdminPassword.Flags().StringVarP(&resetAdminTarget, "username", "u", "", "superadmin whose password is reset (required)")
	resetAdminPassword.MarkFlagRequired("username")
	rootCmd.AddCommand(resetAdminPassword)
}
