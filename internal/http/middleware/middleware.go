package middleware

import (
	"net/http"
	"strings"
	"time"

	"bengkel/internal/auth"
	"bengkel/internal/http/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request", zap.String("method", c.Request.Method), zap.String("path", c.FullPath()), zap.Int("status", c.Writer.Status()), zap.Duration("latency", time.Since(start)), zap.String("request_id", c.GetString("request_id")))
	}
}
func Recover(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic recovered", zap.Any("error", recovered), zap.String("request_id", c.GetString("request_id")))
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal")
	})
}
func CORS(origins []string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Branch-ID, X-Request-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
func Authenticate(tokens *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Token akses diperlukan")
			return
		}
		claims, err := tokens.ParseAccess(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "TOKEN_INVALID", "Token akses tidak valid atau kedaluwarsa")
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("branch_id", claims.BranchID)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
		c.Next()
	}
}
func Authorize(allowedRoles ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, role := range allowedRoles {
		allowed[role] = true
	}
	return func(c *gin.Context) {
		roles, _ := c.Get("roles")
		for _, role := range roles.([]string) {
			if role == "owner" || allowed[role] {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses ke aksi ini")
	}
}
func RequirePermission(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, _ := c.Get("roles")
		if values, ok := roles.([]string); ok {
			for _, role := range values {
				if role == "owner" {
					c.Next()
					return
				}
			}
		}
		permissions, _ := c.Get("permissions")
		if values, ok := permissions.([]string); ok {
			for _, permission := range values {
				if permission == required {
					c.Next()
					return
				}
			}
		}
		response.Error(c, http.StatusForbidden, "PERMISSION_DENIED", "Permission "+required+" diperlukan")
	}
}
func ScopeBranch() gin.HandlerFunc {
	return func(c *gin.Context) {
		branch, ok := c.Get("branch_id")
		if !ok || branch == nil {
			response.Error(c, http.StatusForbidden, "BRANCH_REQUIRED", "Cabang aktif diperlukan")
			return
		}
		branchPtr, _ := branch.(*uuid.UUID)
		if branchPtr == nil {
			response.Error(c, http.StatusForbidden, "BRANCH_REQUIRED", "Cabang aktif diperlukan")
			return
		}
		requested := c.GetHeader("X-Branch-ID")
		if requested != "" && requested != branchPtr.String() {
			roles, _ := c.Get("roles")
			isOwner := false
			for _, role := range roles.([]string) {
				isOwner = isOwner || role == "owner"
			}
			if !isOwner {
				response.Error(c, http.StatusForbidden, "BRANCH_SCOPE_VIOLATION", "Data di luar cabang tidak dapat diakses")
				return
			}
			parsed, err := uuid.Parse(requested)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "BRANCH_INVALID", "ID cabang tidak valid")
				return
			}
			branchPtr = &parsed
		}
		c.Set("branch_id", branchPtr)
		c.Next()
	}
}
