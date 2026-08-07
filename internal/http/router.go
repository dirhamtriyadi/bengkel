package http

import (
	"net/http"

	"bengkel/internal/auth"
	"bengkel/internal/config"
	"bengkel/internal/http/handler"
	"bengkel/internal/http/middleware"
	"bengkel/internal/http/response"

	_ "bengkel/docs"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, db *gorm.DB, log *zap.Logger) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recover(log), middleware.Logger(log), middleware.CORS(cfg.CORSOrigins))
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Warn("trusted proxies rejected", zap.Error(err))
	}
	tokens := auth.NewManager(cfg.AccessSecret, cfg.RefreshSecret, cfg.AccessTTL, cfg.RefreshTTL)
	h := handler.New(db, cfg, tokens)
	r.GET("/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			response.Error(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Database tidak tersedia")
			return
		}
		response.OK(c, gin.H{"status": "ok", "service": cfg.AppName})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler, ginSwagger.PersistAuthorization(true)))

	v1 := r.Group("/api/v1")
	{
		authGroup := v1.Group("/auth")
		authGroup.POST("/login", h.Login)
		authGroup.POST("/refresh", h.Refresh)
		authGroup.GET("/me", middleware.Authenticate(tokens), h.Me)
		authGroup.POST("/logout", middleware.Authenticate(tokens), h.Logout)
		public := v1.Group("/public")
		public.GET("/pages/:slug", h.PublicPage)
		public.GET("/settings", h.PublicSettings)
		public.GET("/invoices/:token", h.PublicInvoice)
		public.POST("/invoices/:token/midtrans/snap", h.PublicInvoiceMidtransSnap)
		public.POST("/invoices/:token/midtrans/sync", h.PublicInvoiceMidtransSync)
		v1.POST("/payments/midtrans/notification", h.MidtransNotification)

		secured := v1.Group("")
		secured.Use(middleware.Authenticate(tokens), middleware.ScopeBranch())
		secured.GET("/dashboard", middleware.RequirePermission("dashboard.read"), h.Dashboard)

		secured.GET("/branches", middleware.RequirePermission("branch.manage"), h.ListBranches)
		secured.POST("/branches", middleware.RequirePermission("branch.manage"), h.CreateBranch)
		secured.PUT("/branches/:id", middleware.RequirePermission("branch.manage"), h.UpdateBranch)
		secured.GET("/users", middleware.RequirePermission("user.manage"), h.ListUsers)
		secured.POST("/users", middleware.RequirePermission("user.manage"), h.CreateUser)
		secured.PUT("/users/:id", middleware.RequirePermission("user.manage"), h.UpdateUser)
		secured.DELETE("/users/:id", middleware.RequirePermission("user.manage"), h.DeleteUser)
		secured.GET("/roles", middleware.RequirePermission("role.manage"), h.ListRoles)
		secured.POST("/roles", middleware.RequirePermission("role.manage"), h.CreateRole)
		secured.PUT("/roles/:id", middleware.RequirePermission("role.manage"), h.UpdateRole)
		secured.DELETE("/roles/:id", middleware.RequirePermission("role.manage"), h.DeleteRole)
		secured.GET("/permissions", middleware.RequirePermission("role.manage"), h.ListPermissions)

		secured.GET("/customers", middleware.RequirePermission("customer.read"), h.ListCustomers)
		secured.POST("/customers", middleware.RequirePermission("customer.write"), h.CreateCustomer)
		secured.PUT("/customers/:id", middleware.RequirePermission("customer.write"), h.UpdateCustomer)
		secured.DELETE("/customers/:id", middleware.RequirePermission("customer.write"), h.DeleteCustomer)
		secured.GET("/vehicles", middleware.RequirePermission("vehicle.read"), h.ListVehicles)
		secured.POST("/vehicles", middleware.RequirePermission("vehicle.write"), h.CreateVehicle)
		secured.PUT("/vehicles/:id", middleware.RequirePermission("vehicle.write"), h.UpdateVehicle)
		secured.DELETE("/vehicles/:id", middleware.RequirePermission("vehicle.write"), h.DeleteVehicle)
		secured.GET("/products", middleware.RequirePermission("product.read"), h.ListProducts)
		secured.POST("/products", middleware.RequirePermission("product.write"), h.CreateProduct)
		secured.PUT("/products/:id", middleware.RequirePermission("product.write"), h.UpdateProduct)
		secured.DELETE("/products/:id", middleware.RequirePermission("product.write"), h.DeleteProduct)
		secured.GET("/inventory", middleware.RequirePermission("inventory.read"), h.Inventory)
		secured.GET("/inventory/movements", middleware.RequirePermission("inventory.read"), h.InventoryMovements)
		secured.POST("/inventory/adjustments", middleware.RequirePermission("inventory.adjust"), h.AdjustStock)
		secured.GET("/work-orders", middleware.RequirePermission("work_order.read"), h.ListWorkOrders)
		secured.GET("/work-orders/options/mechanics", middleware.RequirePermission("work_order.read"), h.Mechanics)
		secured.POST("/work-orders/intake", middleware.RequirePermission("work_order.write"), h.IntakeWorkOrder)
		secured.GET("/work-orders/:id", middleware.RequirePermission("work_order.read"), h.WorkOrderDetail)
		secured.POST("/work-orders", middleware.RequirePermission("work_order.write"), h.CreateWorkOrder)
		secured.PATCH("/work-orders/:id", middleware.RequirePermission("work_order.write"), h.UpdateWorkOrder)
		secured.POST("/work-orders/:id/items", middleware.RequirePermission("work_order.write"), h.AddWorkOrderItem)
		secured.DELETE("/work-orders/:id/items/:item_id", middleware.RequirePermission("work_order.write"), h.DeleteWorkOrderItem)
		secured.GET("/sales", middleware.RequirePermission("sale.read"), h.ListSales)
		secured.GET("/sales/:id", middleware.RequirePermission("sale.read"), h.SaleDetail)
		secured.POST("/sales/checkout", middleware.RequirePermission("sale.create"), h.Checkout)
		secured.POST("/sales/:id/public-invoice/whatsapp", middleware.RequirePermission("payment.manage"), h.SendPublicInvoiceWhatsApp)

		secured.GET("/payments", middleware.RequirePermission("payment.read"), h.ListPayments)
		secured.GET("/payments/:id", middleware.RequirePermission("payment.read"), h.PaymentDetail)
		secured.POST("/payments/:id/midtrans/snap", middleware.RequirePermission("payment.manage"), h.CreateMidtransSnap)
		secured.POST("/payments/:id/midtrans/sync", middleware.RequirePermission("payment.manage"), h.SyncMidtransPayment)

		secured.GET("/accounts", middleware.RequirePermission("accounting.read"), h.ListAccounts)
		secured.POST("/accounts", middleware.RequirePermission("accounting.write"), h.CreateAccount)
		secured.PUT("/accounts/:id", middleware.RequirePermission("accounting.write"), h.UpdateAccount)
		secured.DELETE("/accounts/:id", middleware.RequirePermission("accounting.write"), h.DeleteAccount)
		secured.GET("/journals", middleware.RequirePermission("accounting.read"), h.ListJournals)
		secured.GET("/journals/:id", middleware.RequirePermission("accounting.read"), h.JournalDetail)
		secured.POST("/journals", middleware.RequirePermission("accounting.write"), h.CreateJournal)
		secured.PUT("/journals/:id", middleware.RequirePermission("accounting.write"), h.UpdateJournal)
		secured.POST("/journals/:id/post", middleware.RequirePermission("accounting.write"), h.PostJournal)
		secured.POST("/journals/:id/reverse", middleware.RequirePermission("accounting.write"), h.ReverseJournal)
		secured.DELETE("/journals/:id", middleware.RequirePermission("accounting.write"), h.DeleteJournal)

		secured.GET("/reports/profit-loss", middleware.RequirePermission("report.read"), h.ProfitLoss)
		secured.GET("/reports/general-ledger", middleware.RequirePermission("report.read"), h.GeneralLedger)
		secured.GET("/reports/trial-balance", middleware.RequirePermission("report.read"), h.TrialBalance)
		secured.GET("/reports/balance-sheet", middleware.RequirePermission("report.read"), h.BalanceSheet)
		secured.GET("/reports/cash-flow", middleware.RequirePermission("report.read"), h.CashFlow)
		secured.GET("/reports/sales", middleware.RequirePermission("report.read"), h.SalesReport)
		secured.GET("/reports/inventory-valuation", middleware.RequirePermission("report.read"), h.InventoryValuation)
		secured.GET("/audit-logs", middleware.RequirePermission("audit.read"), h.AuditLogs)
		secured.GET("/settings", middleware.RequirePermission("settings.manage"), h.Settings)
		secured.PUT("/settings/:key", middleware.RequirePermission("settings.manage"), h.UpsertSetting)
		secured.GET("/integrations/whatsapp", middleware.RequirePermission("settings.manage"), h.WhatsAppIntegration)
		secured.PUT("/integrations/whatsapp", middleware.RequirePermission("settings.manage"), h.UpdateWhatsAppIntegration)
		secured.POST("/integrations/whatsapp/session/start", middleware.RequirePermission("settings.manage"), h.StartWhatsAppSession)
		secured.GET("/integrations/whatsapp/session/status", middleware.RequirePermission("settings.manage"), h.WhatsAppSessionStatus)
		secured.GET("/integrations/whatsapp/session/qr", middleware.RequirePermission("settings.manage"), h.WhatsAppSessionQR)
		secured.GET("/cms/pages", middleware.RequirePermission("cms.manage"), h.CMSPages)
		secured.PUT("/cms/pages/:slug", middleware.RequirePermission("cms.manage"), h.UpsertCMSPage)
	}
	return r
}
