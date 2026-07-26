package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/http/validation"
	"bengkel/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type intakeCustomerInput struct {
	Name    string `json:"name" validate:"required,max=150"`
	Phone   string `json:"phone" validate:"max=30"`
	Email   string `json:"email" validate:"omitempty,email"`
	Address string `json:"address"`
}

type intakeVehicleInput struct {
	Identifier  string `json:"identifier" validate:"required,max=100"`
	PlateNumber string `json:"plate_number" validate:"max=30"`
	Brand       string `json:"brand" validate:"max=80"`
	Model       string `json:"model" validate:"max=100"`
	Year        int    `json:"year" validate:"min=0"`
	Color       string `json:"color" validate:"max=50"`
}

type intakeInput struct {
	CustomerID *uuid.UUID           `json:"customer_id"`
	VehicleID  *uuid.UUID           `json:"vehicle_id"`
	Customer   *intakeCustomerInput `json:"customer"`
	Vehicle    *intakeVehicleInput  `json:"vehicle"`
	MechanicID *uuid.UUID           `json:"mechanic_id"`
	Complaint  string               `json:"complaint" validate:"required,max=1000"`
	Odometer   int64                `json:"odometer" validate:"min=0"`
}

type workOrderUpdateInput struct {
	MechanicID *uuid.UUID `json:"mechanic_id"`
	Diagnosis  string     `json:"diagnosis" validate:"max=3000"`
	Status     string     `json:"status" validate:"required,oneof=inspection approval in_progress completed cancelled"`
}

type workOrderDetailItem struct {
	model.WorkOrderItem
	ProductSKU string `json:"product_sku"`
}

type workOrderDetail struct {
	WorkOrder model.WorkOrder       `json:"work_order"`
	Customer  model.Customer        `json:"customer"`
	Vehicle   model.Vehicle         `json:"vehicle"`
	Mechanic  *model.User           `json:"mechanic"`
	Items     []workOrderDetailItem `json:"items"`
	Subtotal  int64                 `json:"subtotal"`
	Sale      *model.Sale           `json:"sale,omitempty"`
	Payment   *model.Payment        `json:"payment,omitempty"`
}

var errIntakeCustomerRequired = errors.New("intake customer required")

// IntakeWorkOrder godoc
// @Summary Terima motor baru atau kendaraan terdaftar
// @Tags Work Orders
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body intakeInput true "Penerimaan motor"
// @Success 201 {object} response.Envelope
// @Router /work-orders/intake [post]
func (h *Handler) IntakeWorkOrder(c *gin.Context) {
	var in intakeInput
	if !bind(c, &in) {
		return
	}
	if in.VehicleID == nil && in.Vehicle == nil {
		response.Error(c, http.StatusUnprocessableEntity, "VEHICLE_REQUIRED", "Pilih kendaraan terdaftar atau isi data motor baru")
		return
	}
	if in.CustomerID == nil && in.Customer == nil && in.VehicleID == nil {
		response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_REQUIRED", "Pilih pelanggan atau isi data pelanggan baru")
		return
	}
	if in.Customer != nil {
		if details := validation.Struct(in.Customer); len(details) > 0 {
			response.Error(c, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Validasi pelanggan gagal", details...)
			return
		}
	}
	if in.Vehicle != nil {
		if details := validation.Struct(in.Vehicle); len(details) > 0 {
			response.Error(c, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Validasi kendaraan gagal", details...)
			return
		}
	}
	if in.MechanicID != nil && !h.mechanicExists(*in.MechanicID, branchID(c)) {
		response.Error(c, http.StatusUnprocessableEntity, "MECHANIC_NOT_FOUND", "Montir tidak ditemukan pada cabang aktif")
		return
	}

	var customer model.Customer
	var vehicle model.Vehicle
	var workOrder model.WorkOrder
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if in.VehicleID != nil {
			if err := tx.Where("id=? AND branch_id=?", *in.VehicleID, branchID(c)).First(&vehicle).Error; err != nil {
				return err
			}
			if vehicle.CustomerID != nil {
				if err := tx.Where("id=? AND branch_id=?", *vehicle.CustomerID, branchID(c)).First(&customer).Error; err != nil {
					return err
				}
			}
		}
		if customer.ID == uuid.Nil && in.CustomerID != nil {
			if err := tx.Where("id=? AND branch_id=?", *in.CustomerID, branchID(c)).First(&customer).Error; err != nil {
				return err
			}
		}
		if customer.ID == uuid.Nil {
			if in.Customer == nil {
				return errIntakeCustomerRequired
			}
			customer = model.Customer{
				BranchID: branchID(c), Code: number("CUST"), Name: strings.TrimSpace(in.Customer.Name),
				Phone: in.Customer.Phone, Email: in.Customer.Email, Address: in.Customer.Address,
			}
			if err := tx.Create(&customer).Error; err != nil {
				return err
			}
		}
		if vehicle.ID == uuid.Nil {
			vehicle = model.Vehicle{
				BranchID: branchID(c), CustomerID: &customer.ID, Identifier: strings.TrimSpace(in.Vehicle.Identifier),
				PlateNumber: strings.TrimSpace(in.Vehicle.PlateNumber), Brand: in.Vehicle.Brand, Model: in.Vehicle.Model,
				Year: in.Vehicle.Year, Color: in.Vehicle.Color, Odometer: in.Odometer,
			}
			if err := tx.Create(&vehicle).Error; err != nil {
				return err
			}
		}
		workOrder = model.WorkOrder{
			BranchID: branchID(c), Number: number("WO"), CustomerID: customer.ID, VehicleID: vehicle.ID,
			MechanicID: in.MechanicID, Status: "inspection", Complaint: strings.TrimSpace(in.Complaint), Odometer: in.Odometer,
		}
		return tx.Create(&workOrder).Error
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			response.Error(c, http.StatusConflict, "INTAKE_CONFLICT", "Identitas kendaraan sudah digunakan")
		} else if errors.Is(err, errIntakeCustomerRequired) {
			response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_REQUIRED", "Kendaraan belum memiliki pelanggan; pilih atau isi data pelanggan")
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(c, http.StatusUnprocessableEntity, "REFERENCE_NOT_FOUND", "Pelanggan atau kendaraan tidak ditemukan")
		} else {
			serverError(c)
		}
		return
	}
	h.audit(c, "intake", "work_order", &workOrder.ID, nil, map[string]any{"work_order": workOrder, "customer": customer, "vehicle": vehicle})
	response.Created(c, gin.H{"work_order": workOrder, "customer": customer, "vehicle": vehicle})
}

