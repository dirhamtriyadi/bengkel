package handler

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bengkel/internal/auth"
	"bengkel/internal/config"
	"bengkel/internal/http/response"
	"bengkel/internal/http/validation"
	"bengkel/internal/model"
	"bengkel/internal/paymentgateway"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Handler struct {
	DB     *gorm.DB
	Config config.Config
	Tokens *auth.Manager
}

func New(db *gorm.DB, cfg config.Config, tokens *auth.Manager) *Handler {
	return &Handler{DB: db, Config: cfg, Tokens: tokens}
}

type loginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}
type refreshInput struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
type authUser struct {
	ID          uuid.UUID  `json:"id"`
	BranchID    *uuid.UUID `json:"branch_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
}
type tokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         authUser  `json:"user"`
}

// Login godoc
// @Summary Login pengguna
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body loginInput true "Kredensial"
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var input loginInput
	if !bind(c, &input) {
		return
	}
	var user model.User
	if err := h.DB.Where("lower(email)=lower(?) AND is_active=true", input.Email).First(&user).Error; err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau kata sandi salah")
		return
	}
	roles, permissions := h.identity(user.ID)
	access, refresh, expiry, err := h.Tokens.Issue(user.ID, user.BranchID, roles, permissions)
	if err != nil {
		serverError(c)
		return
	}
	record := model.RefreshToken{UserID: user.ID, TokenHash: auth.Hash(refresh), ExpiresAt: time.Now().Add(h.Tokens.RefreshExpiry()), UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP()}
	now := time.Now()
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Model(&user).Update("last_login_at", now).Error
	}); err != nil {
		serverError(c)
		return
	}
	h.audit(c, "login", "auth", &user.ID, nil, map[string]any{"email": user.Email})
	response.OK(c, tokenResponse{access, refresh, "Bearer", expiry, authUser{user.ID, user.BranchID, user.Name, user.Email, roles, permissions}})
}

// Refresh godoc
// @Summary Rotasi access dan refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body refreshInput true "Refresh token"
// @Success 200 {object} response.Envelope
// @Router /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var input refreshInput
	if !bind(c, &input) {
		return
	}
	claims, err := h.Tokens.ParseRefresh(input.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "REFRESH_TOKEN_INVALID", "Refresh token tidak valid atau kedaluwarsa")
		return
	}
	var old model.RefreshToken
	if err := h.DB.Where("token_hash=? AND revoked_at IS NULL AND expires_at>now()", auth.Hash(input.RefreshToken)).First(&old).Error; err != nil {
		response.Error(c, http.StatusUnauthorized, "REFRESH_TOKEN_REVOKED", "Refresh token tidak aktif")
		return
	}
	var user model.User
	if err := h.DB.Where("id=? AND is_active=true", claims.UserID).First(&user).Error; err != nil {
		response.Error(c, http.StatusUnauthorized, "USER_INACTIVE", "Pengguna tidak aktif")
		return
	}
	roles, permissions := h.identity(user.ID)
	access, refresh, expiry, err := h.Tokens.Issue(user.ID, user.BranchID, roles, permissions)
	if err != nil {
		serverError(c)
		return
	}
	now := time.Now()
	newToken := model.RefreshToken{UserID: user.ID, TokenHash: auth.Hash(refresh), ExpiresAt: now.Add(h.Tokens.RefreshExpiry()), UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP()}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&old).Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Create(&newToken).Error
	}); err != nil {
		serverError(c)
		return
	}
	response.OK(c, tokenResponse{access, refresh, "Bearer", expiry, authUser{user.ID, user.BranchID, user.Name, user.Email, roles, permissions}})
}

// Me godoc
// @Summary Profil pengguna aktif
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	id := userID(c)
	var user model.User
	if err := h.DB.First(&user, "id=?", id).Error; err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	roles, permissions := h.identity(id)
	response.OK(c, authUser{user.ID, user.BranchID, user.Name, user.Email, roles, permissions})
}

// Logout godoc
// @Summary Cabut refresh token
// @Tags Auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body refreshInput true "Refresh token"
// @Success 200 {object} response.Envelope
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var input refreshInput
	if !bind(c, &input) {
		return
	}
	now := time.Now()
	h.DB.Model(&model.RefreshToken{}).Where("token_hash=? AND user_id=?", auth.Hash(input.RefreshToken), userID(c)).Update("revoked_at", now)
	response.OK(c, gin.H{"message": "Berhasil keluar"})
}
func (h *Handler) identity(userID uuid.UUID) ([]string, []string) {
	var roles, permissions []string
	h.DB.Table("roles r").Select("r.code").Joins("JOIN user_roles ur ON ur.role_id=r.id").Where("ur.user_id=?", userID).Scan(&roles)
	h.DB.Table("permissions p").Distinct("p.code").Joins("JOIN role_permissions rp ON rp.permission_id=p.id").Joins("JOIN user_roles ur ON ur.role_id=rp.role_id").Where("ur.user_id=?", userID).Scan(&permissions)
	return roles, permissions
}

type customerInput struct {
	Code    string `json:"code" validate:"required,max=50"`
	Name    string `json:"name" validate:"required,max=150"`
	Phone   string `json:"phone" validate:"max=30"`
	Email   string `json:"email" validate:"omitempty,email"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

// ListCustomers godoc
// @Summary Daftar pelanggan
// @Tags Customers
// @Security BearerAuth
// @Produce json
// @Param page query int false "Halaman"
// @Param per_page query int false "Jumlah per halaman"
// @Param search query string false "Pencarian"
// @Success 200 {object} response.Envelope
// @Router /customers [get]
func (h *Handler) ListCustomers(c *gin.Context) {
	var rows []model.Customer
	list(c, h.DB.Model(&model.Customer{}).Where("branch_id=?", branchID(c)), &rows, []string{"name", "code", "phone"})
}

// CreateCustomer godoc
// @Summary Tambah pelanggan
// @Tags Customers
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body customerInput true "Pelanggan"
// @Success 201 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /customers [post]
func (h *Handler) CreateCustomer(c *gin.Context) {
	var in customerInput
	if !bind(c, &in) {
		return
	}
	row := model.Customer{BranchID: branchID(c), Code: in.Code, Name: in.Name, Phone: in.Phone, Email: in.Email, Address: in.Address, Notes: in.Notes}
	err := h.DB.Create(&row).Error
	conflict(c, err, "CUSTOMER_EXISTS", "Kode pelanggan sudah digunakan")
	if err == nil {
		h.audit(c, "create", "customer", &row.ID, nil, row)
		response.Created(c, row)
	}
}

// UpdateCustomer godoc
// @Summary Ubah pelanggan
// @Tags Customers
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Customer ID"
// @Param payload body customerInput true "Pelanggan"
// @Success 200 {object} response.Envelope
// @Router /customers/{id} [put]
func (h *Handler) UpdateCustomer(c *gin.Context) {
	var in customerInput
	if !bind(c, &in) {
		return
	}
	var row model.Customer
	if !h.findScoped(c, &row) {
		return
	}
	before := row
	row.Code = in.Code
	row.Name = in.Name
	row.Phone = in.Phone
	row.Email = in.Email
	row.Address = in.Address
	row.Notes = in.Notes
	err := h.DB.Save(&row).Error
	conflict(c, err, "CUSTOMER_EXISTS", "Kode pelanggan sudah digunakan")
	if err == nil {
		h.audit(c, "update", "customer", &row.ID, before, row)
		response.OK(c, row)
	}
}

// DeleteCustomer godoc
// @Summary Hapus pelanggan
// @Tags Customers
// @Security BearerAuth
// @Param id path string true "Customer ID"
// @Success 200 {object} response.Envelope
// @Router /customers/{id} [delete]
func (h *Handler) DeleteCustomer(c *gin.Context) {
	var row model.Customer
	if !h.findScoped(c, &row) {
		return
	}
	var references int64
	h.DB.Model(&model.Vehicle{}).Where("customer_id=?", row.ID).Count(&references)
	if references == 0 {
		h.DB.Model(&model.WorkOrder{}).Where("customer_id=?", row.ID).Count(&references)
	}
	if references == 0 {
		h.DB.Model(&model.Sale{}).Where("customer_id=?", row.ID).Count(&references)
	}
	if references > 0 {
		response.Error(c, http.StatusConflict, "CUSTOMER_IN_USE", "Pelanggan memiliki kendaraan atau histori transaksi dan tidak dapat dihapus")
		return
	}
	if err := h.DB.Delete(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "customer", &row.ID, row, nil)
	response.OK(c, gin.H{"message": "Pelanggan dihapus"})
}

type vehicleInput struct {
	CustomerID  *uuid.UUID `json:"customer_id"`
	Identifier  string     `json:"identifier" validate:"required,max=100"`
	PlateNumber string     `json:"plate_number" validate:"max=30"`
	Brand       string     `json:"brand" validate:"max=80"`
	Model       string     `json:"model" validate:"max=100"`
	Year        int        `json:"year" validate:"min=0"`
	Color       string     `json:"color"`
	Odometer    int64      `json:"odometer" validate:"min=0"`
	Notes       string     `json:"notes"`
}

// ListVehicles godoc
// @Summary Daftar kendaraan
// @Tags Vehicles
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /vehicles [get]
func (h *Handler) ListVehicles(c *gin.Context) {
	var rows []model.Vehicle
	list(c, h.DB.Model(&model.Vehicle{}).Where("branch_id=?", branchID(c)), &rows, []string{"identifier", "plate_number", "brand", "model"})
}

// CreateVehicle godoc
// @Summary Tambah kendaraan
// @Tags Vehicles
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body vehicleInput true "Kendaraan"
// @Success 201 {object} response.Envelope
// @Router /vehicles [post]
func (h *Handler) CreateVehicle(c *gin.Context) {
	var in vehicleInput
	if !bind(c, &in) {
		return
	}
	row := model.Vehicle{BranchID: branchID(c), CustomerID: in.CustomerID, Identifier: in.Identifier, PlateNumber: in.PlateNumber, Brand: in.Brand, Model: in.Model, Year: in.Year, Color: in.Color, Odometer: in.Odometer, Notes: in.Notes}
	err := h.DB.Create(&row).Error
	conflict(c, err, "VEHICLE_EXISTS", "Identitas kendaraan sudah digunakan")
	if err == nil {
		h.audit(c, "create", "vehicle", &row.ID, nil, row)
		response.Created(c, row)
	}
}

type productInput struct {
	SKU          string  `json:"sku" validate:"required,max=80"`
	Name         string  `json:"name" validate:"required,max=180"`
	Type         string  `json:"type" validate:"required,oneof=part service other"`
	Unit         string  `json:"unit" validate:"required,max=30"`
	CostPrice    int64   `json:"cost_price" validate:"min=0"`
	SellingPrice int64   `json:"selling_price" validate:"min=0"`
	MinStock     float64 `json:"min_stock" validate:"min=0"`
	IsActive     *bool   `json:"is_active"`
}

// ListProducts godoc
// @Summary Daftar barang dan jasa
// @Tags Products
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /products [get]
func (h *Handler) ListProducts(c *gin.Context) {
	var rows []model.Product
	query := h.DB.Model(&model.Product{}).Where("branch_id IS NULL OR branch_id=?", branchID(c))
	list(c, query, &rows, []string{"sku", "name", "type"})
}

// CreateProduct godoc
// @Summary Tambah barang atau jasa
// @Tags Products
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body productInput true "Produk"
// @Success 201 {object} response.Envelope
// @Router /products [post]
func (h *Handler) CreateProduct(c *gin.Context) {
	var in productInput
	if !bind(c, &in) {
		return
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	bid := branchID(c)
	row := model.Product{BranchID: &bid, SKU: in.SKU, Name: in.Name, Type: in.Type, Unit: in.Unit, CostPrice: in.CostPrice, SellingPrice: in.SellingPrice, MinStock: in.MinStock, IsActive: active}
	err := h.DB.Create(&row).Error
	conflict(c, err, "PRODUCT_EXISTS", "SKU sudah digunakan")
	if err == nil {
		h.audit(c, "create", "product", &row.ID, nil, row)
		response.Created(c, row)
	}
}

type stockInput struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Direction string    `json:"direction" validate:"required,oneof=in out adjustment"`
	Quantity  float64   `json:"quantity" validate:"required,gt=0"`
	UnitCost  int64     `json:"unit_cost" validate:"min=0"`
	Notes     string    `json:"notes" validate:"max=500"`
}

// AdjustStock godoc
// @Summary Penyesuaian stok dengan row lock
// @Tags Inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body stockInput true "Pergerakan stok"
// @Success 201 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /inventory/adjustments [post]
func (h *Handler) AdjustStock(c *gin.Context) {
	var in stockInput
	if !bind(c, &in) {
		return
	}
	var movement model.InventoryMovement
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var balance model.InventoryBalance
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("branch_id=? AND product_id=?", branchID(c), in.ProductID).First(&balance).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			balance = model.InventoryBalance{BranchID: branchID(c), ProductID: in.ProductID}
			if err := tx.Create(&balance).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		delta := in.Quantity
		if in.Direction == "out" {
			delta = -in.Quantity
		}
		if in.Direction == "adjustment" {
			delta = in.Quantity - balance.Quantity
		}
		if balance.Quantity+delta < 0 {
			return errInsufficientStock
		}
		if err := tx.Model(&balance).Updates(map[string]any{"quantity": gorm.Expr("quantity + ?", delta), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		movement = model.InventoryMovement{BranchID: branchID(c), ProductID: in.ProductID, ReferenceType: "manual", Direction: in.Direction, Quantity: in.Quantity, UnitCost: in.UnitCost, Notes: in.Notes, CreatedBy: userID(c)}
		return tx.Create(&movement).Error
	})
	if errors.Is(err, errInsufficientStock) {
		response.Error(c, http.StatusConflict, "INSUFFICIENT_STOCK", "Stok tidak mencukupi")
		return
	}
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "adjust", "inventory", &movement.ID, nil, movement)
	response.Created(c, movement)
}

