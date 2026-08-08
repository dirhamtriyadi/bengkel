package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/model"
	"bengkel/internal/paymentgateway"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errPaymentAttemptNotCurrent = errors.New("payment attempt is not current")
	errPaymentAttemptReplaced   = errors.New("payment attempt is already replaced")
	errPaymentAlreadyPaid       = errors.New("payment is already paid")
	errSaleNotPending           = errors.New("sale is not pending")
)

type changePaymentMethodInput struct {
	Method         string `json:"method" validate:"required,oneof=cash midtrans"`
	AmountReceived int64  `json:"amount_received" validate:"min=0"`
}

type changePaymentMethodResult struct {
	Sale     model.Sale    `json:"sale"`
	Payment  model.Payment `json:"payment"`
	PrintURL string        `json:"print_url"`
}

// ChangePaymentMethod godoc
// @Summary Ganti metode atau buat ulang attempt pembayaran
// @Description Membatalkan sesi/instruksi Midtrans lama, menyimpan attempt lama sebagai riwayat, lalu membuat pembayaran tunai atau attempt Midtrans baru. Hanya invoice pending yang dapat diproses.
// @Tags Payments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "ID payment attempt aktif"
// @Param payload body changePaymentMethodInput true "Metode pembayaran pengganti"
// @Success 201 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /payments/{id}/change-method [post]
func (h *Handler) ChangePaymentMethod(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "ID pembayaran tidak valid")
		return
	}
	var input changePaymentMethodInput
	if !bind(c, &input) {
		return
	}

	var current model.Payment
	if err := h.DB.Where("id=? AND branch_id=?", id, branchID(c)).First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Pembayaran tidak ditemukan")
			return
		}
		serverError(c)
		return
	}
	var sale model.Sale
	if err := h.DB.Where("id=? AND branch_id=?", current.SaleID, branchID(c)).First(&sale).Error; err != nil {
		serverError(c)
		return
	}
	if sale.Status != "pending" {
		response.Error(c, http.StatusConflict, "SALE_NOT_PENDING", "Metode pembayaran hanya dapat diganti pada invoice pending")
		return
	}
	if current.Status == "paid" || current.Status == "refunded" {
		response.Error(c, http.StatusConflict, "PAYMENT_ALREADY_PAID", "Pembayaran sudah berhasil dan tidak dapat diganti")
		return
	}

	baseAmount := current.BaseAmount
	if baseAmount == 0 {
		baseAmount = sale.GrandTotal - sale.GatewayFee
	}
	if baseAmount < 1 {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_AMOUNT_INVALID", "Nominal tagihan tidak valid")
		return
	}
	if input.Method == "cash" && input.AmountReceived < baseAmount {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_AMOUNT_INSUFFICIENT", "Uang diterima kurang dari total tagihan",
			response.FieldError{Field: "amount_received", Rule: "gte", Message: "Uang diterima minimal sebesar total tagihan"})
		return
	}

	var replacementConfiguration midtransRuntimeConfig
	var replacementFeeSettings midtransFeeSettings
	if input.Method == "midtrans" {
		replacementConfiguration, err = h.midtransCredentials(sale.BranchID)
		if err != nil {
			h.respondMidtransConfigurationError(c, err)
			return
		}
		replacementFeeSettings, err = h.midtransFeeSettings(sale.BranchID)
		if err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_FEE_CONFIG_INVALID", err.Error())
			return
		}
	}

	if current.Method == "midtrans" {
		if operationError := h.retireMidtransAttempt(c.Request.Context(), current); operationError != nil {
			operationError.respond(c)
			return
		}
	}

	now := time.Now()
	replacement := model.Payment{
		Base:       model.Base{ID: uuid.New()},
		BranchID:   sale.BranchID,
		SaleID:     sale.ID,
		Method:     input.Method,
		Status:     "pending",
		BaseAmount: baseAmount,
		Amount:     baseAmount,
		FeeBearer:  "merchant",
		Metadata: map[string]any{
			"changed_from_payment_id": current.ID.String(),
			"changed_by":              userID(c).String(),
		},
	}
	if input.Method == "midtrans" {
		replacement.Provider = "midtrans"
		replacement.ProviderReference = replacementMidtransOrderID(sale.Number, replacement.ID)
		replacement.FeeBearer = replacementFeeSettings.feeBearer()
		snapshot, snapshotError := h.encryptMidtransPaymentSnapshot(replacementConfiguration, replacement.BranchID, replacement.ID)
		if snapshotError != nil {
			serverError(c)
			return
		}
		replacement.Metadata[midtransCredentialsSnapshotMetadataKey] = snapshot
	} else {
		replacement.Status = "paid"
		replacement.PaidAt = &now
		replacement.Metadata["amount_received"] = input.AmountReceived
	}

	before := redactedPayment(current)
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		var lockedSale model.Sale
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND branch_id=?", sale.ID, sale.BranchID).First(&lockedSale).Error; err != nil {
			return err
		}
		if lockedSale.Status != "pending" {
			return errSaleNotPending
		}

		var lockedCurrent model.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND branch_id=?", current.ID, current.BranchID).First(&lockedCurrent).Error; err != nil {
			return err
		}
		if lockedCurrent.Status == "paid" || lockedCurrent.Status == "refunded" {
			return errPaymentAlreadyPaid
		}
		if lockedCurrent.Metadata != nil {
			if value, exists := lockedCurrent.Metadata["superseded_by"]; exists && value != nil && value != "" {
				return errPaymentAttemptReplaced
			}
		}
		var latest model.Payment
		if err := tx.Where("sale_id=? AND branch_id=?", sale.ID, sale.BranchID).Order("created_at DESC, id DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.ID != lockedCurrent.ID {
			return errPaymentAttemptNotCurrent
		}

		if err := tx.Create(&replacement).Error; err != nil {
			return err
		}
		if lockedCurrent.Metadata == nil {
			lockedCurrent.Metadata = map[string]any{}
		}
		lockedCurrent.Metadata["superseded_at"] = now.UTC().Format(time.RFC3339Nano)
		lockedCurrent.Metadata["superseded_by"] = replacement.ID.String()
		lockedCurrent.Metadata["superseded_method"] = replacement.Method
		lockedCurrent.Metadata["superseded_by_user"] = userID(c).String()
		if err := tx.Model(&lockedCurrent).Updates(map[string]any{
			"status": "expired", "metadata": lockedCurrent.Metadata, "updated_at": now,
		}).Error; err != nil {
			return err
		}

		if replacement.Method == "midtrans" {
			lockedSale.GatewayFee = 0
			lockedSale.GrandTotal = baseAmount
			lockedSale.AmountPaid = 0
			lockedSale.ChangeAmount = 0
			return tx.Save(&lockedSale).Error
		}

		lockedSale.Status = "paid"
		lockedSale.GatewayFee = 0
		lockedSale.GrandTotal = baseAmount
		lockedSale.AmountPaid = input.AmountReceived
		lockedSale.ChangeAmount = input.AmountReceived - baseAmount
		lockedSale.PaidAt = &now
		if err := tx.Save(&lockedSale).Error; err != nil {
			return err
		}
		var cogs int64
		if err := tx.Model(&model.SaleItem{}).Where("sale_id=?", lockedSale.ID).Select("COALESCE(SUM(unit_cost*quantity),0)").Scan(&cogs).Error; err != nil {
			return err
		}
		sale = lockedSale
		return h.postSaleJournal(tx, lockedSale, cogs, replacement)
	})
	if err != nil {
		switch {
		case errors.Is(err, errPaymentAlreadyPaid):
			response.Error(c, http.StatusConflict, "PAYMENT_ALREADY_PAID", "Pembayaran sudah berhasil dan tidak dapat diganti")
		case errors.Is(err, errSaleNotPending):
			response.Error(c, http.StatusConflict, "SALE_NOT_PENDING", "Invoice tidak lagi berstatus pending")
		case errors.Is(err, errPaymentAttemptNotCurrent), errors.Is(err, errPaymentAttemptReplaced):
			response.Error(c, http.StatusConflict, "PAYMENT_ATTEMPT_NOT_CURRENT", "Attempt pembayaran ini sudah diganti. Muat ulang data terlebih dahulu")
		default:
			serverError(c)
		}
		return
	}
	if replacement.Method == "midtrans" {
		sale.GrandTotal = baseAmount
		sale.GatewayFee = 0
		sale.AmountPaid = 0
		sale.ChangeAmount = 0
	}
	h.audit(c, "change_method", "payment", &replacement.ID, before, map[string]any{
		"payment": redactedPayment(replacement), "replaced_payment_id": current.ID, "method": replacement.Method,
	})
	response.Created(c, changePaymentMethodResult{
		Sale: sale, Payment: redactedPayment(replacement), PrintURL: "/print/receipt/" + sale.ID.String(),
	})
}