// WorkOrderDetail godoc
// @Summary Detail pekerjaan, motor, pelanggan, dan item terpakai
// @Tags Work Orders
// @Security BearerAuth
// @Produce json
// @Param id path string true "Work order ID"
// @Success 200 {object} response.Envelope
// @Router /work-orders/{id} [get]
func (h *Handler) WorkOrderDetail(c *gin.Context) {
	var row model.WorkOrder
	if !h.findScoped(c, &row) {
		return
	}
	var customer model.Customer
	var vehicle model.Vehicle
	if err := h.DB.First(&customer, "id=?", row.CustomerID).Error; err != nil {
		serverError(c)
		return
	}
	if err := h.DB.First(&vehicle, "id=?", row.VehicleID).Error; err != nil {
		serverError(c)
		return
	}
	var mechanic *model.User
	if row.MechanicID != nil {
		var user model.User
		if err := h.DB.First(&user, "id=?", row.MechanicID).Error; err == nil {
			mechanic = &user
		}
	}
	items := make([]workOrderDetailItem, 0)
	if err := h.DB.Table("work_order_items woi").Select("woi.*,p.sku product_sku").
		Joins("JOIN products p ON p.id=woi.product_id").
		Where("woi.work_order_id=? AND woi.deleted_at IS NULL", row.ID).Order("woi.created_at").Scan(&items).Error; err != nil {
		serverError(c)
		return
	}
	var subtotal int64
	for _, item := range items {
		subtotal += item.Subtotal
	}
	var sale *model.Sale
	var payment *model.Payment
	var activeSale model.Sale
	if err := h.DB.Where("work_order_id=? AND status NOT IN ('void','refunded')", row.ID).Order("created_at DESC").First(&activeSale).Error; err == nil {
		sale = &activeSale
		var activePayment model.Payment
		if err := h.DB.Where("sale_id=?", activeSale.ID).Order("created_at DESC").First(&activePayment).Error; err == nil {
			payment = &activePayment
		}
	}
	response.OK(c, workOrderDetail{row, customer, vehicle, mechanic, items, subtotal, sale, payment})
}

// UpdateWorkOrder godoc
// @Summary Perbarui diagnosis, montir, dan status pekerjaan
// @Tags Work Orders
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Work order ID"
// @Param payload body workOrderUpdateInput true "Pekerjaan"
// @Success 200 {object} response.Envelope
// @Router /work-orders/{id} [patch]
func (h *Handler) UpdateWorkOrder(c *gin.Context) {
	var row model.WorkOrder
	if !h.findScoped(c, &row) {
		return
	}
	var in workOrderUpdateInput
	if !bind(c, &in) {
		return
	}
	if row.Status == "invoiced" || row.Status == "cancelled" {
		response.Error(c, http.StatusConflict, "WORK_ORDER_LOCKED", "Work order sudah dikunci")
		return
	}
	if !validWorkOrderTransition(row.Status, in.Status) {
		response.Error(c, http.StatusConflict, "STATUS_TRANSITION_INVALID", "Perubahan status work order tidak valid")
		return
	}
	if in.MechanicID != nil && !h.mechanicExists(*in.MechanicID, branchID(c)) {
		response.Error(c, http.StatusUnprocessableEntity, "MECHANIC_NOT_FOUND", "Montir tidak ditemukan pada cabang aktif")
		return
	}
	if in.Status == "completed" {
		var count int64
		h.DB.Model(&model.WorkOrderItem{}).Where("work_order_id=?", row.ID).Count(&count)
		if count == 0 {
			response.Error(c, http.StatusConflict, "WORK_ORDER_EMPTY", "Tambahkan minimal satu barang atau jasa sebelum menyelesaikan pekerjaan")
			return
		}
	}
	before := row
	row.MechanicID, row.Diagnosis, row.Status = in.MechanicID, strings.TrimSpace(in.Diagnosis), in.Status
	now := time.Now()
	if in.Status == "in_progress" && row.StartedAt == nil {
		row.StartedAt = &now
	}
	if in.Status == "completed" {
		row.CompletedAt = &now
	}
	if err := h.DB.Save(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "update", "work_order", &row.ID, before, row)
	response.OK(c, row)
}