// Inventory godoc
// @Summary Saldo persediaan cabang
// @Tags Inventory
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /inventory [get]
func (h *Handler) Inventory(c *gin.Context) {
	type row struct {
		model.InventoryBalance
		SKU      string  `json:"sku"`
		Name     string  `json:"name"`
		Unit     string  `json:"unit"`
		MinStock float64 `json:"min_stock"`
	}
	var rows []row
	query := h.DB.Table("inventory_balances ib").Select("ib.*, p.sku, p.name, p.unit, p.min_stock").Joins("JOIN products p ON p.id=ib.product_id").Where("ib.branch_id=?", branchID(c))
	list(c, query, &rows, []string{"p.sku", "p.name"})
}

type workOrderInput struct {
	CustomerID uuid.UUID  `json:"customer_id" validate:"required"`
	VehicleID  uuid.UUID  `json:"vehicle_id" validate:"required"`
	MechanicID *uuid.UUID `json:"mechanic_id"`
	Complaint  string     `json:"complaint" validate:"required"`
	Odometer   int64      `json:"odometer" validate:"min=0"`
}

// ListWorkOrders godoc
// @Summary Daftar work order
// @Tags Work Orders
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /work-orders [get]
func (h *Handler) ListWorkOrders(c *gin.Context) {
	type row struct {
		model.WorkOrder
		CustomerName      string `json:"customer_name"`
		VehicleIdentifier string `json:"vehicle_identifier"`
		PlateNumber       string `json:"plate_number"`
		MechanicName      string `json:"mechanic_name"`
	}
	var rows []row
	query := h.DB.Table("work_orders wo").Select("wo.*,c.name customer_name,v.identifier vehicle_identifier,v.plate_number,COALESCE(m.name,'') mechanic_name").
		Joins("JOIN customers c ON c.id=wo.customer_id").Joins("JOIN vehicles v ON v.id=wo.vehicle_id").
		Joins("LEFT JOIN users m ON m.id=wo.mechanic_id").
		Where("wo.branch_id=? AND wo.deleted_at IS NULL", branchID(c))
	list(c, query, &rows, []string{"wo.number", "wo.complaint", "wo.status", "c.name", "v.identifier", "v.plate_number", "m.name"})
}