func (h *Handler) retireMidtransAttempt(ctx context.Context, payment model.Payment) *operationError {
	token, _ := payment.Metadata["snap_token"].(string)
	if strings.TrimSpace(token) == "" {
		return nil
	}
	configuration, err := h.midtransCredentialsForPayment(payment)
	if err != nil {
		return midtransConfigurationOperationError(err)
	}

	transaction, err := h.checkMidtransTransactionWithConfig(ctx, payment, configuration)
	if err == nil {
		return h.retireStartedMidtransTransaction(ctx, payment, configuration, transaction)
	}
	if !errors.Is(err, errMidtransNotStarted) {
		return &operationError{http.StatusBadGateway, "MIDTRANS_STATUS_UNAVAILABLE", "Status pembayaran lama tidak dapat diverifikasi sebelum metode diganti"}
	}

	gateway := paymentgateway.NewMidtrans(configuration.ServerKey, configuration.production())
	if err := gateway.CancelSnapSession(ctx, token); err != nil {
		if errors.Is(err, paymentgateway.ErrSnapSessionInProgress) {
			transaction, statusErr := h.checkMidtransTransactionWithConfig(ctx, payment, configuration)
			if statusErr == nil {
				return h.retireStartedMidtransTransaction(ctx, payment, configuration, transaction)
			}
		}
		return &operationError{http.StatusBadGateway, "MIDTRANS_SESSION_CANCEL_FAILED", "Sesi Midtrans lama tidak dapat dibatalkan: " + err.Error()}
	}
	return nil
}

