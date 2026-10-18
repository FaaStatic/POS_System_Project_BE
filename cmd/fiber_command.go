package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	envconfig "shop_project_be/config/env_config"
	fiberconfig "shop_project_be/config/fiber_config"
	"shop_project_be/infrastructure/api/payment"
	"shop_project_be/infrastructure/cache"
	"shop_project_be/infrastructure/database"
	"shop_project_be/infrastructure/fcm"
	loggerconfig "shop_project_be/infrastructure/logger"
	"shop_project_be/internal/delivery/http/handler"
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/delivery/http/route"
	"shop_project_be/internal/repository"
	"shop_project_be/internal/usecase"
	"shop_project_be/pkg/jwt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var serverRun = &cobra.Command{
	Use:   "serve",
	Short: "Running Server Based Fiber",
	Run: func(cmd *cobra.Command, args []string) {
		ctxBg := context.Background()

		env := os.Getenv("APP_ENV")
		loggerconfig.LoggerCustom(env)
		defer loggerconfig.Logger.Sync()

		envConf, err := envconfig.InitEnvConfig(loggerconfig.Logger)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init config", zap.Error(err))
		}

		redisClient, err := cache.InitRedis(&envConf.Redis)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init redis", zap.Error(err))
		}
		defer redisClient.Close()
		db, err := database.InitDB(envConf.DB, loggerconfig.Logger, env)
		if err != nil {
			loggerconfig.Logger.Fatal("failed to init database", zap.Error(err))
		}

		productRepo := repository.NewProductRepository(db)
		trxRepo := repository.NewTransactionRepository(db)
		userRepo := repository.NewUserRepository(db)
		customerRepo := repository.NewCustomerRepository(db)
		debtRepo := repository.NewDebtRepository(db)
		paymentRepo := repository.NewPaymentRepository(db)
		sessionRepo := repository.NewSessionRepository(redisClient)
		fcmRepo := repository.NewDeviceTokenRepository(db)

		jwtService := jwt.NewJWTService(envConf.JWT.Secret,
			time.Duration(envConf.JWT.AccessTokenTTL)*time.Second,
			time.Duration(envConf.JWT.RefreshTokenTTL)*time.Second)
		midtransGateway := payment.NewMidtransGateway(envConf.Midtrans.ServerKey, envConf.Midtrans.Environment)
		sender, err := fcm.NewSender(ctxBg, envConf.FirebaseStr.GOOGLE_APPLICATION_CREDENTIALS)

		if err != nil {
			loggerconfig.Logger.Fatal("fail init FCM sender", zap.Error(err))
		}

		productUC := usecase.NewProductUsecase(productRepo, loggerconfig.Logger)
		trxUC := usecase.NewTransactionUsecase(trxRepo, productRepo, userRepo, envConf.App.Name, loggerconfig.Logger)
		customerUC := usecase.NewCustomerUsecase(customerRepo, loggerconfig.Logger)
		debtUC := usecase.NewDebtUsecase(debtRepo, loggerconfig.Logger)
		userUC := usecase.NewUserUsecase(userRepo, sessionRepo, loggerconfig.Logger, jwtService)
		fcmUC := usecase.NewFcmUsecase(sender, fcmRepo, loggerconfig.Logger)
		paymentUC := usecase.NewPaymentUsecase(paymentRepo, midtransGateway, productRepo, trxUC, trxRepo, fcmUC, loggerconfig.Logger)

		rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if !envConf.Midtrans.Configured() {
			loggerconfig.Logger.Warn("midtrans not configured: online payment routes disabled")
		}
		if envConf.Midtrans.Configured() && !fiber.IsChild() {
			go func() {
				ticker := time.NewTicker(2 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-rootCtx.Done():
						return
					case <-ticker.C:
						if err := paymentUC.ReconcileStalePayments(rootCtx); err != nil {
							loggerconfig.Logger.Error("payment reconciliation sweep failed", zap.Error(err))
						}
					}
				}
			}()
		}

		handlers := route.Handlers{
			User:        handler.NewUserHandler(userUC, fcmUC, loggerconfig.Logger),
			Product:     handler.NewProductHandler(productUC, loggerconfig.Logger),
			Transaction: handler.NewTransactionHandler(trxUC, loggerconfig.Logger),
			Customer:    handler.NewCustomerHandler(customerUC, loggerconfig.Logger),
			Debt:        handler.NewDebtHandler(debtUC, loggerconfig.Logger),
			Payment:     handler.NewPaymentHandler(paymentUC, loggerconfig.Logger),
			Fcm:         handler.NewFcmHandler(fcmUC, loggerconfig.Logger),
		}
		jwtMw := middleware.NewJwtMiddleware(jwtService, sessionRepo)
		storage := cache.NewLimiterStorage(&envConf.Redis)
		app := fiberconfig.InitFiber(env, envConf, loggerconfig.Logger, storage, route.New(handlers, jwtMw, storage, loggerconfig.Logger, envConf.Midtrans.Configured()))

		loggerconfig.Logger.Info("server starting",
			zap.String("app", envConf.App.Name),
			zap.String("port", envConf.App.Port),
		)

		if err := app.Listen(":"+envConf.App.Port, fiberconfig.GetFiberConfigListener(envConf.App.Env, envConf.App.Prefork, rootCtx)); err != nil {
			loggerconfig.Logger.Fatal("server error", zap.Error(err))
		}
	},
}

func init() {
	rootCmd.AddCommand(serverRun)
}