// CreateWorkOrder godoc
// @Summary Terima kendaraan dan buat work order
// @Tags Work Orders
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body workOrderInput true "Work order"
// @Success 201 {object} response.Envelope
// @Router /work-orders [post]
func (h *Handler) CreateWorkOrder(c *gin.Context) {
	var in workOrderInput
	if !bind(c, &in) {
		return
	}
	row := model.WorkOrder{BranchID: branchID(c), Number: number("WO"), CustomerID: in.CustomerID, VehicleID: in.VehicleID, MechanicID: in.MechanicID, Status: "inspection", Complaint: in.Complaint, Odometer: in.Odometer}
	if err := h.DB.Create(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "create", "work_order", &row.ID, nil, row)
	response.Created(c, row)
}

type workItemInput struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  float64   `json:"quantity" validate:"required,gt=0"`
	Discount  int64     `json:"discount" validate:"min=0"`
}

// AddWorkOrderItem godoc
// @Summary Catat barang atau jasa pada work order
// @Tags Work Orders
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Work Order ID"
// @Param payload body workItemInput true "Item"
// @Success 201 {object} response.Envelope
// @Router /work-orders/{id}/items [post]
func (h *Handler) AddWorkOrderItem(c *gin.Context) {
	var in workItemInput
	if !bind(c, &in) {
		return
	}
	var wo model.WorkOrder
	if !h.findScoped(c, &wo) {
		return
	}
	if wo.Status == "completed" || wo.Status == "invoiced" || wo.Status == "cancelled" {
		response.Error(c, http.StatusConflict, "WORK_ORDER_LOCKED", "Work order sudah dikunci")
		return
	}
	var product model.Product
	if err := h.DB.Where("id=? AND is_active=true AND (branch_id IS NULL OR branch_id=?)", in.ProductID, branchID(c)).First(&product).Error; err != nil {
		response.Error(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan")
		return
	}
	subtotal := int64(math.Round(float64(product.SellingPrice)*in.Quantity)) - in.Discount
	if subtotal < 0 {
		response.Error(c, http.StatusUnprocessableEntity, "DISCOUNT_INVALID", "Diskon melebihi nilai item")
		return
	}
	item := model.WorkOrderItem{WorkOrderID: wo.ID, ProductID: product.ID, Description: product.Name, Type: product.Type, Quantity: in.Quantity, UnitPrice: product.SellingPrice, UnitCost: product.CostPrice, Discount: in.Discount, Subtotal: subtotal}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if product.Type == "part" {
			var balance model.InventoryBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("branch_id=? AND product_id=?", branchID(c), product.ID).First(&balance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errInsufficientStock
				}
				return err
			}
			if balance.Quantity < in.Quantity {
				return errInsufficientStock
			}
			if err := tx.Model(&balance).Updates(map[string]any{"quantity": gorm.Expr("quantity - ?", in.Quantity), "updated_at": time.Now()}).Error; err != nil {
				return err
			}
			item.StockDeducted = true
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		if product.Type == "part" {
			ref := item.ID
			return tx.Create(&model.InventoryMovement{BranchID: branchID(c), ProductID: product.ID, ReferenceType: "work_order", ReferenceID: &ref, Direction: "out", Quantity: in.Quantity, UnitCost: product.CostPrice, Notes: "Pemakaian " + wo.Number, CreatedBy: userID(c)}).Error
		}
		return nil
	})
	if errors.Is(err, errInsufficientStock) {
		response.Error(c, http.StatusConflict, "INSUFFICIENT_STOCK", "Stok barang tidak mencukupi")
		return
	}
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "add_item", "work_order", &wo.ID, nil, item)
	response.Created(c, item)
}

type checkoutInput struct {
	WorkOrderID   *uuid.UUID      `json:"work_order_id"`
	CustomerID    *uuid.UUID      `json:"customer_id"`
	Items         []workItemInput `json:"items" validate:"dive"`
	Discount      int64           `json:"discount" validate:"min=0"`
	Tax           int64           `json:"tax" validate:"min=0"`
	PaymentMethod string          `json:"payment_method" validate:"required,oneof=cash midtrans"`
	AmountPaid    int64           `json:"amount_paid" validate:"min=0"`
	FeeBearer     string          `json:"fee_bearer" validate:"omitempty,oneof=merchant customer split"`
	Notes         string          `json:"notes"`
}
type checkoutResult struct {
	Sale     model.Sale    `json:"sale"`
	Payment  model.Payment `json:"payment"`
	PrintURL string        `json:"print_url"`
}

var (
	errInsufficientStock = errors.New("insufficient stock")
	errWorkOrderNotReady = errors.New("work order not ready")
	errWorkOrderInvoiced = errors.New("work order already invoiced")
	errWorkOrderEmpty    = errors.New("work order has no items")
)

