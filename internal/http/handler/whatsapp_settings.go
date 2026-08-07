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
	"net/url"
	"strings"
	"time"

	"bengkel/internal/http/response"
	"bengkel/internal/model"
	"bengkel/internal/whatsapp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const whatsAppSettingKey = "integration.whatsapp.wwebjs"

var (
	errWhatsAppNotConfigured = errors.New("WhatsApp integration is not configured")
	errWhatsAppDisabled      = errors.New("WhatsApp integration is disabled")
	errWhatsAppSecretInvalid = errors.New("WhatsApp API token cannot be decrypted")
)

type whatsAppStoredSettings struct {
	Enabled            bool   `json:"enabled"`
	BaseURL            string `json:"base_url"`
	SessionID          string `json:"session_id"`
	APITokenCiphertext string `json:"api_token_ciphertext"`
}

type whatsAppSettingsView struct {
	Enabled            bool   `json:"enabled"`
	BaseURL            string `json:"base_url"`
	SessionID          string `json:"session_id"`
	APITokenConfigured bool   `json:"api_token_configured"`
}

type whatsAppSettingsInput struct {
	Enabled       bool   `json:"enabled"`
	BaseURL       string `json:"base_url" validate:"max=500"`
	APIToken      string `json:"api_token" validate:"max=2048"`
	ClearAPIToken bool   `json:"clear_api_token"`
	SessionID     string `json:"session_id" validate:"max=30"`
}

// WhatsAppIntegration godoc
// @Summary Konfigurasi wwebjs-api cabang
// @Description Token API tidak pernah dikembalikan; response hanya menunjukkan apakah token sudah tersimpan.
// @Tags WhatsApp
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /integrations/whatsapp [get]
func (h *Handler) WhatsAppIntegration(c *gin.Context) {
	stored, err := h.loadWhatsAppSettings(branchID(c))
	if errors.Is(err, errWhatsAppNotConfigured) {
		response.OK(c, whatsAppSettingsView{})
		return
	}
	if err != nil {
		serverError(c)
		return
	}
	response.OK(c, whatsAppView(stored))
}

// UpdateWhatsAppIntegration godoc
// @Summary Simpan konfigurasi wwebjs-api cabang
// @Description URL API, token API terenkripsi, dan session ID berupa nomor WhatsApp disimpan di database.
// @Tags WhatsApp
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body whatsAppSettingsInput true "Konfigurasi WhatsApp"
// @Success 200 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /integrations/whatsapp [put]
func (h *Handler) UpdateWhatsAppIntegration(c *gin.Context) {
	var input whatsAppSettingsInput
	if !bind(c, &input) {
		return
	}
	bid := branchID(c)
	stored, err := h.loadWhatsAppSettings(bid)
	if errors.Is(err, errWhatsAppNotConfigured) {
		stored = whatsAppStoredSettings{}
		err = nil
	}
	if err != nil {
		serverError(c)
		return
	}
	before := whatsAppView(stored)

	baseURL, err := normalizeWhatsAppBaseURL(input.BaseURL)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "WHATSAPP_URL_INVALID", err.Error())
		return
	}
	sessionID := ""
	if strings.TrimSpace(input.SessionID) != "" {
		sessionID, err = whatsapp.NormalizeNumber(input.SessionID)
		if err != nil {
			response.Error(c, http.StatusUnprocessableEntity, "WHATSAPP_SESSION_ID_INVALID", "Session ID harus berupa nomor WhatsApp yang valid, misalnya 62812...")
			return
		}
	}
	stored.Enabled = input.Enabled
	stored.BaseURL = baseURL
	stored.SessionID = sessionID
	if input.ClearAPIToken {
		stored.APITokenCiphertext = ""
	}
	if token := strings.TrimSpace(input.APIToken); token != "" {
		if strings.ContainsAny(token, "\r\n") {
			response.Error(c, http.StatusUnprocessableEntity, "WHATSAPP_API_TOKEN_INVALID", "Token API tidak boleh mengandung baris baru")
			return
		}
		stored.APITokenCiphertext, err = h.encryptWhatsAppToken(token, bid)
		if err != nil {
			serverError(c)
			return
		}
	}
	if stored.Enabled && (stored.BaseURL == "" || stored.SessionID == "" || stored.APITokenCiphertext == "") {
		response.Error(c, http.StatusUnprocessableEntity, "WHATSAPP_CONFIG_INCOMPLETE", "URL API, token API, dan nomor session wajib diisi sebelum integrasi diaktifkan")
		return
	}

	value, err := json.Marshal(stored)
	if err != nil {
		serverError(c)
		return
	}
	var row model.Setting
	err = h.DB.Where("branch_id=? AND key=?", bid, whatsAppSettingKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.Setting{BranchID: &bid, Key: whatsAppSettingKey, Value: value, IsPublic: false}
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
	after := whatsAppView(stored)
	h.audit(c, "update", "whatsapp_integration", &row.ID, before, after)
	response.OK(c, after)
}

// StartWhatsAppSession godoc
// @Summary Mulai session wwebjs-api
// @Tags WhatsApp
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /integrations/whatsapp/session/start [post]
func (h *Handler) StartWhatsAppSession(c *gin.Context) {
	client, settings, err := h.whatsAppClient(branchID(c))
	if err != nil {
		h.respondWhatsAppConfigurationError(c, err)
		return
	}
	message, err := client.StartSession(c.Request.Context())
	if err != nil {
		h.respondWhatsAppProviderError(c, err)
		return
	}
	h.audit(c, "start_session", "whatsapp_integration", nil, nil, map[string]any{"session_id": maskPhone(settings.SessionID)})
	response.OK(c, gin.H{"message": message, "session_id": settings.SessionID})
}

