package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bengkel/internal/http/response"
	"bengkel/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	midtransSettingKey                     = "integration.midtrans"
	midtransCredentialsSnapshotMetadataKey = "midtrans_credentials_snapshot"
	midtransSandboxSnapURL                 = "https://app.sandbox.midtrans.com/snap/snap.js"
	midtransProductionSnapURL              = "https://app.midtrans.com/snap/snap.js"
)

var (
	errMidtransNotConfigured = errors.New("Midtrans integration is not configured")
	errMidtransDisabled      = errors.New("Midtrans integration is disabled")
	errMidtransSecretInvalid = errors.New("Midtrans credentials cannot be decrypted")
)

type midtransStoredSettings struct {
	Enabled             bool   `json:"enabled"`
	Environment         string `json:"environment"`
	MerchantID          string `json:"merchant_id"`
	ServerKeyCiphertext string `json:"server_key_ciphertext"`
	ClientKeyCiphertext string `json:"client_key_ciphertext"`
}

type midtransSettingsView struct {
	Enabled             bool   `json:"enabled"`
	Environment         string `json:"environment"`
	MerchantID          string `json:"merchant_id"`
	ServerKeyConfigured bool   `json:"server_key_configured"`
	ClientKeyConfigured bool   `json:"client_key_configured"`
}

type midtransSettingsInput struct {
	Enabled        bool   `json:"enabled"`
	Environment    string `json:"environment" validate:"required,oneof=sandbox production"`
	MerchantID     string `json:"merchant_id" validate:"max=100"`
	ServerKey      string `json:"server_key" validate:"max=2048"`
	ClientKey      string `json:"client_key" validate:"max=2048"`
	ClearServerKey bool   `json:"clear_server_key"`
	ClearClientKey bool   `json:"clear_client_key"`
}

type midtransRuntimeConfig struct {
	Environment string `json:"environment"`
	MerchantID  string `json:"merchant_id,omitempty"`
	ServerKey   string `json:"server_key"`
	ClientKey   string `json:"client_key"`
}

func (configuration midtransRuntimeConfig) production() bool {
	return configuration.Environment == "production"
}

func (configuration midtransRuntimeConfig) snapURL() string {
	if configuration.production() {
		return midtransProductionSnapURL
	}
	return midtransSandboxSnapURL
}

// MidtransIntegration godoc
// @Summary Konfigurasi Midtrans cabang
// @Description Server Key dan Client Key tidak pernah dikembalikan; response hanya menunjukkan apakah key terenkripsi sudah tersimpan.
// @Tags Payments
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /integrations/midtrans [get]
func (h *Handler) MidtransIntegration(c *gin.Context) {
	stored, err := h.loadMidtransSettings(branchID(c))
	if errors.Is(err, errMidtransNotConfigured) {
		response.OK(c, midtransSettingsView{Environment: "sandbox"})
		return
	}
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, midtransView(stored))
}

// UpdateMidtransIntegration godoc
// @Summary Simpan konfigurasi Midtrans cabang
// @Description Merchant ID, mode, Server Key, dan Client Key disimpan di database. Kedua key dienkripsi dan input kosong mempertahankan key lama.
// @Tags Payments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body midtransSettingsInput true "Konfigurasi Midtrans"
// @Success 200 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /integrations/midtrans [put]
func (h *Handler) UpdateMidtransIntegration(c *gin.Context) {
	var input midtransSettingsInput
	if !bind(c, &input) {
		return
	}
	bid := branchID(c)
	stored, err := h.loadMidtransSettings(bid)
	if errors.Is(err, errMidtransNotConfigured) {
		stored = midtransStoredSettings{Environment: "sandbox"}
		err = nil
	}
	if err != nil {
		serverError(c)
		return
	}
	before := midtransView(stored)

	stored.Enabled = input.Enabled
	stored.Environment = strings.TrimSpace(input.Environment)
	stored.MerchantID = strings.TrimSpace(input.MerchantID)
	if input.ClearServerKey {
		stored.ServerKeyCiphertext = ""
	}
	if input.ClearClientKey {
		stored.ClientKeyCiphertext = ""
	}
	if key := strings.TrimSpace(input.ServerKey); key != "" {
		if err := validateMidtransSecret("Server Key", key); err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_SERVER_KEY_INVALID", err.Error())
			return
		}
		stored.ServerKeyCiphertext, err = h.encryptMidtransSettingSecret(key, "server_key", bid)
		if err != nil {
			serverError(c)
			return
		}
	}
	if key := strings.TrimSpace(input.ClientKey); key != "" {
		if err := validateMidtransSecret("Client Key", key); err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_CLIENT_KEY_INVALID", err.Error())
			return
		}
		stored.ClientKeyCiphertext, err = h.encryptMidtransSettingSecret(key, "client_key", bid)
		if err != nil {
			serverError(c)
			return
		}
	}

	if stored.Enabled && (stored.ServerKeyCiphertext == "" || stored.ClientKeyCiphertext == "") {
		response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_CONFIG_INCOMPLETE", "Server Key dan Client Key wajib diisi sebelum integrasi diaktifkan")
		return
	}
	if stored.ServerKeyCiphertext != "" && stored.ClientKeyCiphertext != "" {
		configuration, configurationError := h.runtimeMidtransConfig(stored, bid)
		if configurationError != nil {
			h.respondMidtransConfigurationError(c, configurationError)
			return
		}
		if err := validateMidtransKeyPair(configuration); err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "MIDTRANS_KEY_ENVIRONMENT_MISMATCH", err.Error())
			return
		}
	}

	value, err := json.Marshal(stored)
	if err != nil {
		serverError(c)
		return
	}
	var row model.Setting
	err = h.DB.Where("branch_id=? AND key=?", bid, midtransSettingKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.Setting{BranchID: &bid, Key: midtransSettingKey, Value: value, IsPublic: false}
		err = h.DB.Create(&row).Error
	} else if err == nil {
		row.Value = value
		row.IsPublic = false
		err = h.DB.Save(&row).Error
	}
	if err != nil {
		serverError(c)
		return
	}
	after := midtransView(stored)
	h.audit(c, "update", "midtrans_integration", &row.ID, before, after)
	response.OK(c, after)
}

