package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/model"
	"bengkel/internal/whatsapp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errPublicInvoiceNotFound = errors.New("public invoice not found")

type publicInvoiceBundle struct {
	Link     model.PublicInvoiceLink
	Sale     model.Sale
	Payment  model.Payment
	Customer model.Customer
	Branch   model.Branch
	Items    []model.SaleItem
}

type publicInvoiceItem struct {
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   int64   `json:"unit_price"`
	Discount    int64   `json:"discount"`
	Subtotal    int64   `json:"subtotal"`
}

type publicInvoiceData struct {
	Invoice struct {
		Number     string    `json:"number"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
		Subtotal   int64     `json:"subtotal"`
		Discount   int64     `json:"discount"`
		Tax        int64     `json:"tax"`
		GatewayFee int64     `json:"gateway_fee"`
		GrandTotal int64     `json:"grand_total"`
		AmountPaid int64     `json:"amount_paid"`
	} `json:"invoice"`
	Customer struct {
		Name string `json:"name"`
	} `json:"customer"`
	Branch struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
	} `json:"branch"`
	Payment struct {
		Method         string `json:"method"`
		Status         string `json:"status"`
		BaseAmount     int64  `json:"base_amount"`
		Amount         int64  `json:"amount"`
		CustomerFee    int64  `json:"customer_fee"`
		FeeBearer      string `json:"fee_bearer"`
		PaymentChannel string `json:"payment_channel"`
		Environment    string `json:"environment"`
		Payable        bool   `json:"payable"`
	} `json:"payment"`
	Items     []publicInvoiceItem `json:"items"`
	ExpiresAt time.Time           `json:"expires_at"`
}

// PublicInvoice godoc
// @Summary Invoice publik berdasarkan bearer token
// @Description Menampilkan invoice tanpa login. Token acak pada URL menjadi kredensial akses dan memiliki masa berlaku.
// @Tags Public Invoices
// @Produce json
// @Param token path string true "Public invoice token"
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /public/invoices/{token} [get]
func (h *Handler) PublicInvoice(c *gin.Context) {
	securePublicInvoiceHeaders(c)
	bundle, err := h.loadPublicInvoice(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.respondPublicInvoiceError(c, err)
		return
	}
	response.OK(c, h.publicInvoiceData(bundle))
}

// PublicInvoiceMidtransSnap godoc
// @Summary Mulai atau lanjutkan pembayaran invoice publik
// @Description Membuat atau mengambil ulang token Midtrans Snap tanpa mewajibkan customer login.
// @Tags Public Invoices
// @Produce json
// @Param token path string true "Public invoice token"
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /public/invoices/{token}/midtrans/snap [post]
func (h *Handler) PublicInvoiceMidtransSnap(c *gin.Context) {
	securePublicInvoiceHeaders(c)
	rawToken := c.Param("token")
	bundle, err := h.loadPublicInvoice(c.Request.Context(), rawToken)
	if err != nil {
		h.respondPublicInvoiceError(c, err)
		return
	}
	if bundle.Sale.Status != "pending" || bundle.Payment.Method != "midtrans" || bundle.Payment.Status != "pending" {
		response.Error(c, http.StatusUnprocessableEntity, "INVOICE_NOT_PAYABLE", "Invoice ini tidak dapat dibayar atau sudah selesai")
		return
	}
	snapToken, hasSnapTokenValue := bundle.Payment.Metadata["snap_token"].(string)
	hadSnapToken := hasSnapTokenValue && snapToken != ""
	finishURL := strings.TrimRight(h.Config.FrontendURL, "/") + "/invoice/" + rawToken
	result, operationError := h.getOrCreateMidtransSnap(c.Request.Context(), &bundle.Payment, bundle.Sale, bundle.Customer, finishURL)
	if operationError != nil {
		operationError.respond(c)
		return
	}
	if !hadSnapToken {
		h.auditPublic(c, "start_public_payment", "payment", bundle.Payment.ID, bundle.Sale.BranchID, map[string]any{"invoice": bundle.Sale.Number})
	}
	response.OK(c, result)
}

// PublicInvoiceMidtransSync godoc
// @Summary Sinkronkan status pembayaran invoice publik
// @Tags Public Invoices
// @Produce json
// @Param token path string true "Public invoice token"
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /public/invoices/{token}/midtrans/sync [post]
func (h *Handler) PublicInvoiceMidtransSync(c *gin.Context) {
	securePublicInvoiceHeaders(c)
	bundle, err := h.loadPublicInvoice(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.respondPublicInvoiceError(c, err)
		return
	}
	if bundle.Payment.Method != "midtrans" {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_NOT_MIDTRANS", "Pembayaran invoice ini bukan transaksi Midtrans")
		return
	}
	if bundle.Payment.Status == "paid" || bundle.Sale.Status == "paid" {
		response.OK(c, gin.H{"status": "paid", "already_processed": true})
		return
	}
	transaction, err := h.checkMidtransTransaction(c.Request.Context(), bundle.Payment)
	if err != nil {
		if errors.Is(err, errMidtransNotStarted) {
			response.OK(c, gin.H{"status": "pending", "already_processed": true, "not_started": true})
			return
		}
		h.respondMidtransStatusError(c, err)
		return
	}
	status, alreadyProcessed, err := h.applyMidtransTransaction(bundle.Payment, transaction)
	if err != nil {
		serverError(c)
		return
	}
	if !alreadyProcessed {
		h.auditPublic(c, "sync_public_payment", "payment", bundle.Payment.ID, bundle.Sale.BranchID, map[string]any{"invoice": bundle.Sale.Number, "status": status})
	}
	response.OK(c, gin.H{"status": status, "already_processed": alreadyProcessed})
}

// SendPublicInvoiceWhatsApp godoc
// @Summary Kirim invoice publik ke WhatsApp pelanggan
// @Description Nomor selalu diambil dari customer sale, diverifikasi melalui wwebjs-api, lalu dikirimi bearer link yang kedaluwarsa.
// @Tags Sales
// @Security BearerAuth
// @Produce json
// @Param id path string true "Sale ID"
// @Success 200 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Failure 503 {object} response.Envelope
// @Router /sales/{id}/public-invoice/whatsapp [post]
func (h *Handler) SendPublicInvoiceWhatsApp(c *gin.Context) {
	var sale model.Sale
	if !h.findScoped(c, &sale) {
		return
	}
	if sale.Status != "pending" {
		response.Error(c, http.StatusUnprocessableEntity, "INVOICE_NOT_PAYABLE", "Hanya invoice pending yang dapat dikirim untuk pembayaran")
		return
	}
	if sale.CustomerID == nil {
		response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_REQUIRED", "Transaksi belum memiliki pelanggan")
		return
	}
	var customer model.Customer
	if err := h.DB.Where("id=? AND branch_id=?", *sale.CustomerID, sale.BranchID).First(&customer).Error; err != nil {
		serverError(c)
		return
	}
	number, err := whatsapp.NormalizeNumber(customer.Phone)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_WHATSAPP_INVALID", "Nomor WhatsApp pelanggan kosong atau tidak valid")
		return
	}
	var payment model.Payment
	if err := h.DB.Where("sale_id=? AND branch_id=?", sale.ID, sale.BranchID).Order("created_at DESC, id DESC").First(&payment).Error; err != nil {
		serverError(c)
		return
	}
	if payment.Method != "midtrans" || payment.Status != "pending" {
		response.Error(c, http.StatusUnprocessableEntity, "PAYMENT_NOT_PAYABLE", "Invoice tidak memiliki pembayaran Midtrans yang masih pending")
		return
	}
	sender, _, err := h.whatsAppClient(sale.BranchID)
	if err != nil {
		h.respondWhatsAppConfigurationError(c, err)
		return
	}

	registered, err := sender.IsRegistered(c.Request.Context(), number)
	if err != nil {
		h.audit(c, "send_whatsapp_failed", "sale", &sale.ID, nil, map[string]any{"reason": whatsAppFailureCode(err)})
		h.respondWhatsAppProviderError(c, err)
		return
	}
	if !registered {
		h.audit(c, "send_whatsapp_failed", "sale", &sale.ID, nil, map[string]any{"reason": "not_registered"})
		response.Error(c, http.StatusUnprocessableEntity, "CUSTOMER_NOT_ON_WHATSAPP", "Nomor pelanggan tidak terdaftar di WhatsApp")
		return
	}

	creator := userID(c)
	link, _, publicURL, err := h.issuePublicInvoiceLink(sale.BranchID, sale.ID, &creator, "whatsapp")
	if err != nil {
		serverError(c)
		return
	}
	var branch model.Branch
	if err := h.DB.First(&branch, "id=?", sale.BranchID).Error; err != nil {
		h.revokeFailedInvoiceLink(link, err)
		serverError(c)
		return
	}
	message := publicInvoiceMessage(customer.Name, branch, sale, publicURL, link.ExpiresAt)
	sent, err := sender.SendText(c.Request.Context(), number, message)
	if err != nil {
		h.revokeFailedInvoiceLink(link, err)
		h.audit(c, "send_whatsapp_failed", "sale", &sale.ID, nil, map[string]any{"reason": whatsAppFailureCode(err)})
		h.respondWhatsAppProviderError(c, err)
		return
	}
	now := time.Now()
	if err := h.DB.Model(&link).Updates(map[string]any{
		"delivered_at":        now,
		"recipient_phone":     number,
		"provider_message_id": truncateText(sent.MessageID, 255),
		"delivery_error":      "",
		"updated_at":          now,
	}).Error; err != nil {
		serverError(c)
		return
	}
	h.audit(c, "send_whatsapp", "sale", &sale.ID, nil, map[string]any{
		"invoice": sale.Number, "recipient": maskPhone(number), "expires_at": link.ExpiresAt,
	})
	response.OK(c, gin.H{
		"invoice_number": sale.Number,
		"public_url":     publicURL,
		"expires_at":     link.ExpiresAt,
		"whatsapp": gin.H{
			"sent": true, "recipient": maskPhone(number), "message_id": sent.MessageID,
		},
	})
}

func (h *Handler) issuePublicInvoiceLink(branchID, saleID uuid.UUID, createdBy *uuid.UUID, source string) (model.PublicInvoiceLink, string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return model.PublicInvoiceLink{}, "", "", fmt.Errorf("generate public invoice token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	ttl := h.Config.PublicInvoiceTTL
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	link := model.PublicInvoiceLink{
		BranchID: branchID, SaleID: saleID, CreatedBy: createdBy, TokenHash: publicInvoiceTokenHash(token),
		Source: source, ExpiresAt: time.Now().Add(ttl),
	}
	if err := h.DB.Create(&link).Error; err != nil {
		return model.PublicInvoiceLink{}, "", "", err
	}
	publicURL := strings.TrimRight(h.Config.FrontendURL, "/") + "/invoice/" + token
	return link, token, publicURL, nil
}

func (h *Handler) loadPublicInvoice(ctx context.Context, token string) (publicInvoiceBundle, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return publicInvoiceBundle{}, errPublicInvoiceNotFound
	}
	db := h.DB.WithContext(ctx)
	var bundle publicInvoiceBundle
	if err := db.Where("token_hash=? AND revoked_at IS NULL AND expires_at>now()", publicInvoiceTokenHash(token)).First(&bundle.Link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return publicInvoiceBundle{}, errPublicInvoiceNotFound
		}
		return publicInvoiceBundle{}, err
	}
	if err := db.Where("id=? AND branch_id=?", bundle.Link.SaleID, bundle.Link.BranchID).First(&bundle.Sale).Error; err != nil {
		return publicInvoiceBundle{}, err
	}
	if err := db.Where("sale_id=? AND branch_id=?", bundle.Sale.ID, bundle.Sale.BranchID).Order("created_at DESC, id DESC").First(&bundle.Payment).Error; err != nil {
		return publicInvoiceBundle{}, err
	}
	if bundle.Sale.CustomerID != nil {
		if err := db.Select("id", "name").Where("id=? AND branch_id=?", *bundle.Sale.CustomerID, bundle.Sale.BranchID).First(&bundle.Customer).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return publicInvoiceBundle{}, err
		}
	}
	if err := db.Select("id", "name", "address", "phone", "timezone").First(&bundle.Branch, "id=?", bundle.Sale.BranchID).Error; err != nil {
		return publicInvoiceBundle{}, err
	}
	if err := db.Where("sale_id=?", bundle.Sale.ID).Order("created_at").Find(&bundle.Items).Error; err != nil {
		return publicInvoiceBundle{}, err
	}
	return bundle, nil
}

func (h *Handler) publicInvoiceData(bundle publicInvoiceBundle) publicInvoiceData {
	var data publicInvoiceData
	data.Invoice.Number = bundle.Sale.Number
	data.Invoice.Status = bundle.Sale.Status
	data.Invoice.CreatedAt = bundle.Sale.CreatedAt
	data.Invoice.Subtotal = bundle.Sale.Subtotal
	data.Invoice.Discount = bundle.Sale.Discount
	data.Invoice.Tax = bundle.Sale.Tax
	data.Invoice.GatewayFee = bundle.Sale.GatewayFee
	data.Invoice.GrandTotal = bundle.Sale.GrandTotal
	data.Invoice.AmountPaid = bundle.Sale.AmountPaid
	data.Customer.Name = bundle.Customer.Name
	data.Branch.Name = bundle.Branch.Name
	data.Branch.Address = bundle.Branch.Address
	data.Branch.Phone = bundle.Branch.Phone
	data.Payment.Method = bundle.Payment.Method
	data.Payment.Status = bundle.Payment.Status
	data.Payment.BaseAmount = bundle.Payment.BaseAmount
	data.Payment.Amount = bundle.Payment.Amount
	data.Payment.CustomerFee = bundle.Payment.CustomerFee
	data.Payment.FeeBearer = bundle.Payment.FeeBearer
	data.Payment.PaymentChannel = bundle.Payment.PaymentChannel
	data.Payment.Environment = "sandbox"
	configuration, configurationError := h.midtransCredentialsForPayment(bundle.Payment)
	if configurationError == nil {
		data.Payment.Environment = configuration.Environment
	}
	data.Payment.Payable = configurationError == nil && bundle.Sale.Status == "pending" && bundle.Payment.Method == "midtrans" && bundle.Payment.Status == "pending"
	data.ExpiresAt = bundle.Link.ExpiresAt
	data.Items = make([]publicInvoiceItem, 0, len(bundle.Items))
	for _, item := range bundle.Items {
		data.Items = append(data.Items, publicInvoiceItem{
			Description: item.Description, Type: item.Type, Quantity: item.Quantity, UnitPrice: item.UnitPrice,
			Discount: item.Discount, Subtotal: item.Subtotal,
		})
	}
	return data
}

func (h *Handler) respondPublicInvoiceError(c *gin.Context, err error) {
	if errors.Is(err, errPublicInvoiceNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(c, http.StatusNotFound, "PUBLIC_INVOICE_NOT_FOUND", "Tautan invoice tidak ditemukan atau sudah kedaluwarsa")
		return
	}
	serverError(c)
}

func (h *Handler) revokeFailedInvoiceLink(link model.PublicInvoiceLink, providerError error) {
	now := time.Now()
	_ = h.DB.Model(&link).Updates(map[string]any{
		"revoked_at": now, "delivery_error": truncateText(providerError.Error(), 500), "updated_at": now,
	}).Error
}

func (h *Handler) respondWhatsAppProviderError(c *gin.Context, err error) {
	var providerError *whatsapp.Error
	if errors.As(err, &providerError) {
		message := strings.ToLower(providerError.Message)
		if strings.Contains(message, "qr code not ready") {
			response.Error(c, http.StatusConflict, "WHATSAPP_QR_NOT_READY", "QR belum tersedia atau session sudah terhubung. Muat ulang status lalu coba lagi")
			return
		}
		if providerError.StatusCode == http.StatusNotFound || strings.Contains(message, "session") {
			response.Error(c, http.StatusServiceUnavailable, "WHATSAPP_SESSION_NOT_READY", "Sesi WhatsApp belum terhubung. Mulai sesi dan pindai QR terlebih dahulu")
			return
		}
		if providerError.StatusCode == http.StatusForbidden {
			response.Error(c, http.StatusBadGateway, "WHATSAPP_AUTH_FAILED", "API key wwebjs-api ditolak")
			return
		}
		if providerError.StatusCode == http.StatusUnprocessableEntity {
			response.Error(c, http.StatusUnprocessableEntity, "WHATSAPP_SESSION_START_FAILED", "Session WhatsApp tidak dapat dimulai. Periksa nomor session dan status service")
			return
		}
	}
	response.Error(c, http.StatusBadGateway, "WHATSAPP_SEND_FAILED", "Invoice belum berhasil dikirim melalui WhatsApp")
}

func whatsAppFailureCode(err error) string {
	var providerError *whatsapp.Error
	if errors.As(err, &providerError) {
		return fmt.Sprintf("provider_http_%d", providerError.StatusCode)
	}
	return "provider_unavailable"
}

func publicInvoiceTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func publicInvoiceMessage(customerName string, branch model.Branch, sale model.Sale, publicURL string, expiresAt time.Time) string {
	location := time.Local
	if branch.Timezone != "" {
		if loaded, err := time.LoadLocation(branch.Timezone); err == nil {
			location = loaded
		}
	}
	name := singleLine(customerName)
	if name == "" {
		name = "Pelanggan"
	}
	branchName := singleLine(branch.Name)
	if branchName == "" {
		branchName = "Bengkel"
	}
	return fmt.Sprintf("Halo %s,\n\nBerikut invoice *%s* dari *%s*.\nTotal tagihan: *%s*\n\nBayar secara online melalui tautan berikut:\n%s\n\nTautan berlaku sampai %s. Biaya admin, jika dibebankan kepada pelanggan, akan mengikuti channel yang dipilih di Midtrans. Jangan bagikan tautan ini kepada orang lain. Abaikan pesan ini jika pembayaran sudah selesai.",
		name, singleLine(sale.Number), branchName, formatRupiah(sale.GrandTotal), publicURL,
		expiresAt.In(location).Format("02 Jan 2006 15:04 MST"))
}

func formatRupiah(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := fmt.Sprintf("%d", value)
	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "." + digits[index:]
	}
	if negative {
		digits = "-" + digits
	}
	return "Rp" + digits
}

func singleLine(value string) string {
	return truncateText(strings.Join(strings.Fields(value), " "), 150)
}

func truncateText(value string, maximum int) string {
	characters := []rune(value)
	if len(characters) <= maximum {
		return value
	}
	return string(characters[:maximum])
}

func maskPhone(number string) string {
	if len(number) <= 6 {
		return strings.Repeat("*", len(number))
	}
	return number[:2] + strings.Repeat("*", len(number)-6) + number[len(number)-4:]
}

func securePublicInvoiceHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
}

func (h *Handler) auditPublic(c *gin.Context, action, resource string, resourceID, branchID uuid.UUID, after map[string]any) {
	h.DB.Create(&model.AuditLog{
		ID: uuid.New(), BranchID: &branchID, Action: action, Resource: resource, ResourceID: &resourceID,
		IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.GetString("request_id"),
		After: map[string]any{"snapshot": after}, CreatedAt: time.Now(),
	})
}