// WhatsAppSessionStatus godoc
// @Summary Status koneksi session wwebjs-api
// @Tags WhatsApp
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /integrations/whatsapp/session/status [get]
func (h *Handler) WhatsAppSessionStatus(c *gin.Context) {
	client, settings, err := h.whatsAppClient(branchID(c))
	if err != nil {
		h.respondWhatsAppConfigurationError(c, err)
		return
	}
	status, err := client.Status(c.Request.Context())
	if err != nil {
		h.respondWhatsAppProviderError(c, err)
		return
	}
	response.OK(c, gin.H{
		"connected": status.Connected, "state": status.State, "message": status.Message,
		"session_id": settings.SessionID,
	})
}

// WhatsAppSessionQR godoc
// @Summary QR pairing session wwebjs-api
// @Description Mengambil PNG QR dari service wwebjs-api terpisah dan meneruskannya sebagai data URL.
// @Tags WhatsApp
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Router /integrations/whatsapp/session/qr [get]
func (h *Handler) WhatsAppSessionQR(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	client, settings, err := h.whatsAppClient(branchID(c))
	if err != nil {
		h.respondWhatsAppConfigurationError(c, err)
		return
	}
	image, err := client.QRCodeImage(c.Request.Context())
	if err != nil {
		h.respondWhatsAppProviderError(c, err)
		return
	}
	response.OK(c, gin.H{
		"session_id": settings.SessionID,
		"image":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(image),
	})
}

func (h *Handler) whatsAppClient(branchID uuid.UUID) (*whatsapp.Client, whatsAppSettingsView, error) {
	stored, err := h.loadWhatsAppSettings(branchID)
	if err != nil {
		return nil, whatsAppSettingsView{}, err
	}
	if !stored.Enabled {
		return nil, whatsAppView(stored), errWhatsAppDisabled
	}
	if stored.BaseURL == "" || stored.SessionID == "" || stored.APITokenCiphertext == "" {
		return nil, whatsAppView(stored), errWhatsAppNotConfigured
	}
	token, err := h.decryptWhatsAppToken(stored.APITokenCiphertext, branchID)
	if err != nil {
		return nil, whatsAppView(stored), errWhatsAppSecretInvalid
	}
	return whatsapp.New(stored.BaseURL, token, stored.SessionID, 25*time.Second), whatsAppView(stored), nil
}

func (h *Handler) loadWhatsAppSettings(branchID uuid.UUID) (whatsAppStoredSettings, error) {
	var row model.Setting
	if err := h.DB.Where("branch_id=? AND key=?", branchID, whatsAppSettingKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return whatsAppStoredSettings{}, errWhatsAppNotConfigured
		}
		return whatsAppStoredSettings{}, err
	}
	var stored whatsAppStoredSettings
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return whatsAppStoredSettings{}, fmt.Errorf("decode WhatsApp settings: %w", err)
	}
	return stored, nil
}

func whatsAppView(stored whatsAppStoredSettings) whatsAppSettingsView {
	return whatsAppSettingsView{
		Enabled: stored.Enabled, BaseURL: stored.BaseURL, SessionID: stored.SessionID,
		APITokenConfigured: stored.APITokenCiphertext != "",
	}
}

func normalizeWhatsAppBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("URL wwebjs-api harus berupa URL http(s) absolut tanpa query, fragment, atau userinfo")
	}
	return value, nil
}

func (h *Handler) encryptWhatsAppToken(token string, branchID uuid.UUID) (string, error) {
	gcm, err := h.whatsAppCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(token), []byte(whatsAppSettingKey+":"+branchID.String()))
	payload := append(append(make([]byte, 0, len(nonce)+len(ciphertext)), nonce...), ciphertext...)
	return "v1:" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (h *Handler) decryptWhatsAppToken(value string, branchID uuid.UUID) (string, error) {
	if !strings.HasPrefix(value, "v1:") {
		return "", errWhatsAppSecretInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", errWhatsAppSecretInvalid
	}
	gcm, err := h.whatsAppCipher()
	if err != nil || len(payload) < gcm.NonceSize() {
		return "", errWhatsAppSecretInvalid
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(whatsAppSettingKey+":"+branchID.String()))
	if err != nil {
		return "", errWhatsAppSecretInvalid
	}
	return string(plaintext), nil
}

func (h *Handler) whatsAppCipher() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("BengkelOS/wwebjs-setting/v1\x00" + h.Config.RefreshSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (h *Handler) respondWhatsAppConfigurationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errWhatsAppDisabled):
		response.Error(c, http.StatusServiceUnavailable, "WHATSAPP_DISABLED", "Integrasi WhatsApp belum diaktifkan pada pengaturan")
	case errors.Is(err, errWhatsAppNotConfigured):
		response.Error(c, http.StatusServiceUnavailable, "WHATSAPP_NOT_CONFIGURED", "URL API, token API, atau nomor session WhatsApp belum dikonfigurasi")
	case errors.Is(err, errWhatsAppSecretInvalid):
		response.Error(c, http.StatusServiceUnavailable, "WHATSAPP_TOKEN_UNREADABLE", "Token API WhatsApp tidak dapat dibaca. Simpan ulang token pada pengaturan")
	default:
		serverError(c)
	}
}

func (h *Handler) redactWhatsAppSetting(row *model.Setting) {
	if row.Key != whatsAppSettingKey {
		return
	}
	var stored whatsAppStoredSettings
	if json.Unmarshal(row.Value, &stored) != nil {
		row.Value = json.RawMessage(`{"enabled":false,"base_url":"","session_id":"","api_token_configured":false}`)
		return
	}
	row.Value, _ = json.Marshal(whatsAppView(stored))
}