// Checkout godoc
// @Summary Checkout penjualan retail atau work order
// @Description Mengunci stok, membuat sale/payment, dan mem-posting jurnal untuk pembayaran tunai.
// @Tags Sales
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body checkoutInput true "Checkout"
// @Success 201 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /sales/checkout [post]
func (h *Handler) Checkout(c *gin.Context) {
	var in checkoutInput
	if !bind(c, &in) {
		return
	}
	if in.WorkOrderID == nil && len(in.Items) == 0 {
		response.Error(c, http.StatusUnprocessableEntity, "ITEMS_REQUIRED", "Pilih minimal satu barang atau jasa")
		return
	}
	feeBearer := "merchant"
	var feeSettings midtransFeeSettings
	var midtransConfiguration midtransRuntimeConfig
	if in.PaymentMethod == "midtrans" {
		var err error
		midtransConfiguration, err = h.midtransCredentials(branchID(c))
		if err != nil {
			h.respondMidtransConfigurationError(c, err)
			return
		}
		feeSettings, err = h.midtransFeeSettings(branchID(c))
		if err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_FEE_CONFIG_INVALID", err.Error())
			return
		}
		feeBearer = feeSettings.feeBearer()
	}
	var sale model.Sale
	var payment model.Payment
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		customerID := in.CustomerID
		var workOrder *model.WorkOrder
		var workOrderItems []model.WorkOrderItem
		if in.WorkOrderID != nil {
			var row model.WorkOrder
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND branch_id=?", *in.WorkOrderID, branchID(c)).First(&row).Error; err != nil {
				return err
			}
			if row.Status != "completed" {
				return errWorkOrderNotReady
			}
			var existing int64
			if err := tx.Model(&model.Sale{}).Where("work_order_id=? AND status NOT IN ('void','refunded')", row.ID).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return errWorkOrderInvoiced
			}
			if err := tx.Where("work_order_id=?", row.ID).Order("created_at").Find(&workOrderItems).Error; err != nil {
				return err
			}
			if len(workOrderItems) == 0 {
				return errWorkOrderEmpty
			}
			customerID = &row.CustomerID
			workOrder = &row
		}
		sale = model.Sale{BranchID: branchID(c), Number: number("INV"), CustomerID: customerID, WorkOrderID: in.WorkOrderID, CashierID: userID(c), Status: "pending", Discount: in.Discount, Tax: in.Tax, Notes: in.Notes}
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}
		var subtotal, cogs int64
		if workOrder != nil {
			for _, used := range workOrderItems {
				item := model.SaleItem{SaleID: sale.ID, ProductID: used.ProductID, Description: used.Description, Type: used.Type, Quantity: used.Quantity, UnitPrice: used.UnitPrice, UnitCost: used.UnitCost, Discount: used.Discount, Subtotal: used.Subtotal}
				if err := tx.Create(&item).Error; err != nil {
					return err
				}
				subtotal += used.Subtotal
				cogs += int64(math.Round(float64(used.UnitCost) * used.Quantity))
				if used.Type == "part" && !used.StockDeducted {
					var product model.Product
					if err := tx.First(&product, "id=?", used.ProductID).Error; err != nil {
						return err
					}
					if err := h.consumeStock(tx, sale.ID, product, used.Quantity, c); err != nil {
						return err
					}
					if err := tx.Model(&used).Update("stock_deducted", true).Error; err != nil {
						return err
					}
				}
			}
		} else {
			for _, requested := range in.Items {
				var product model.Product
				if err := tx.Where("id=? AND is_active=true AND (branch_id IS NULL OR branch_id=?)", requested.ProductID, branchID(c)).First(&product).Error; err != nil {
					return err
				}
				line := int64(math.Round(float64(product.SellingPrice)*requested.Quantity)) - requested.Discount
				if line < 0 {
					return fmt.Errorf("invalid discount")
				}
				item := model.SaleItem{SaleID: sale.ID, ProductID: product.ID, Description: product.Name, Type: product.Type, Quantity: requested.Quantity, UnitPrice: product.SellingPrice, UnitCost: product.CostPrice, Discount: requested.Discount, Subtotal: line}
				if err := tx.Create(&item).Error; err != nil {
					return err
				}
				subtotal += line
				cogs += int64(math.Round(float64(product.CostPrice) * requested.Quantity))
				if product.Type == "part" {
					if err := h.consumeStock(tx, sale.ID, product, requested.Quantity, c); err != nil {
						return err
					}
				}
			}
		}
		grand := subtotal - in.Discount + in.Tax
		if grand < 0 {
			return fmt.Errorf("invalid total")
		}
		if in.PaymentMethod == "midtrans" {
			if grand < 1 {
				return fmt.Errorf("total Midtrans minimal Rp1")
			}
		}
		status := "pending"
		var paidAt *time.Time
		if in.PaymentMethod == "cash" {
			if in.AmountPaid < grand {
				return fmt.Errorf("payment too small")
			}
			status = "paid"
			now := time.Now()
			paidAt = &now
		}
		sale.Subtotal = subtotal
		sale.GrandTotal = grand
		sale.GatewayFee = 0
		sale.AmountPaid = in.AmountPaid
		sale.Status = status
		sale.PaidAt = paidAt
		if status == "paid" {
			sale.ChangeAmount = in.AmountPaid - grand
		}
		if err := tx.Save(&sale).Error; err != nil {
			return err
		}
		payment = model.Payment{
			Base:       model.Base{ID: uuid.New()},
			BranchID:   branchID(c),
			SaleID:     sale.ID,
			Method:     in.PaymentMethod,
			Provider:   map[bool]string{true: "midtrans", false: "cash"}[in.PaymentMethod == "midtrans"],
			Status:     status,
			BaseAmount: grand,
			Amount:     grand,
			FeeBearer:  feeBearer,
			PaidAt:     paidAt,
			Metadata:   map[string]any{},
		}
		if in.PaymentMethod == "midtrans" {
			payment.ProviderReference = sale.Number
			snapshot, snapshotError := h.encryptMidtransPaymentSnapshot(midtransConfiguration, payment.BranchID, payment.ID)
			if snapshotError != nil {
				return snapshotError
			}
			payment.Metadata[midtransCredentialsSnapshotMetadataKey] = snapshot
		}
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
		if workOrder != nil {
			if err := tx.Model(workOrder).Update("status", "invoiced").Error; err != nil {
				return err
			}
		}
		if status == "paid" {
			return h.postSaleJournal(tx, sale, cogs, payment)
		}
		return nil
	})
	if errors.Is(err, errInsufficientStock) {
		response.Error(c, http.StatusConflict, "INSUFFICIENT_STOCK", "Stok salah satu barang tidak mencukupi")
		return
	}
	if errors.Is(err, errWorkOrderNotReady) {
		response.Error(c, http.StatusConflict, "WORK_ORDER_NOT_READY", "Work order harus berstatus completed sebelum checkout")
		return
	}
	if errors.Is(err, errWorkOrderInvoiced) {
		response.Error(c, http.StatusConflict, "WORK_ORDER_INVOICED", "Work order sudah memiliki transaksi aktif")
		return
	}
	if errors.Is(err, errWorkOrderEmpty) {
		response.Error(c, http.StatusConflict, "WORK_ORDER_EMPTY", "Work order belum memiliki barang atau jasa")
		return
	}
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "CHECKOUT_FAILED", err.Error())
		return
	}
	h.audit(c, "checkout", "sale", &sale.ID, nil, sale)
	response.Created(c, checkoutResult{sale, redactedPayment(payment), "/print/receipt/" + sale.ID.String()})
}

