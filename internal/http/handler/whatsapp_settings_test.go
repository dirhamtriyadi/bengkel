package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"bengkel/internal/config"
	"bengkel/internal/model"

	"github.com/google/uuid"
)

func TestWhatsAppTokenEncryptionIsBranchBound(t *testing.T) {
	handler := Handler{Config: config.Config{RefreshSecret: strings.Repeat("r", 48)}}
	branchID := uuid.New()
	ciphertext, err := handler.encryptWhatsAppToken("api-secret-value", branchID)
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "api-secret-value" || strings.Contains(ciphertext, "api-secret-value") {
		t.Fatal("ciphertext leaks plaintext")
	}
	plaintext, err := handler.decryptWhatsAppToken(ciphertext, branchID)
	if err != nil || plaintext != "api-secret-value" {
		t.Fatalf("unexpected decrypted value: %q %v", plaintext, err)
	}
	if _, err := handler.decryptWhatsAppToken(ciphertext, uuid.New()); err == nil {
		t.Fatal("ciphertext must not decrypt for another branch")
	}
	replacement := "A"
	if strings.HasSuffix(ciphertext, replacement) {
		replacement = "B"
	}
	tampered := ciphertext[:len(ciphertext)-1] + replacement
	if _, err := handler.decryptWhatsAppToken(tampered, branchID); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}

func TestRedactWhatsAppSettingNeverReturnsSecret(t *testing.T) {
	handler := Handler{}
	row := model.Setting{
		Key:   whatsAppSettingKey,
		Value: json.RawMessage(`{"enabled":true,"base_url":"https://wa.example.com","session_id":"628123456789","api_token_ciphertext":"v1:secret-ciphertext"}`),
	}
	handler.redactWhatsAppSetting(&row)

	serialized := string(row.Value)
	if strings.Contains(serialized, "secret-ciphertext") || strings.Contains(serialized, "api_token_ciphertext") {
		t.Fatalf("redacted settings leak token data: %s", serialized)
	}
	if !strings.Contains(serialized, `"api_token_configured":true`) {
		t.Fatalf("redacted settings should retain configured state: %s", serialized)
	}
}

func TestNormalizeWhatsAppBaseURL(t *testing.T) {
	valid, err := normalizeWhatsAppBaseURL(" https://wa.example.com/api/ ")
	if err != nil || valid != "https://wa.example.com/api" {
		t.Fatalf("unexpected normalized URL: %q %v", valid, err)
	}
	for _, invalid := range []string{"wa.example.com", "file:///tmp/socket", "https://user:pass@wa.example.com", "https://wa.example.com?token=secret"} {
		if _, err := normalizeWhatsAppBaseURL(invalid); err == nil {
			t.Errorf("URL %q should be rejected", invalid)
		}
	}
}
