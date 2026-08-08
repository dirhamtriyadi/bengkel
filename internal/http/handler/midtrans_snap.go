package handler

import (
	"context"
	"errors"
	"net/http"

	"bengkel/internal/http/response"
	"bengkel/internal/model"
	"bengkel/internal/paymentgateway"

	"github.com/gin-gonic/gin"
)

type midtransSnapResult struct {
	Token       string `json:"token"`
	RedirectURL any    `json:"redirect_url,omitempty"`
	Environment string `json:"environment"`
	ClientKey   string `json:"client_key"`
	SnapURL     string `json:"snap_url"`
	PublicURL   string `json:"public_url,omitempty"`
}

type operationError struct {
	Status  int
	Code    string
	Message string
}

func (err *operationError) respond(c *gin.Context) {
	response.Error(c, err.Status, err.Code, err.Message)
}

func (h *Handler) getOrCreateMidtransSnap(ctx context.Context, payment *model.Payment, sale model.Sale, customer model.Customer, finishURL string) (midtransSnapResult, *operationError) {
	if payment.Method != "midtrans" || payment.Status != "pending" {
		return midtransSnapResult{}, &operationError{http.StatusUnprocessableEntity, "PAYMENT_NOT_PAYABLE", "Pembayaran ini tidak dapat diproses dengan Midtrans"}
	}
	configuration, err := h.midtransCredentialsForPayment(*payment)
	if err != nil {
		return midtransSnapResult{}, midtransConfigurationOperationError(err)
	}
	if payment.Metadata == nil {
		payment.Metadata = map[string]any{}
	}
	if _, exists := payment.Metadata[midtransCredentialsSnapshotMetadataKey]; !exists {
		snapshot, snapshotError := h.encryptMidtransPaymentSnapshot(configuration, payment.BranchID, payment.ID)
		if snapshotError != nil {
			return midtransSnapResult{}, &operationError{http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal"}
		}
		payment.Metadata[midtransCredentialsSnapshotMetadataKey] = snapshot
	}
	if token, ok := payment.Metadata["snap_token"].(string); ok && token != "" {
		if err := h.DB.WithContext(ctx).Model(payment).Update("metadata", payment.Metadata).Error; err != nil {
			return midtransSnapResult{}, &operationError{http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal"}
		}
		return midtransSnapResult{
			Token: token, RedirectURL: payment.Metadata["redirect_url"], Environment: configuration.Environment,
			ClientKey: configuration.ClientKey, SnapURL: configuration.snapURL(),
		}, nil
	}

	feeSettings, err := h.midtransFeeSettings(payment.BranchID)
	if snapshot, ok := payment.Metadata["fee_config_snapshot"]; ok {
		if saved, snapshotErr := decodeMidtransFeeSnapshot(snapshot); snapshotErr == nil {
			feeSettings = saved
			err = nil
		}
	}
	if err != nil {
		return midtransSnapResult{}, &operationError{http.StatusUnprocessableEntity, "MIDTRANS_FEE_CONFIG_INVALID", err.Error()}
	}
	channels := feeSettings.enabledChannels()
	enabledPayments := make([]string, 0, len(channels))
	feeConfigs := make([]paymentgateway.PaymentFeeConfig, 0, len(channels))
	for _, channel := range channels {
		enabledPayments = append(enabledPayments, channel.PaymentType)
		feeConfigs = append(feeConfigs, paymentgateway.PaymentFeeConfig{
			PaymentType:        channel.PaymentType,
			Acquirer:           channel.Acquirer,
			CustomerPercentage: channel.CustomerPercentage,
		})
	}

	snapAmount := payment.BaseAmount
	if snapAmount == 0 {
		snapAmount = payment.Amount
	}
	gateway := paymentgateway.NewMidtrans(configuration.ServerKey, configuration.production())
	result, err := gateway.CreateSnap(ctx, paymentgateway.SnapInput{
		IdempotencyKey:   payment.ID.String(),
		OrderID:          payment.ProviderReference,
		Amount:           snapAmount,
		ItemID:           payment.SaleID.String(),
		ItemName:         "Pembayaran " + payment.ProviderReference,
		CustomerName:     customer.Name,
		CustomerEmail:    customer.Email,
		CustomerPhone:    customer.Phone,
		FinishURL:        finishURL,
		EnabledPayments:  enabledPayments,
		AutomaticFee:     feeSettings.AutomaticFee,
		PaymentFeeConfig: feeConfigs,
	})
	if err != nil {
		return midtransSnapResult{}, &operationError{http.StatusBadGateway, "MIDTRANS_REJECTED", "Midtrans menolak pembuatan transaksi: " + err.Error()}
	}
	payment.Metadata["snap_token"] = result.Token
	payment.Metadata["redirect_url"] = result.RedirectURL
	payment.Metadata["environment"] = configuration.Environment
	payment.Metadata["sdk"] = "midtrans-go"
	payment.Metadata["automatic_fee"] = feeSettings.AutomaticFee
	payment.Metadata["fee_config_snapshot"] = feeSettings
	if err := h.DB.WithContext(ctx).Model(payment).Update("metadata", payment.Metadata).Error; err != nil {
		return midtransSnapResult{}, &operationError{http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal"}
	}
	return midtransSnapResult{
		Token: result.Token, RedirectURL: result.RedirectURL, Environment: configuration.Environment,
		ClientKey: configuration.ClientKey, SnapURL: configuration.snapURL(),
	}, nil
}

func midtransConfigurationOperationError(err error) *operationError {
	switch {
	case errors.Is(err, errMidtransDisabled):
		return &operationError{http.StatusServiceUnavailable, "MIDTRANS_DISABLED", "Integrasi Midtrans belum diaktifkan pada pengaturan cabang"}
	case errors.Is(err, errMidtransNotConfigured):
		return &operationError{http.StatusServiceUnavailable, "MIDTRANS_NOT_CONFIGURED", "Server Key, Client Key, atau mode Midtrans belum dikonfigurasi pada pengaturan cabang"}
	case errors.Is(err, errMidtransSecretInvalid):
		return &operationError{http.StatusServiceUnavailable, "MIDTRANS_KEY_UNREADABLE", "Key Midtrans tidak dapat dibaca. Simpan ulang key pada pengaturan"}
	default:
		return &operationError{http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan internal"}
	}
}