func (h *Handler) consumeStock(tx *gorm.DB, saleID uuid.UUID, p model.Product, qty float64, c *gin.Context) error {
	var balance model.InventoryBalance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("branch_id=? AND product_id=?", branchID(c), p.ID).First(&balance).Error; err != nil || balance.Quantity < qty {
		return errInsufficientStock
	}
	if err := tx.Model(&balance).Updates(map[string]any{"quantity": gorm.Expr("quantity - ?", qty), "updated_at": time.Now()}).Error; err != nil {
		return err
	}
	ref := saleID
	return tx.Create(&model.InventoryMovement{BranchID: branchID(c), ProductID: p.ID, ReferenceType: "sale", ReferenceID: &ref, Direction: "out", Quantity: qty, UnitCost: p.CostPrice, CreatedBy: userID(c)}).Error
}
func (h *Handler) postSaleJournal(tx *gorm.DB, sale model.Sale, cogs int64, payment model.Payment) error {
	codes := []string{"1101", "4101", "5101", "1201", "5202", "4201"}
	accounts := map[string]uuid.UUID{}
	var rows []model.Account
	if err := tx.Where("code IN ? AND (branch_id IS NULL OR branch_id=?)", codes, sale.BranchID).Find(&rows).Error; err != nil {
		return err
	}
	for _, account := range rows {
		accounts[account.Code] = account.ID
	}
	required := []string{"1101", "4101", "5101", "1201", "5202"}
	if payment.CustomerFee > 0 {
		required = append(required, "4201")
	}
	for _, code := range required {
		if accounts[code] == uuid.Nil {
			return errors.New("chart of accounts is incomplete")
		}
	}
	if payment.BaseAmount == 0 {
		payment.BaseAmount = sale.GrandTotal - payment.CustomerFee
	}
	if payment.Amount == 0 {
		payment.Amount = payment.BaseAmount + payment.CustomerFee
	}
	netSettlement := payment.Amount - payment.ProviderFee
	if netSettlement < 0 {
		return errors.New("Midtrans settlement amount is invalid")
	}
	if payment.BaseAmount+payment.CustomerFee != payment.Amount {
		return errors.New("payment fee reconciliation is not balanced")
	}
	entry := model.JournalEntry{BranchID: sale.BranchID, Number: number("JE"), EntryDate: time.Now(), Description: "Penjualan " + sale.Number, ReferenceType: "sale", ReferenceID: &sale.ID, Status: "draft", CreatedBy: sale.CashierID}
	if err := tx.Create(&entry).Error; err != nil {
		return err
	}
	lines := make([]model.JournalLine, 0, 6)
	if netSettlement > 0 {
		lines = append(lines, model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["1101"], Description: "Settlement kas/bank", Debit: netSettlement})
	}
	lines = append(lines, model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["4101"], Description: "Pendapatan penjualan", Credit: payment.BaseAmount})
	if cogs > 0 {
		lines = append(lines,
			model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["5101"], Description: "Harga pokok penjualan", Debit: cogs},
			model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["1201"], Description: "Persediaan", Credit: cogs},
		)
	}
	if payment.ProviderFee > 0 {
		lines = append(lines, model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["5202"], Description: "Beban payment gateway (" + payment.FeeBearer + ")", Debit: payment.ProviderFee})
	}
	if payment.CustomerFee > 0 {
		lines = append(lines, model.JournalLine{JournalEntryID: entry.ID, AccountID: accounts["4201"], Description: "Pemulihan biaya payment gateway", Credit: payment.CustomerFee})
	}
	if err := tx.Create(&lines).Error; err != nil {
		return err
	}
	return tx.Model(&entry).Update("status", "posted").Error
}

// ListSales godoc
// @Summary Daftar transaksi
// @Tags Sales
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /sales [get]
func (h *Handler) ListSales(c *gin.Context) {
	type row struct {
		model.Sale
		CustomerName  string `json:"customer_name"`
		CustomerPhone string `json:"customer_phone"`
		PaymentMethod string `json:"payment_method"`
	}
	var rows []row
	query := h.DB.Model(&model.Sale{}).Select(`sales.*,
		COALESCE((SELECT c.name FROM customers c WHERE c.id=sales.customer_id AND c.deleted_at IS NULL), '') customer_name,
		COALESCE((SELECT c.phone FROM customers c WHERE c.id=sales.customer_id AND c.deleted_at IS NULL), '') customer_phone,
		COALESCE((SELECT p.method FROM payments p WHERE p.sale_id=sales.id AND p.deleted_at IS NULL ORDER BY p.created_at DESC LIMIT 1), '') payment_method`).
		Where("sales.branch_id=?", branchID(c))
	list(c, query, &rows, []string{"number", "status", "notes"})
}

// SaleDetail godoc
// @Summary Detail sale untuk invoice/receipt
// @Tags Sales
// @Security BearerAuth
// @Produce json
// @Param id path string true "Sale ID"
// @Success 200 {object} response.Envelope
// @Router /sales/{id} [get]
func (h *Handler) SaleDetail(c *gin.Context) {
	var sale model.Sale
	if !h.findScoped(c, &sale) {
		return
	}
	var items []model.SaleItem
	var payments []model.Payment
	h.DB.Where("sale_id=?", sale.ID).Find(&items)
	h.DB.Where("sale_id=?", sale.ID).Find(&payments)
	for index := range payments {
		payments[index] = redactedPayment(payments[index])
	}
	var branch model.Branch
	h.DB.First(&branch, "id=?", sale.BranchID)
	response.OK(c, gin.H{"sale": sale, "items": items, "payments": payments, "branch": branch})
}

// CreateMidtransSnap godoc
// @Summary Buat atau ambil ulang token pembayaran Midtrans Snap
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Payment ID"
// @Success 200 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /payments/{id}/midtrans/snap [post]
func (h *Handler) CreateMidtransSnap(c *gin.Context) {
	var payment model.Payment
	if !h.findScoped(c, &payment) {
		return
	}
	if payment.Method != "midtrans" || payment.Status != "pending" {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_NOT_PAYABLE", "Pembayaran ini tidak dapat diproses dengan Midtrans")
		return
	}
	var sale model.Sale
	if err := h.DB.Where("id=? AND branch_id=?", payment.SaleID, branchID(c)).First(&sale).Error; err != nil {
		serverError(c)
		return
	}
	var customer model.Customer
	if sale.CustomerID != nil {
		_ = h.DB.Where("id=? AND branch_id=?", *sale.CustomerID, branchID(c)).First(&customer).Error
	}
	hasSnapToken := false
	if token, ok := payment.Metadata["snap_token"].(string); ok && token != "" {
		hasSnapToken = true
	}
	finishURL := strings.TrimRight(h.Config.FrontendURL, "/") + "/print/receipt/" + sale.ID.String()
	publicURL := ""
	var issuedLink *model.PublicInvoiceLink
	if !hasSnapToken {
		creator := userID(c)
		link, _, issuedURL, err := h.issuePublicInvoiceLink(sale.BranchID, sale.ID, &creator, "midtrans_snap")
		if err != nil {
			serverError(c)
			return
		}
		issuedLink = &link
		finishURL = issuedURL
		publicURL = issuedURL
	}
	result, operationError := h.getOrCreateMidtransSnap(c.Request.Context(), &payment, sale, customer, finishURL)
	if operationError != nil {
		// A provider rejection means the newly issued bearer link was never used
		// as a valid Midtrans finish URL. Keep it only for an internal persistence
		// failure, because Midtrans may already have created the transaction.
		if issuedLink != nil && operationError.Code != "INTERNAL_ERROR" {
			h.revokeFailedInvoiceLink(*issuedLink, errors.New(operationError.Code))
		}
		operationError.respond(c)
		return
	}
	result.PublicURL = publicURL
	h.audit(c, "create_snap", "payment", &payment.ID, nil, map[string]any{"order_id": payment.ProviderReference})
	response.OK(c, result)
}

// Dashboard godoc
// @Summary Statistik dashboard cabang
// @Tags Dashboard
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /dashboard [get]
func (h *Handler) Dashboard(c *gin.Context) {
	bid := branchID(c)
	var revenue, openWO, lowStock int64
	h.DB.Model(&model.Sale{}).Where("branch_id=? AND status='paid' AND paid_at>=date_trunc('month',now())", bid).Select("COALESCE(sum(grand_total-gateway_fee),0)").Scan(&revenue)
	h.DB.Model(&model.WorkOrder{}).Where("branch_id=? AND status NOT IN ('completed','invoiced','cancelled')", bid).Count(&openWO)
	h.DB.Table("inventory_balances i").Joins("JOIN products p ON p.id=i.product_id").Where("i.branch_id=? AND i.quantity<=p.min_stock", bid).Count(&lowStock)
	response.OK(c, gin.H{"monthly_revenue": revenue, "open_work_orders": openWO, "low_stock_products": lowStock})
}

