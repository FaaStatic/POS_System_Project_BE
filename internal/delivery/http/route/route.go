package route

import (
	"shop_project_be/internal/constant/apipath"
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

// New memasang semua rute di /api/{version}/{group}/{service}. Rute tanpa JWT hanya
// login, refresh, dan webhook Midtrans; group account dan sales memuat keduanya,
// jadi JWT dipasang per rute atau per service, bukan per group.
func New(h Handlers, jwtMw *middleware.JWTMiddleware, storage fiber.Storage, log *zap.Logger, midtransConfigured bool, version string) func(router fiber.Router) {
	return func(router fiber.Router) {
		jwt := jwtMw.Auth(log)
		userLimit := limiter.New(middleware.GetUserLimiter(storage))
		onlySuper := jwtMw.RequireRole(enum.Superadmin.String())
		requireMidtrans := middleware.RequireFeature(midtransConfigured, "online payment")
		// secure: JWT dan limiter per user, lalu middleware tambahan service itu.
		secure := func(extra ...any) []any {
			return append([]any{jwt, userLimit}, extra...)
		}

		account := router.Group(apipath.Group(version, apipath.GroupAccount))
		auth := account.Group("/auth")
		auth.Post("/login", limiter.New(middleware.GetLoginLimiter(storage)), h.User.Login)
		auth.Post("/refresh", limiter.New(middleware.GetRefreshLimiter(storage)), h.User.Refresh)
		auth.Post("/logout", jwt, userLimit, h.User.Logout)
		auth.Post("/register", jwt, userLimit, onlySuper, h.User.Register)

		users := account.Group("/users", secure(onlySuper)...)
		users.Get("", h.User.ListStaff)
		users.Patch("/:id", h.User.UpdateStaff)
		users.Delete("/:id", h.User.DeactivateStaff)
		users.Post("/:id/restore", h.User.RestoreStaff)
		users.Delete("/:id/permanent", h.User.PurgeStaff)

		fcm := account.Group("/fcm", secure()...)
		fcm.Post("/register", h.Fcm.Register)
		fcm.Post("/logout", h.Fcm.Logout)
		fcm.Get("/devices", onlySuper, h.Fcm.ListDevices)
		fcm.Delete("/devices/:id", onlySuper, h.Fcm.RevokeDevice)

		catalog := router.Group(apipath.Group(version, apipath.GroupCatalog), secure()...)

		products := catalog.Group("/products")
		products.Post("", onlySuper, h.Product.Add)
		products.Post("/bulk", onlySuper, h.Product.AddBulk)
		// /template didaftarkan sebelum /:id supaya tidak tertangkap sebagai id.
		products.Get("/template", onlySuper, h.Product.DownloadTemplate)
		products.Get("", h.Product.List)
		products.Get("/export/barcodes", h.Product.ExportBarcodes)
		products.Get("/:id", h.Product.Get)
		// /stock didaftarkan sebelum /:id supaya tidak tertangkap sebagai id.
		products.Patch("/stock", onlySuper, h.Product.UpdateStock)
		products.Patch("/:id", h.Product.Update)
		products.Delete("/:id", onlySuper, h.Product.Delete)

		categories := catalog.Group("/categories")
		categories.Get("", h.Product.ListCategories)
		categories.Post("", onlySuper, h.Product.AddCategory)
		categories.Patch("/:id", onlySuper, h.Product.RenameCategory)
		categories.Delete("/:id", onlySuper, h.Product.DeleteCategory)

		sales := router.Group(apipath.Group(version, apipath.GroupSales))
		// Webhook didaftarkan sebelum group payments ber-JWT di bawahnya; Midtrans tidak
		// membawa token.
		sales.Post("/payments/notification", requireMidtrans, limiter.New(middleware.GetWebhookLimiter(storage)), h.Payment.Notification)

		transactions := sales.Group("/transactions", secure()...)
		transactions.Post("", h.Transaction.Add)
		transactions.Post("/sync", h.Transaction.Sync)
		transactions.Get("", h.Transaction.List)
		transactions.Get("/report/month", onlySuper, h.Transaction.ReportMonth)
		transactions.Get("/report/transaction", h.Transaction.ReportTransaction)
		transactions.Get("/:id", h.Transaction.Get)
		transactions.Delete("/:id", h.Transaction.Delete)
		transactions.Patch("/:id/conflict-review", onlySuper, h.Transaction.ReviewConflict)

		payments := sales.Group("/payments", secure(requireMidtrans)...)
		payments.Post("/qris", h.Payment.ChargeQris)
		payments.Post("/va", h.Payment.ChargeVA)
		payments.Get("", onlySuper, h.Payment.List)
		payments.Get("/:order_id/status", h.Payment.Status)

		customers := sales.Group("/customers", secure()...)
		customers.Post("", h.Customer.Add)
		customers.Get("", h.Customer.List)
		customers.Get("/:id", h.Customer.Get)
		customers.Get("/:id/transactions", h.Transaction.ListByCustomer)
		customers.Put("/:id", onlySuper, h.Customer.Update)
		// DELETE /:id menonaktifkan (soft delete); /permanent menghapus yang sudah nonaktif.
		customers.Delete("/:id", onlySuper, h.Customer.Delete)
		customers.Post("/:id/restore", onlySuper, h.Customer.Restore)
		customers.Delete("/:id/permanent", onlySuper, h.Customer.Purge)

		debts := sales.Group("/debts", secure()...)
		debts.Post("", onlySuper, h.Debt.Add)
		debts.Post("/pay", h.Debt.Pay)
		debts.Get("", h.Debt.List)
		debts.Get("/report", onlySuper, h.Debt.Report)
		debts.Get("/:id", h.Debt.Get)

		insight := router.Group(apipath.Group(version, apipath.GroupInsight), secure()...)
		insight.Get("/dashboard/summary", onlySuper, h.Dashboard.Summary)
		insight.Get("/reports/:file", handler.DownloadReport)
	}
}
