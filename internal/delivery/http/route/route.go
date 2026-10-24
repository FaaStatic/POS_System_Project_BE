package route

import (
	"shop_project_be/internal/constant/enum"
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
	Dashboard   *handler.DashboardHandler
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

		onlySuper := jwtMw.RequireRole(enum.Superadmin.String())

		api.Post("/auth/register", onlySuper, h.User.Register)

		users := api.Group("/users", onlySuper)
		users.Get("", h.User.ListStaff)
		users.Patch("/:id", h.User.UpdateStaff)
		users.Delete("/:id", h.User.DeactivateStaff)
		users.Post("/:id/restore", h.User.RestoreStaff)
		users.Delete("/:id/permanent", h.User.PurgeStaff)

		api.Get("/dashboard/summary", onlySuper, h.Dashboard.Summary)

		api.Get("/reports/:file", handler.DownloadReport)

		products := api.Group("/products")
		products.Post("", onlySuper, h.Product.Add)
		products.Post("/bulk", onlySuper, h.Product.AddBulk)
		products.Get("", h.Product.List)
		products.Get("/:id", h.Product.Get)
		// /stock didaftarkan sebelum /:id supaya tidak tertangkap sebagai id.
		products.Patch("/stock", onlySuper, h.Product.UpdateStock)
		products.Patch("/:id", h.Product.Update)
		products.Delete("/:id", onlySuper, h.Product.Delete)

		api.Get("/categories", h.Product.ListCategories)
		api.Post("/categories", onlySuper, h.Product.AddCategory)
		api.Patch("/categories/:id", onlySuper, h.Product.RenameCategory)
		api.Delete("/categories/:id", onlySuper, h.Product.DeleteCategory)

		transactions := api.Group("/transactions")
		transactions.Post("", h.Transaction.Add)
		transactions.Post("/sync", h.Transaction.Sync)
		transactions.Get("", h.Transaction.List)
		transactions.Get("/report/month", onlySuper, h.Transaction.ReportMonth)
		transactions.Get("/report/transaction", h.Transaction.ReportTransaction)
		transactions.Get("/:id", h.Transaction.Get)
		transactions.Delete("/:id", h.Transaction.Delete)
		transactions.Patch("/:id/conflict-review", onlySuper, h.Transaction.ReviewConflict)

		payments := api.Group("/payments", requireMidtrans)
		payments.Post("/qris", h.Payment.ChargeQris)
		payments.Post("/va", h.Payment.ChargeVA)
		payments.Get("", onlySuper, h.Payment.List)
		payments.Get("/:order_id/status", h.Payment.Status)

		customers := api.Group("/customers")
		customers.Post("", h.Customer.Add)
		customers.Get("", h.Customer.List)
		customers.Get("/:id", h.Customer.Get)
		customers.Get("/:id/transactions", h.Transaction.ListByCustomer)
		customers.Put("/:id", onlySuper, h.Customer.Update)
		// DELETE /:id menonaktifkan (soft delete); /permanent menghapus yang sudah nonaktif.
		customers.Delete("/:id", onlySuper, h.Customer.Delete)
		customers.Post("/:id/restore", onlySuper, h.Customer.Restore)
		customers.Delete("/:id/permanent", onlySuper, h.Customer.Purge)

		debts := api.Group("/debts")
		debts.Post("", onlySuper, h.Debt.Add)
		debts.Post("/pay", h.Debt.Pay)
		debts.Get("", h.Debt.List)
		debts.Get("/report", onlySuper, h.Debt.Report)
		debts.Get("/:id", h.Debt.Get)

		fcm := api.Group("/fcm")
		fcm.Post("/register", h.Fcm.Register)
		fcm.Post("/logout", h.Fcm.Logout)
		fcm.Get("/devices", onlySuper, h.Fcm.ListDevices)
		fcm.Delete("/devices/:id", onlySuper, h.Fcm.RevokeDevice)
	}
}