func (h *Handler) retireStartedMidtransTransaction(ctx context.Context, payment model.Payment, configuration midtransRuntimeConfig, transaction *paymentgateway.TransactionStatus) *operationError {
	if midtransTransactionSuccessful(transaction) {
		if _, _, err := h.applyMidtransTransaction(payment, transaction); err != nil {
			return &operationError{http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal"}
		}
		return &operationError{http.StatusConflict, "PAYMENT_ALREADY_PAID", "Pembayaran lama ternyata sudah berhasil. Status invoice telah diperbarui dan metode tidak diganti"}
	}
	status := strings.ToLower(transaction.TransactionStatus)
	switch status {
	case "cancel", "deny", "expire", "failure":
		return nil
	case "pending":
		gateway := paymentgateway.NewMidtrans(configuration.ServerKey, configuration.production())
		if err := gateway.ExpireTransaction(ctx, payment.ProviderReference); err != nil {
			return &operationError{http.StatusBadGateway, "MIDTRANS_EXPIRE_FAILED", "Instruksi pembayaran Midtrans lama tidak dapat dikedaluwarsakan: " + err.Error()}
		}
		return nil
	default:
		return &operationError{http.StatusConflict, "PAYMENT_CHANGE_UNSAFE", "Status pembayaran Midtrans lama tidak aman untuk diganti. Sinkronkan status pembayaran terlebih dahulu"}
	}
}

func midtransTransactionSuccessful(transaction *paymentgateway.TransactionStatus) bool {
	if transaction == nil {
		return false
	}
	return transaction.TransactionStatus == "settlement" || (transaction.TransactionStatus == "capture" && transaction.FraudStatus == "accept")
}

func replacementMidtransOrderID(invoiceNumber string, paymentID uuid.UUID) string {
	suffix := "-R-" + strings.ToUpper(strings.ReplaceAll(paymentID.String(), "-", "")[:10])
	base := []rune(strings.TrimSpace(invoiceNumber))
	maximumBaseLength := 50 - len([]rune(suffix))
	if len(base) > maximumBaseLength {
		base = base[:maximumBaseLength]
	}
	if len(base) == 0 {
		base = []rune("PAYMENT")
	}
	return string(base) + suffix
}
