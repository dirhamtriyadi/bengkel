package handler

import (
	"net/http"

	"bengkel/internal/http/response"
	"bengkel/internal/model"

	"github.com/gin-gonic/gin"
)

// UpdateVehicle godoc
// @Summary Ubah data kendaraan
// @Tags Vehicles
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Vehicle ID"
// @Param payload body vehicleInput true "Kendaraan"
// @Success 200 {object} response.Envelope
// @Router /vehicles/{id} [put]
func (h *Handler) UpdateVehicle(c *gin.Context) {
	var row model.Vehicle
	if !h.findScoped(c, &row) {
		return
	}
	var in vehicleInput
	if !bind(c, &in) {
		return
	}
	if in.CustomerID != nil {
		var count int64
		h.DB.Model(&model.Customer{}).Where("id=? AND branch_id=?", *in.CustomerID, branchID(c)).Count(&count)
		if count == 0 {
			response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_NOT_FOUND", "Pelanggan tidak ditemukan")
			return
		}
	}
	before := row
	row.CustomerID, row.Identifier, row.PlateNumber = in.CustomerID, in.Identifier, in.PlateNumber
	row.Brand, row.Model, row.Year, row.Color, row.Odometer, row.Notes = in.Brand, in.Model, in.Year, in.Color, in.Odometer, in.Notes
	if err := h.DB.Save(&row).Error; err != nil {
		conflict(c, err, "VEHICLE_EXISTS", "Identitas kendaraan sudah digunakan")
		return
	}
	h.audit(c, "update", "vehicle", &row.ID, before, row)
	response.OK(c, row)
}

// DeleteVehicle godoc
// @Summary Hapus kendaraan yang belum memiliki work order
// @Tags Vehicles
// @Security BearerAuth
// @Produce json
// @Param id path string true "Vehicle ID"
// @Success 200 {object} response.Envelope
// @Router /vehicles/{id} [delete]
func (h *Handler) DeleteVehicle(c *gin.Context) {
	var row model.Vehicle
	if !h.findScoped(c, &row) {
		return
	}
	var count int64
	h.DB.Model(&model.WorkOrder{}).Where("vehicle_id=?", row.ID).Count(&count)
	if count > 0 {
		response.Error(c, http.StatusConflict, "VEHICLE_IN_USE", "Kendaraan memiliki riwayat servis dan tidak dapat dihapus")
		return
	}
	if err := h.DB.Delete(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "delete", "vehicle", &row.ID, row, nil)
	response.OK(c, gin.H{"message": "Kendaraan berhasil dihapus"})
}

// UpdateProduct godoc
// @Summary Ubah barang atau jasa
// @Tags Products
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param payload body productInput true "Produk"
// @Success 200 {object} response.Envelope
// @Router /products/{id} [put]
func (h *Handler) UpdateProduct(c *gin.Context) {
	var row model.Product
	if !h.findScoped(c, &row) {
		return
	}
	var in productInput
	if !bind(c, &in) {
		return
	}
	before := row
	row.SKU, row.Name, row.Type, row.Unit = in.SKU, in.Name, in.Type, in.Unit
	row.CostPrice, row.SellingPrice, row.MinStock = in.CostPrice, in.SellingPrice, in.MinStock
	if in.IsActive != nil {
		row.IsActive = *in.IsActive
	}
	if err := h.DB.Save(&row).Error; err != nil {
		conflict(c, err, "PRODUCT_EXISTS", "SKU sudah digunakan")
		return
	}
	h.audit(c, "update", "product", &row.ID, before, row)
	response.OK(c, row)
}

// DeleteProduct godoc
// @Summary Nonaktifkan produk yang sudah pernah dipakai
// @Tags Products
// @Security BearerAuth
// @Produce json
// @Param id path string true "Product ID"
// @Success 200 {object} response.Envelope
// @Router /products/{id} [delete]
func (h *Handler) DeleteProduct(c *gin.Context) {
	var row model.Product
	if !h.findScoped(c, &row) {
		return
	}
	before := row
	row.IsActive = false
	if err := h.DB.Save(&row).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "deactivate", "product", &row.ID, before, row)
	response.OK(c, gin.H{"message": "Produk dinonaktifkan"})
}

// InventoryMovements godoc
// @Summary Riwayat pergerakan persediaan
// @Tags Inventory
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /inventory/movements [get]
func (h *Handler) InventoryMovements(c *gin.Context) {
	query := h.DB.Table("inventory_movements im").Select("im.*,p.sku,p.name product_name").
		Joins("JOIN products p ON p.id=im.product_id").
		Where("im.branch_id=? AND im.deleted_at IS NULL", branchID(c))
	rows := make([]map[string]any, 0)
	list(c, query, &rows, []string{"p.sku", "p.name", "im.reference_type", "im.notes"})
}