// AuditLogs godoc
// @Summary Daftar audit log immutable
// @Tags Audit
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /audit-logs [get]
func (h *Handler) AuditLogs(c *gin.Context) {
	var rows []model.AuditLog
	list(c, h.DB.Model(&model.AuditLog{}).Where("branch_id=?", branchID(c)), &rows, []string{"action", "resource", "request_id"})
}

// ProfitLoss godoc
// @Summary Laporan laba rugi
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Param from query string false "Tanggal mulai YYYY-MM-DD"
// @Param to query string false "Tanggal akhir YYYY-MM-DD"
// @Success 200 {object} response.Envelope
// @Router /reports/profit-loss [get]
func (h *Handler) ProfitLoss(c *gin.Context) {
	now := time.Now()
	firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	from := c.DefaultQuery("from", firstDay.Format("2006-01-02"))
	to := c.DefaultQuery("to", now.Format("2006-01-02"))
	type row struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Debit   int64  `json:"debit"`
		Credit  int64  `json:"credit"`
		Balance int64  `json:"balance"`
	}
	rows := make([]row, 0)
	err := h.DB.Table("journal_lines jl").Select("a.code,a.name,a.type,SUM(jl.debit) debit,SUM(jl.credit) credit,CASE WHEN a.type='revenue' THEN SUM(jl.credit-jl.debit) ELSE SUM(jl.debit-jl.credit) END balance").Joins("JOIN journal_entries je ON je.id=jl.journal_entry_id").Joins("JOIN accounts a ON a.id=jl.account_id").Where("je.branch_id=? AND je.status='posted' AND je.entry_date BETWEEN ? AND ? AND a.type IN ('revenue','expense')", branchID(c), from, to).Group("a.code,a.name,a.type").Order("a.code").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	var revenue, expense int64
	for _, r := range rows {
		if r.Type == "revenue" {
			revenue += r.Balance
		} else {
			expense += r.Balance
		}
	}
	response.OK(c, gin.H{"from": from, "to": to, "accounts": rows, "total_revenue": revenue, "total_expense": expense, "net_profit": revenue - expense})
}

// GeneralLedger godoc
// @Summary Buku besar
// @Tags Reports
// @Security BearerAuth
// @Produce json
// @Param account_id query string false "Filter account ID"
// @Param from query string false "Tanggal mulai YYYY-MM-DD"
// @Param to query string false "Tanggal akhir YYYY-MM-DD"
// @Success 200 {object} response.Envelope
// @Router /reports/general-ledger [get]
func (h *Handler) GeneralLedger(c *gin.Context) {
	account := c.Query("account_id")
	from, to := reportPeriod(c)
	query := h.DB.Table("journal_lines jl").Select("jl.*,je.number journal_number,je.entry_date,je.description journal_description,a.code account_code,a.name account_name").Joins("JOIN journal_entries je ON je.id=jl.journal_entry_id").Joins("JOIN accounts a ON a.id=jl.account_id").Where("je.branch_id=? AND je.status='posted' AND je.entry_date BETWEEN ? AND ?", branchID(c), from, to)
	if account != "" {
		query = query.Where("jl.account_id=?", account)
	}
	rows := make([]map[string]any, 0)
	list(c, query, &rows, []string{"je.number", "je.description", "a.code", "a.name"})
}

// PublicPage godoc
// @Summary Halaman CMS terpublikasi
// @Tags Public
// @Produce json
// @Param slug path string true "Slug"
// @Success 200 {object} response.Envelope
// @Router /public/pages/{slug} [get]
func (h *Handler) PublicPage(c *gin.Context) {
	var page model.CMSPage
	if err := h.DB.Where("slug=? AND status='published'", c.Param("slug")).First(&page).Error; err != nil {
		response.Error(c, http.StatusNotFound, "PAGE_NOT_FOUND", "Halaman tidak ditemukan")
		return
	}
	response.OK(c, page)
}

// PublicSettings godoc
// @Summary Pengaturan publik
// @Tags Public
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /public/settings [get]
func (h *Handler) PublicSettings(c *gin.Context) {
	var rows []model.Setting
	h.DB.Where("is_public=true").Find(&rows)
	response.OK(c, rows)
}

// Settings godoc
// @Summary Daftar pengaturan cabang
// @Tags Settings
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /settings [get]
func (h *Handler) Settings(c *gin.Context) {
	var rows []model.Setting
	h.DB.Where("branch_id=?", branchID(c)).Order("key").Find(&rows)
	for index := range rows {
		h.redactWhatsAppSetting(&rows[index])
		h.redactMidtransSetting(&rows[index])
	}
	response.OK(c, rows)
}
func (h *Handler) UpsertSetting(c *gin.Context) {
	var in struct {
		Value    json.RawMessage `json:"value" validate:"required"`
		IsPublic bool            `json:"is_public"`
	}
	if !bind(c, &in) {
		return
	}
	key := c.Param("key")
	if key == whatsAppSettingKey || key == midtransSettingKey {
		response.Error(c, http.StatusConflict, "SETTING_REQUIRES_DEDICATED_ENDPOINT", "Gunakan endpoint integrasi khusus agar kredensial disimpan terenkripsi")
		return
	}
	if key == midtransChannelsSettingKey {
		settings, err := parseMidtransFeeSettings(in.Value)
		if err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_FEE_CONFIG_INVALID", err.Error())
			return
		}
		normalized, err := json.Marshal(settings)
		if err != nil {
			serverError(c)
			return
		}
		in.Value = normalized
	}
	var row model.Setting
	err := h.DB.Where("branch_id=? AND key=?", branchID(c), key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		bid := branchID(c)
		row = model.Setting{BranchID: &bid, Key: key, Value: in.Value, IsPublic: in.IsPublic}
		err = h.DB.Create(&row).Error
	} else if err == nil {
		row.Value = in.Value
		row.IsPublic = in.IsPublic
		err = h.DB.Save(&row).Error
	}
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "update", "setting", &row.ID, nil, row)
	response.OK(c, row)
}

// CMSPages godoc
// @Summary Daftar halaman CMS
// @Tags CMS
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /cms/pages [get]
func (h *Handler) CMSPages(c *gin.Context) {
	var rows []model.CMSPage
	list(c, h.DB.Model(&model.CMSPage{}), &rows, []string{"slug", "title", "status"})
}
func (h *Handler) UpsertCMSPage(c *gin.Context) {
	var in struct {
		Title           string         `json:"title" validate:"required"`
		MetaTitle       string         `json:"meta_title" validate:"max=200"`
		MetaDescription string         `json:"meta_description" validate:"max=320"`
		Content         map[string]any `json:"content" validate:"required"`
		Status          string         `json:"status" validate:"required,oneof=draft published archived"`
	}
	if !bind(c, &in) {
		return
	}
	slug := c.Param("slug")
	var row model.CMSPage
	err := h.DB.Where("slug=?", slug).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.CMSPage{Slug: slug}
		err = nil
	}
	row.Title = in.Title
	row.MetaTitle = in.MetaTitle
	row.MetaDescription = in.MetaDescription
	row.Content = in.Content
	row.Status = in.Status
	if in.Status == "published" && row.PublishedAt == nil {
		now := time.Now()
		row.PublishedAt = &now
	}
	if err == nil {
		err = h.DB.Save(&row).Error
	}
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "update", "cms_page", &row.ID, nil, row)
	response.OK(c, row)
}

