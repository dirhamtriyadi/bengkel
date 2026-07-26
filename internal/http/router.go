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
		v1.POST("/payments/midtrans/notification", h.MidtransNotification)

		secured := v1.Group("")
		secured.Use(middleware.Authenticate(tokens), middleware.ScopeBranch())
		secured.GET("/dashboard", h.Dashboard)
		secured.GET("/customers", h.ListCustomers)
		secured.POST("/customers", middleware.Authorize("manager", "cashier"), h.CreateCustomer)
		secured.PUT("/customers/:id", middleware.Authorize("manager", "cashier"), h.UpdateCustomer)
		secured.DELETE("/customers/:id", middleware.Authorize("manager"), h.DeleteCustomer)
		secured.GET("/vehicles", h.ListVehicles)
		secured.POST("/vehicles", middleware.Authorize("manager", "cashier"), h.CreateVehicle)
		secured.GET("/products", h.ListProducts)
		secured.POST("/products", middleware.Authorize("manager"), h.CreateProduct)
		secured.GET("/inventory", h.Inventory)
		secured.POST("/inventory/adjustments", middleware.Authorize("manager"), h.AdjustStock)
		secured.GET("/work-orders", h.ListWorkOrders)
		secured.POST("/work-orders", middleware.Authorize("manager", "cashier"), h.CreateWorkOrder)
		secured.POST("/work-orders/:id/items", middleware.Authorize("manager", "mechanic", "cashier"), h.AddWorkOrderItem)
		secured.GET("/sales", h.ListSales)
		secured.GET("/sales/:id", h.SaleDetail)
		secured.POST("/sales/checkout", middleware.Authorize("manager", "cashier"), h.Checkout)
		secured.POST("/payments/:id/midtrans/snap", middleware.Authorize("manager", "cashier"), h.CreateMidtransSnap)
		secured.GET("/reports/profit-loss", middleware.Authorize("manager"), h.ProfitLoss)
		secured.GET("/reports/general-ledger", middleware.Authorize("manager"), h.GeneralLedger)
		secured.GET("/audit-logs", middleware.Authorize("manager"), h.AuditLogs)
		secured.GET("/settings", middleware.Authorize("manager"), h.Settings)
		secured.PUT("/settings/:key", middleware.Authorize("manager"), h.UpsertSetting)
		secured.GET("/cms/pages", middleware.Authorize("manager"), h.CMSPages)
		secured.PUT("/cms/pages/:slug", middleware.Authorize("manager"), h.UpsertCMSPage)
	}
	return r
}
