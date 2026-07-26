package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Base struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (b *Base) BeforeCreate(*gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

type Branch struct {
	Base
	Code     string `json:"code"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Phone    string `json:"phone"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
	IsActive bool   `json:"is_active"`
}
type User struct {
	Base
	BranchID     *uuid.UUID `json:"branch_id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Phone        string     `json:"phone"`
	PasswordHash string     `json:"-"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}
type Role struct {
	Base
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Permission struct {
	Base
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type UserRole struct {
	UserID    uuid.UUID  `json:"user_id" gorm:"primaryKey"`
	RoleID    uuid.UUID  `json:"role_id" gorm:"primaryKey"`
	BranchID  *uuid.UUID `json:"branch_id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
}
type RefreshToken struct {
	Base
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	UserAgent string     `json:"user_agent"`
	IPAddress string     `json:"ip_address"`
}
type Customer struct {
	Base
	BranchID uuid.UUID `json:"branch_id"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
	Email    string    `json:"email"`
	Address  string    `json:"address"`
	Notes    string    `json:"notes"`
}
type Vehicle struct {
	Base
	BranchID    uuid.UUID  `json:"branch_id"`
	CustomerID  *uuid.UUID `json:"customer_id"`
	Identifier  string     `json:"identifier"`
	PlateNumber string     `json:"plate_number"`
	Brand       string     `json:"brand"`
	Model       string     `json:"model"`
	Year        int        `json:"year"`
	Color       string     `json:"color"`
	Odometer    int64      `json:"odometer"`
	Notes       string     `json:"notes"`
}
type Product struct {
	Base
	BranchID     *uuid.UUID `json:"branch_id"`
	SKU          string     `json:"sku"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	Unit         string     `json:"unit"`
	CostPrice    int64      `json:"cost_price"`
	SellingPrice int64      `json:"selling_price"`
	MinStock     float64    `json:"min_stock"`
	IsActive     bool       `json:"is_active"`
}
type InventoryBalance struct {
	BranchID  uuid.UUID `json:"branch_id" gorm:"primaryKey"`
	ProductID uuid.UUID `json:"product_id" gorm:"primaryKey"`
	Quantity  float64   `json:"quantity"`
	UpdatedAt time.Time `json:"updated_at"`
}
type InventoryMovement struct {
	Base
	BranchID      uuid.UUID  `json:"branch_id"`
	ProductID     uuid.UUID  `json:"product_id"`
	ReferenceType string     `json:"reference_type"`
	ReferenceID   *uuid.UUID `json:"reference_id"`
	Direction     string     `json:"direction"`
	Quantity      float64    `json:"quantity"`
	UnitCost      int64      `json:"unit_cost"`
	Notes         string     `json:"notes"`
	CreatedBy     uuid.UUID  `json:"created_by"`
}
type WorkOrder struct {
	Base
	BranchID    uuid.UUID  `json:"branch_id"`
	Number      string     `json:"number"`
	CustomerID  uuid.UUID  `json:"customer_id"`
	VehicleID   uuid.UUID  `json:"vehicle_id"`
	MechanicID  *uuid.UUID `json:"mechanic_id"`
	Status      string     `json:"status"`
	Complaint   string     `json:"complaint"`
	Diagnosis   string     `json:"diagnosis"`
	Odometer    int64      `json:"odometer"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
type WorkOrderItem struct {
	Base
	WorkOrderID   uuid.UUID `json:"work_order_id"`
	ProductID     uuid.UUID `json:"product_id"`
	Description   string    `json:"description"`
	Type          string    `json:"type"`
	Quantity      float64   `json:"quantity"`
	UnitPrice     int64     `json:"unit_price"`
	UnitCost      int64     `json:"unit_cost"`
	Discount      int64     `json:"discount"`
	Subtotal      int64     `json:"subtotal"`
	StockDeducted bool      `json:"stock_deducted"`
}
type Sale struct {
	Base
	BranchID     uuid.UUID  `json:"branch_id"`
	Number       string     `json:"number"`
	CustomerID   *uuid.UUID `json:"customer_id"`
	WorkOrderID  *uuid.UUID `json:"work_order_id"`
	CashierID    uuid.UUID  `json:"cashier_id"`
	Status       string     `json:"status"`
	Subtotal     int64      `json:"subtotal"`
	Discount     int64      `json:"discount"`
	Tax          int64      `json:"tax"`
	GatewayFee   int64      `json:"gateway_fee"`
	GrandTotal   int64      `json:"grand_total"`
	AmountPaid   int64      `json:"amount_paid"`
	ChangeAmount int64      `json:"change_amount"`
	Notes        string     `json:"notes"`
	PaidAt       *time.Time `json:"paid_at"`
}
type SaleItem struct {
	Base
	SaleID      uuid.UUID `json:"sale_id"`
	ProductID   uuid.UUID `json:"product_id"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	Quantity    float64   `json:"quantity"`
	UnitPrice   int64     `json:"unit_price"`
	UnitCost    int64     `json:"unit_cost"`
	Discount    int64     `json:"discount"`
	Subtotal    int64     `json:"subtotal"`
}
type Payment struct {
	Base
	BranchID          uuid.UUID      `json:"branch_id"`
	SaleID            uuid.UUID      `json:"sale_id"`
	Method            string         `json:"method"`
	Provider          string         `json:"provider"`
	ProviderReference string         `json:"provider_reference"`
	Status            string         `json:"status"`
	BaseAmount        int64          `json:"base_amount"`
	Amount            int64          `json:"amount"`
	Fee               int64          `json:"fee"`
	CustomerFee       int64          `json:"customer_fee"`
	ProviderFee       int64          `json:"provider_fee"`
	FeeBearer         string         `json:"fee_bearer"`
	PaymentChannel    string         `json:"payment_channel"`
	PaidAt            *time.Time     `json:"paid_at"`
	Metadata          map[string]any `json:"metadata" gorm:"serializer:json"`
}
type Account struct {
	Base
	BranchID *uuid.UUID `json:"branch_id"`
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	ParentID *uuid.UUID `json:"parent_id"`
	IsActive bool       `json:"is_active"`
}
type JournalEntry struct {
	Base
	BranchID      uuid.UUID  `json:"branch_id"`
	Number        string     `json:"number"`
	EntryDate     time.Time  `json:"entry_date"`
	Description   string     `json:"description"`
	ReferenceType string     `json:"reference_type"`
	ReferenceID   *uuid.UUID `json:"reference_id"`
	Status        string     `json:"status"`
	CreatedBy     uuid.UUID  `json:"created_by"`
}
type JournalLine struct {
	Base
	JournalEntryID uuid.UUID `json:"journal_entry_id"`
	AccountID      uuid.UUID `json:"account_id"`
	Description    string    `json:"description"`
	Debit          int64     `json:"debit"`
	Credit         int64     `json:"credit"`
}
type AuditLog struct {
	ID         uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	BranchID   *uuid.UUID     `json:"branch_id"`
	UserID     *uuid.UUID     `json:"user_id"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID *uuid.UUID     `json:"resource_id"`
	IPAddress  string         `json:"ip_address"`
	UserAgent  string         `json:"user_agent"`
	RequestID  string         `json:"request_id"`
	Before     map[string]any `json:"before" gorm:"serializer:json"`
	After      map[string]any `json:"after" gorm:"serializer:json"`
	CreatedAt  time.Time      `json:"created_at"`
}
type CMSPage struct {
	Base
	Slug            string         `json:"slug"`
	Title           string         `json:"title"`
	MetaTitle       string         `json:"meta_title"`
	MetaDescription string         `json:"meta_description"`
	Content         map[string]any `json:"content" gorm:"serializer:json"`
	Status          string         `json:"status"`
	PublishedAt     *time.Time     `json:"published_at"`
}
type Setting struct {
	Base
	BranchID *uuid.UUID      `json:"branch_id"`
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value" gorm:"type:jsonb"`
	IsPublic bool            `json:"is_public"`
}