func (h *Handler) loadMidtransSettings(branchID uuid.UUID) (midtransStoredSettings, error) {
	var row model.Setting
	if err := h.DB.Where("branch_id=? AND key=?", branchID, midtransSettingKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return midtransStoredSettings{}, errMidtransNotConfigured
		}
		return midtransStoredSettings{}, err
	}
	var stored midtransStoredSettings
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return midtransStoredSettings{}, fmt.Errorf("decode Midtrans settings: %w", err)
	}
	if stored.Environment == "" {
		stored.Environment = "sandbox"
	}
	return stored, nil
}

func midtransView(stored midtransStoredSettings) midtransSettingsView {
	environment := stored.Environment
	if environment == "" {
		environment = "sandbox"
	}
	return midtransSettingsView{
		Enabled: stored.Enabled, Environment: environment, MerchantID: stored.MerchantID,
		ServerKeyConfigured: stored.ServerKeyCiphertext != "", ClientKeyConfigured: stored.ClientKeyCiphertext != "",
	}
}

func (h *Handler) midtransCredentials(branchID uuid.UUID) (midtransRuntimeConfig, error) {
	stored, err := h.loadMidtransSettings(branchID)
	if err != nil {
		return midtransRuntimeConfig{}, err
	}
	if !stored.Enabled {
		return midtransRuntimeConfig{}, errMidtransDisabled
	}
	if stored.ServerKeyCiphertext == "" || stored.ClientKeyCiphertext == "" {
		return midtransRuntimeConfig{}, errMidtransNotConfigured
	}
	configuration, err := h.runtimeMidtransConfig(stored, branchID)
	if err != nil {
		return midtransRuntimeConfig{}, err
	}
	if err := validateMidtransKeyPair(configuration); err != nil {
		return midtransRuntimeConfig{}, fmt.Errorf("%w: %v", errMidtransNotConfigured, err)
	}
	return configuration, nil
}

func (h *Handler) runtimeMidtransConfig(stored midtransStoredSettings, branchID uuid.UUID) (midtransRuntimeConfig, error) {
	serverKey, err := h.decryptMidtransSettingSecret(stored.ServerKeyCiphertext, "server_key", branchID)
	if err != nil {
		return midtransRuntimeConfig{}, errMidtransSecretInvalid
	}
	clientKey, err := h.decryptMidtransSettingSecret(stored.ClientKeyCiphertext, "client_key", branchID)
	if err != nil {
		return midtransRuntimeConfig{}, errMidtransSecretInvalid
	}
	return midtransRuntimeConfig{
		Environment: stored.Environment, MerchantID: stored.MerchantID,
		ServerKey: serverKey, ClientKey: clientKey,
	}, nil
}

func (h *Handler) midtransCredentialsForPayment(payment model.Payment) (midtransRuntimeConfig, error) {
	if payment.Metadata != nil {
		if snapshot, ok := payment.Metadata[midtransCredentialsSnapshotMetadataKey].(string); ok && snapshot != "" {
			return h.decryptMidtransPaymentSnapshot(snapshot, payment.BranchID, payment.ID)
		}
	}
	return h.midtransCredentials(payment.BranchID)
}

func (h *Handler) encryptMidtransPaymentSnapshot(configuration midtransRuntimeConfig, branchID, paymentID uuid.UUID) (string, error) {
	payload, err := json.Marshal(configuration)
	if err != nil {
		return "", err
	}
	return h.encryptMidtransSecret(payload, "payment-snapshot", midtransSnapshotAAD(branchID, paymentID))
}