// DeleteWorkOrderItem godoc
// @Summary Hapus item dan kembalikan stok barang
// @Tags Work Orders
// @Security BearerAuth
// @Produce json
// @Param id path string true "Work order ID"
// @Param item_id path string true "Item ID"
// @Success 200 {object} response.Envelope
// @Router /work-orders/{id}/items/{item_id} [delete]
func (h *Handler) DeleteWorkOrderItem(c *gin.Context) {
	workOrderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID work order tidak valid")
		return
	}
	itemID, err := uuid.Parse(c.Param("item_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID item tidak valid")
		return
	}
	var workOrder model.WorkOrder
	if err := h.DB.Where("id=? AND branch_id=?", workOrderID, branchID(c)).First(&workOrder).Error; err != nil {
		response.Error(c, http.StatusNotFound, "WORK_ORDER_NOT_FOUND", "Work order tidak ditemukan")
		return
	}
	if workOrder.Status == "completed" || workOrder.Status == "invoiced" || workOrder.Status == "cancelled" {
		response.Error(c, http.StatusConflict, "WORK_ORDER_LOCKED", "Item pada work order ini sudah dikunci")
		return
	}
	var item model.WorkOrderItem
	if err := h.DB.Where("id=? AND work_order_id=?", itemID, workOrder.ID).First(&item).Error; err != nil {
		response.Error(c, http.StatusNotFound, "ITEM_NOT_FOUND", "Item work order tidak ditemukan")
		return
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if item.Type == "part" && item.StockDeducted {
			var balance model.InventoryBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("branch_id=? AND product_id=?", branchID(c), item.ProductID).First(&balance).Error; err != nil {
				return err
			}
			if err := tx.Model(&balance).Updates(map[string]any{"quantity": gorm.Expr("quantity + ?", item.Quantity), "updated_at": time.Now()}).Error; err != nil {
				return err
			}
			ref := item.ID
			if err := tx.Create(&model.InventoryMovement{BranchID: branchID(c), ProductID: item.ProductID, ReferenceType: "work_order_return", ReferenceID: &ref, Direction: "in", Quantity: item.Quantity, UnitCost: item.UnitCost, Notes: "Pengembalian item " + workOrder.Number, CreatedBy: userID(c)}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&item).Error
	})
	if err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete_item", "work_order", &workOrder.ID, item, nil)
	response.OK(c, gin.H{"message": "Item dihapus dan stok dikembalikan"})
}

// Mechanics godoc
// @Summary Daftar montir aktif pada cabang
// @Tags Work Orders
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /work-orders/options/mechanics [get]
func (h *Handler) Mechanics(c *gin.Context) {
	type mechanic struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	rows := make([]mechanic, 0)
	err := h.DB.Table("users u").Distinct("u.id,u.name").Joins("JOIN user_roles ur ON ur.user_id=u.id").Joins("JOIN roles r ON r.id=ur.role_id").
		Where("u.branch_id=? AND u.is_active=true AND u.deleted_at IS NULL AND r.code='mechanic'", branchID(c)).Order("u.name").Scan(&rows).Error
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, rows)
}

func (h *Handler) mechanicExists(id, branchID uuid.UUID) bool {
	var count int64
	h.DB.Table("users u").Joins("JOIN user_roles ur ON ur.user_id=u.id").Joins("JOIN roles r ON r.id=ur.role_id").
		Where("u.id=? AND u.branch_id=? AND u.is_active=true AND u.deleted_at IS NULL AND r.code='mechanic'", id, branchID).Count(&count)
	return count > 0
}

func validWorkOrderTransition(from, to string) bool {
	if from == to {
		return true
	}
	if to == "cancelled" {
		return from != "completed" && from != "invoiced"
	}
	allowed := map[string]string{"inspection": "approval", "approval": "in_progress", "in_progress": "completed"}
	return allowed[from] == to
}
