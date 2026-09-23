package route

import (
	"shop_project_be/internal/delivery/http/handler"
	"shop_project_be/internal/delivery/http/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"go.uber.org/zap"
)

type Handlers struct {
	User        *handler.UserHandler
	Product     *handler.ProductHandler
	Transaction *handler.TransactionHandler
	Customer    *handler.CustomerHandler
	Debt        *handler.DebtHandler
	Payment     *handler.PaymentHandler
	Fcm         *handler.FcmHandler
}

func New(h Handlers, jwtMw *middleware.JWTMiddleware, storage fiber.Storage, log *zap.Logger, midtransConfigured bool) func(router fiber.Router) {
	return func(router fiber.Router) {
		auth := router.Group("/auth")
		auth.Post("/login", limiter.New(middleware.GetLoginLimiter(storage)), h.User.Login)
		auth.Post("/refresh", limiter.New(middleware.GetRefreshLimiter(storage)), h.User.Refresh)

		requireMidtrans := middleware.RequireFeature(midtransConfigured, "online payment")

		router.Post("/payments/notification", requireMidtrans, limiter.New(middleware.GetWebhookLimiter(storage)), h.Payment.Notification)

		api := router.Group("/api", jwtMw.Auth(log), limiter.New(middleware.GetUserLimiter(storage)))

		api.Post("/auth/logout", h.User.Logout)

		onlySuper := jwtMw.RequireRole("superadmin")

		api.Post("/auth/register", onlySuper, h.User.Register)

		api.Get("/reports/:file", handler.DownloadReport)

		products := api.Group("/products")
		products.Post("", h.Product.Add)
		products.Post("/bulk", h.Product.AddBulk)
		products.Get("", h.Product.List)
		products.Get("/:id", h.Product.Get)
		products.Put("/:id", onlySuper, h.Product.Update)
		products.Patch("/stock", h.Product.UpdateStock)
		products.Delete("/:id", onlySuper, h.Product.Delete)

		transactions := api.Group("/transactions")
		transactions.Post("", h.Transaction.Add)
		transactions.Get("", h.Transaction.List)
		transactions.Get("/report/month", onlySuper, h.Transaction.ReportMonth)
		transactions.Get("/report/transaction", h.Transaction.ReportTransaction)
		transactions.Get("/:id", h.Transaction.Get)
		transactions.Delete("/:id", onlySuper, h.Transaction.Delete)

		payments := api.Group("/payments", requireMidtrans)
		payments.Post("/qris", h.Payment.ChargeQris)
		payments.Post("/va", h.Payment.ChargeVA)
		payments.Get("/:order_id/status", h.Payment.Status)

		customers := api.Group("/customers")
		customers.Post("", h.Customer.Add)
		customers.Get("", h.Customer.List)
		customers.Get("/:id", h.Customer.Get)
		customers.Put("/:id", h.Customer.Update)
		customers.Delete("/:id", onlySuper, h.Customer.Delete)

		debts := api.Group("/debts")
		debts.Post("", h.Debt.Add)
		debts.Post("/pay", h.Debt.Pay)
		debts.Get("", h.Debt.List)
		debts.Get("/report", onlySuper, h.Debt.Report)
		debts.Get("/:id", h.Debt.Get)
		debts.Delete("/:id", onlySuper, h.Debt.Delete)

		fcm := api.Group("/fcm")
		fcm.Post("/register", h.Fcm.Register)
		fcm.Post("/logout", h.Fcm.Logout)
	}
}