type midtransNotification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	TransactionID     string `json:"transaction_id"`
}

var (
	errMidtransStatusInvalid  = errors.New("invalid Midtrans status")
	errMidtransAmountMismatch = errors.New("Midtrans amount mismatch")
	errMidtransNotStarted     = errors.New("Midtrans transaction not started")
)

// SyncMidtransPayment godoc
// @Summary Sinkronkan status pembayaran langsung ke Midtrans
// @Description Berguna setelah callback Snap dan saat notification URL tidak dapat menjangkau localhost.
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Payment ID"
// @Success 200 {object} response.Envelope
// @Router /payments/{id}/midtrans/sync [post]
func (h *Handler) SyncMidtransPayment(c *gin.Context) {
	var payment model.Payment
	if !h.findScoped(c, &payment) {
		return
	}
	if payment.Method != "midtrans" {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_NOT_MIDTRANS", "Pembayaran ini bukan transaksi Midtrans")
		return
	}

	transaction, err := h.checkMidtransTransaction(c.Request.Context(), payment)
	if err != nil {
		if errors.Is(err, errMidtransNotStarted) {
			response.OK(c, gin.H{"status": "pending", "already_processed": true, "not_started": true})
			return
		}
		h.respondMidtransStatusError(c, err)
		return
	}
	status, alreadyProcessed, err := h.applyMidtransTransaction(payment, transaction)
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, gin.H{"status": status, "already_processed": alreadyProcessed})
}

func (h *Handler) MidtransNotification(c *gin.Context) {
	var in midtransNotification
	if !bind(c, &in) {
		return
	}
	var payment model.Payment
	if err := h.DB.Where("provider='midtrans' AND provider_reference=?", in.OrderID).First(&payment).Error; err != nil {
		response.Error(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Pembayaran tidak ditemukan")
		return
	}
	configuration, err := h.midtransCredentialsForPayment(payment)
	if err != nil {
		h.respondMidtransConfigurationError(c, err)
		return
	}
	sum := sha512.Sum512([]byte(in.OrderID + in.StatusCode + in.GrossAmount + configuration.ServerKey))
	if !strings.EqualFold(hex.EncodeToString(sum[:]), in.SignatureKey) {
		response.Error(c, http.StatusUnauthorized, "INVALID_SIGNATURE", "Signature Midtrans tidak valid")
		return
	}

	transaction, err := h.checkMidtransTransactionWithConfig(c.Request.Context(), payment, configuration)
	if err != nil {
		h.respondMidtransStatusError(c, err)
		return
	}
	status, alreadyProcessed, err := h.applyMidtransTransaction(payment, transaction)
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, gin.H{"message": "Notification processed", "status": status, "already_processed": alreadyProcessed})
}

func (h *Handler) checkMidtransTransaction(ctx context.Context, payment model.Payment) (*paymentgateway.TransactionStatus, error) {
	configuration, err := h.midtransCredentialsForPayment(payment)
	if err != nil {
		return nil, err
	}
	return h.checkMidtransTransactionWithConfig(ctx, payment, configuration)
}

func (h *Handler) checkMidtransTransactionWithConfig(ctx context.Context, payment model.Payment, configuration midtransRuntimeConfig) (*paymentgateway.TransactionStatus, error) {
	gateway := paymentgateway.NewMidtrans(configuration.ServerKey, configuration.production())
	transaction, err := gateway.CheckTransaction(ctx, payment.ProviderReference)
	if err != nil {
		var providerError *paymentgateway.Error
		if errors.As(err, &providerError) && providerError.StatusCode == http.StatusNotFound {
			return nil, errMidtransNotStarted
		}
		return nil, fmt.Errorf("check Midtrans transaction: %w", err)
	}
	if transaction == nil || transaction.OrderID != payment.ProviderReference {
		return nil, errMidtransStatusInvalid
	}
	grossAmount, err := parseMidtransAmount(transaction.GrossAmount)
	if err != nil {
		return nil, errMidtransAmountMismatch
	}
	baseAmount := payment.BaseAmount
	if baseAmount == 0 {
		baseAmount = payment.Amount
	}
	if transaction.OriginalAmount != "" {
		originalAmount, parseErr := parseMidtransAmount(transaction.OriginalAmount)
		if parseErr != nil || originalAmount != baseAmount {
			return nil, errMidtransAmountMismatch
		}
	} else if grossAmount < baseAmount {
		return nil, errMidtransAmountMismatch
	}
	customerFee := grossAmount - baseAmount
	if transaction.CustomerImposedPaymentFee != "" {
		imposedFee, parseErr := parseMidtransAmount(transaction.CustomerImposedPaymentFee)
		if parseErr != nil || imposedFee != customerFee {
			return nil, errMidtransAmountMismatch
		}
	}
	return transaction, nil
}

func parseMidtransAmount(value string) (int64, error) {
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil || amount < 0 {
		return 0, errors.New("invalid Midtrans amount")
	}
	return int64(math.Round(amount)), nil
}

func (h *Handler) respondMidtransStatusError(c *gin.Context, err error) {
	if errors.Is(err, errMidtransDisabled) || errors.Is(err, errMidtransNotConfigured) || errors.Is(err, errMidtransSecretInvalid) {
		h.respondMidtransConfigurationError(c, err)
		return
	}
	if errors.Is(err, errMidtransStatusInvalid) {
		response.Error(c, http.StatusBadGateway, "MIDTRANS_STATUS_INVALID", "Respons status transaksi Midtrans tidak valid")
		return
	}
	if errors.Is(err, errMidtransAmountMismatch) {
		response.Error(c, http.StatusConflict, "MIDTRANS_AMOUNT_MISMATCH", "Nominal transaksi Midtrans tidak sesuai dengan tagihan")
		return
	}
	response.Error(c, http.StatusBadGateway, "MIDTRANS_STATUS_UNAVAILABLE", "Status transaksi Midtrans tidak dapat diverifikasi: "+err.Error())
}