func (h *Handler) decryptMidtransPaymentSnapshot(value string, branchID, paymentID uuid.UUID) (midtransRuntimeConfig, error) {
	payload, err := h.decryptMidtransSecret(value, "payment-snapshot", midtransSnapshotAAD(branchID, paymentID))
	if err != nil {
		return midtransRuntimeConfig{}, errMidtransSecretInvalid
	}
	var configuration midtransRuntimeConfig
	if json.Unmarshal(payload, &configuration) != nil || validateMidtransKeyPair(configuration) != nil {
		return midtransRuntimeConfig{}, errMidtransSecretInvalid
	}
	return configuration, nil
}

func midtransSnapshotAAD(branchID, paymentID uuid.UUID) string {
	return midtransCredentialsSnapshotMetadataKey + ":" + branchID.String() + ":" + paymentID.String()
}

func (h *Handler) encryptMidtransSettingSecret(value, field string, branchID uuid.UUID) (string, error) {
	return h.encryptMidtransSecret([]byte(value), "setting", midtransSettingKey+":"+field+":"+branchID.String())
}

func (h *Handler) decryptMidtransSettingSecret(value, field string, branchID uuid.UUID) (string, error) {
	plaintext, err := h.decryptMidtransSecret(value, "setting", midtransSettingKey+":"+field+":"+branchID.String())
	if err != nil {
		return "", errMidtransSecretInvalid
	}
	return string(plaintext), nil
}

func (h *Handler) encryptMidtransSecret(value []byte, purpose, aad string) (string, error) {
	gcm, err := h.midtransCipher(purpose)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, value, []byte(aad))
	payload := append(append(make([]byte, 0, len(nonce)+len(ciphertext)), nonce...), ciphertext...)
	return "v1:" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (h *Handler) decryptMidtransSecret(value, purpose, aad string) ([]byte, error) {
	if !strings.HasPrefix(value, "v1:") {
		return nil, errMidtransSecretInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return nil, errMidtransSecretInvalid
	}
	gcm, err := h.midtransCipher(purpose)
	if err != nil || len(payload) < gcm.NonceSize() {
		return nil, errMidtransSecretInvalid
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return nil, errMidtransSecretInvalid
	}
	return plaintext, nil
}

func (h *Handler) midtransCipher(purpose string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("BengkelOS/midtrans-" + purpose + "/v1\x00" + h.Config.RefreshSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func validateMidtransSecret(label, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s tidak boleh mengandung baris baru", label)
	}
	return nil
}

func validateMidtransKeyPair(configuration midtransRuntimeConfig) error {
	serverKey := strings.TrimSpace(configuration.ServerKey)
	clientKey := strings.TrimSpace(configuration.ClientKey)
	if serverKey == "" || clientKey == "" {
		return errors.New("Server Key dan Client Key Midtrans belum lengkap")
	}
	switch configuration.Environment {
	case "sandbox":
		if !strings.HasPrefix(serverKey, "SB-Mid-server-") || !strings.HasPrefix(clientKey, "SB-Mid-client-") {
			return errors.New("mode sandbox wajib memakai Server Key SB-Mid-server-… dan Client Key SB-Mid-client-…")
		}
	case "production":
		if !strings.HasPrefix(serverKey, "Mid-server-") || !strings.HasPrefix(clientKey, "Mid-client-") {
			return errors.New("mode production wajib memakai Server Key Mid-server-… dan Client Key Mid-client-…")
		}
	default:
		return errors.New("environment Midtrans harus sandbox atau production")
	}
	return nil
}

func (h *Handler) respondMidtransConfigurationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errMidtransDisabled):
		response.Error(c, http.StatusServiceUnavailable, "MIDTRANS_DISABLED", "Integrasi Midtrans belum diaktifkan pada pengaturan cabang")
	case errors.Is(err, errMidtransNotConfigured):
		response.Error(c, http.StatusServiceUnavailable, "MIDTRANS_NOT_CONFIGURED", "Server Key, Client Key, atau mode Midtrans belum dikonfigurasi pada pengaturan cabang")
	case errors.Is(err, errMidtransSecretInvalid):
		response.Error(c, http.StatusServiceUnavailable, "MIDTRANS_KEY_UNREADABLE", "Key Midtrans tidak dapat dibaca. Simpan ulang key pada pengaturan")
	default:
		serverError(c)
	}
}

func (h *Handler) redactMidtransSetting(row *model.Setting) {
	if row.Key != midtransSettingKey {
		return
	}
	var stored midtransStoredSettings
	if json.Unmarshal(row.Value, &stored) != nil {
		row.Value = json.RawMessage(`{"enabled":false,"environment":"sandbox","merchant_id":"","server_key_configured":false,"client_key_configured":false}`)
		return
	}
	row.Value, _ = json.Marshal(midtransView(stored))
}

func redactMidtransPaymentMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	redacted := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if key != midtransCredentialsSnapshotMetadataKey && key != "snap_token" && key != "redirect_url" {
			redacted[key] = value
		}
	}
	return redacted
}

func redactedPayment(payment model.Payment) model.Payment {
	payment.Metadata = redactMidtransPaymentMetadata(payment.Metadata)
	return payment
}