func (h *Handler) applyMidtransTransaction(payment model.Payment, transaction *paymentgateway.TransactionStatus) (string, bool, error) {
	status := "pending"
	if (transaction.TransactionStatus == "capture" && transaction.FraudStatus == "accept") || transaction.TransactionStatus == "settlement" {
		status = "paid"
	} else if transaction.TransactionStatus == "deny" || transaction.TransactionStatus == "cancel" || transaction.TransactionStatus == "expire" {
		status = "failed"
	}
	if payment.Status == status && status != "pending" {
		return status, true, nil
	}
	grossAmount, err := parseMidtransAmount(transaction.GrossAmount)
	if err != nil {
		return status, false, err
	}
	baseAmount := payment.BaseAmount
	if baseAmount == 0 {
		baseAmount = payment.Amount
	}
	customerFee := grossAmount - baseAmount
	feeSettings := defaultMidtransFeeSettings()
	if snapshot, ok := payment.Metadata["fee_config_snapshot"]; ok {
		if saved, snapshotErr := decodeMidtransFeeSnapshot(snapshot); snapshotErr == nil {
			feeSettings = saved
		}
	}
	channelKey := midtransChannelKey(transaction)
	channel, found := feeSettings.channel(channelKey)
	if !found {
		channel = midtransChannelSettings{PaymentType: channelKey}
		switch payment.FeeBearer {
		case "customer":
			channel.CustomerPercentage = 100
		case "split":
			channel.CustomerPercentage = 50
		}
	}
	if !feeSettings.AutomaticFee {
		channel.CustomerPercentage = 0
	}
	providerFee := deriveMidtransProviderFee(baseAmount, customerFee, channel)
	feeBearer := "merchant"
	if channel.CustomerPercentage >= 100 {
		feeBearer = "customer"
	} else if channel.CustomerPercentage > 0 {
		feeBearer = "split"
	}
	alreadyProcessed := false
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Webhook, staff sync, and public-page sync can arrive at the same time.
		// Serialize them on the payment row so a sale journal is posted once.
		var lockedPayment model.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedPayment, "id=?", payment.ID).Error; err != nil {
			return err
		}
		if lockedPayment.Status == status && status != "pending" {
			alreadyProcessed = true
			return nil
		}
		payment = lockedPayment
		now := time.Now()
		metadata := payment.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["transaction_id"] = transaction.TransactionID
		metadata["transaction_status"] = transaction.TransactionStatus
		metadata["payment_type"] = transaction.PaymentType
		metadata["payment_channel"] = channelKey
		metadata["bank"] = transaction.Bank
		metadata["store"] = transaction.Store
		metadata["acquirer"] = transaction.Acquirer
		metadata["original_amount"] = baseAmount
		metadata["gross_amount"] = grossAmount
		metadata["customer_imposed_payment_fee"] = customerFee
		metadata["provider_fee_estimated"] = channel.CustomerPercentage < 100
		updates := map[string]any{
			"status":             status,
			"provider_reference": transaction.OrderID,
			"base_amount":        baseAmount,
			"amount":             grossAmount,
			"fee":                providerFee,
			"customer_fee":       customerFee,
			"provider_fee":       providerFee,
			"fee_bearer":         feeBearer,
			"payment_channel":    channelKey,
			"metadata":           metadata,
		}
		if status == "paid" {
			updates["paid_at"] = now
		}
		if err := tx.Model(&payment).Updates(updates).Error; err != nil {
			return err
		}
		if status == "paid" {
			var sale model.Sale
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sale, "id=?", payment.SaleID).Error; err != nil {
				return err
			}
			if sale.Status == "paid" {
				alreadyProcessed = true
				return nil
			}
			sale.Status = "paid"
			sale.PaidAt = &now
			sale.GatewayFee = customerFee
			sale.GrandTotal = grossAmount
			sale.AmountPaid = grossAmount
			if err := tx.Save(&sale).Error; err != nil {
				return err
			}
			var cogs int64
			if err := tx.Model(&model.SaleItem{}).Where("sale_id=?", sale.ID).Select("COALESCE(SUM(unit_cost*quantity),0)").Scan(&cogs).Error; err != nil {
				return err
			}
			payment.BaseAmount = baseAmount
			payment.Amount = grossAmount
			payment.Fee = providerFee
			payment.CustomerFee = customerFee
			payment.ProviderFee = providerFee
			payment.FeeBearer = feeBearer
			payment.PaymentChannel = channelKey
			return h.postSaleJournal(tx, sale, cogs, payment)
		}
		if status == "pending" {
			return tx.Model(&model.Sale{}).Where("id=? AND status='pending'", payment.SaleID).Updates(map[string]any{
				"gateway_fee": customerFee,
				"grand_total": grossAmount,
			}).Error
		}
		var sale model.Sale
		if err := tx.First(&sale, "id=?", payment.SaleID).Error; err != nil {
			return err
		}
		if err := tx.Model(&sale).Where("status='pending'").Update("status", "void").Error; err != nil {
			return err
		}
		if sale.WorkOrderID != nil {
			return tx.Model(&model.WorkOrder{}).Where("id=? AND status='invoiced'", *sale.WorkOrderID).Update("status", "completed").Error
		}
		return nil
	})
	return status, alreadyProcessed, err
}

func midtransChannelKey(transaction *paymentgateway.TransactionStatus) string {
	if transaction == nil {
		return ""
	}
	if transaction.PaymentType == "bank_transfer" {
		switch strings.ToLower(transaction.Bank) {
		case "bca":
			return "bca_va"
		case "bni":
			return "bni_va"
		case "bri":
			return "bri_va"
		case "permata":
			return "permata_va"
		}
	}
	if transaction.PaymentType == "cstore" && transaction.Store != "" {
		return strings.ToLower(transaction.Store)
	}
	return transaction.PaymentType
}

func bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_JSON", "Payload JSON tidak valid")
		return false
	}
	if details := validation.Struct(dst); len(details) > 0 {
		response.Error(c, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Validasi gagal", details...)
		return false
	}
	return true
}
func list(c *gin.Context, query *gorm.DB, dst any, searchColumns []string) {
	page := positive(c.Query("page"), 1)
	per := positive(c.Query("per_page"), 10)
	if per > 100 {
		per = 100
	}
	search := strings.TrimSpace(c.Query("search"))
	if search != "" && len(searchColumns) > 0 {
		parts := make([]string, len(searchColumns))
		args := make([]any, len(searchColumns))
		for i, col := range searchColumns {
			parts[i] = "LOWER(" + col + ") LIKE ?"
			args[i] = "%" + strings.ToLower(search) + "%"
		}
		query = query.Where(strings.Join(parts, " OR "), args...)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		serverError(c)
		return
	}
	sort := c.DefaultQuery("sort", "created_at")
	direction := strings.ToLower(c.DefaultQuery("direction", "desc"))
	if direction != "asc" {
		direction = "desc"
	}
	allowed := map[string]bool{"created_at": true, "updated_at": true, "name": true, "code": true, "number": true, "status": true}
	if !allowed[sort] {
		sort = "created_at"
	}
	if err := query.Order(sort + " " + direction).Offset((page - 1) * per).Limit(per).Find(dst).Error; err != nil {
		serverError(c)
		return
	}
	response.Paginated(c, dst, page, per, total)
}
func positive(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
func userID(c *gin.Context) uuid.UUID   { id, _ := c.Get("user_id"); return id.(uuid.UUID) }
func branchID(c *gin.Context) uuid.UUID { id, _ := c.Get("branch_id"); return *id.(*uuid.UUID) }
func (h *Handler) findScoped(c *gin.Context, dst any) bool {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID tidak valid")
		return false
	}
	if err := h.DB.Where("id=? AND branch_id=?", id, branchID(c)).First(dst).Error; err != nil {
		response.Error(c, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan")
		return false
	}
	return true
}
func (h *Handler) audit(c *gin.Context, action, resource string, resourceID *uuid.UUID, before, after any) {
	toMap := func(v any) map[string]any {
		if v == nil {
			return nil
		}
		return map[string]any{"snapshot": v}
	}
	uid := userIDSafe(c)
	var bid *uuid.UUID
	if value, ok := c.Get("branch_id"); ok {
		bid, _ = value.(*uuid.UUID)
	}
	_ = h.DB.Create(&model.AuditLog{ID: uuid.New(), BranchID: bid, UserID: uid, Action: action, Resource: resource, ResourceID: resourceID, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.GetString("request_id"), Before: toMap(before), After: toMap(after), CreatedAt: time.Now()}).Error
}
func userIDSafe(c *gin.Context) *uuid.UUID {
	value, ok := c.Get("user_id")
	if !ok {
		return nil
	}
	id, ok := value.(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}
func conflict(c *gin.Context, err error, code, message string) {
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "duplicate key") {
		response.Error(c, http.StatusConflict, code, message)
	} else {
		serverError(c)
	}
}
func serverError(c *gin.Context) {
	response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal")
}
func number(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().Format("20060102"), strings.ToUpper(uuid.NewString()[:6]))
}
