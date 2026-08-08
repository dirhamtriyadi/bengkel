package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"bengkel/internal/config"
	"bengkel/internal/model"

	"github.com/google/uuid"
)

func TestMidtransSettingEncryptionIsBranchAndFieldBound(t *testing.T) {
	handler := Handler{Config: config.Config{RefreshSecret: strings.Repeat("r", 48)}}
	branchID := uuid.New()
	ciphertext, err := handler.encryptMidtransSettingSecret("SB-Mid-server-secret", "server_key", branchID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "server-secret") {
		t.Fatal("ciphertext leaks plaintext")
	}
	plaintext, err := handler.decryptMidtransSettingSecret(ciphertext, "server_key", branchID)
	if err != nil || plaintext != "SB-Mid-server-secret" {
		t.Fatalf("unexpected decrypted value: %q %v", plaintext, err)
	}
	if _, err := handler.decryptMidtransSettingSecret(ciphertext, "client_key", branchID); err == nil {
		t.Fatal("server ciphertext must not decrypt as a client key")
	}
	if _, err := handler.decryptMidtransSettingSecret(ciphertext, "server_key", uuid.New()); err == nil {
		t.Fatal("ciphertext must not decrypt for another branch")
	}
}

func TestMidtransPaymentSnapshotIsPaymentBound(t *testing.T) {
	handler := Handler{Config: config.Config{RefreshSecret: strings.Repeat("r", 48)}}
	branchID, paymentID := uuid.New(), uuid.New()
	want := midtransRuntimeConfig{
		Environment: "sandbox", MerchantID: "G123", ServerKey: "SB-Mid-server-secret", ClientKey: "SB-Mid-client-secret",
	}
	snapshot, err := handler.encryptMidtransPaymentSnapshot(want, branchID, paymentID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := handler.decryptMidtransPaymentSnapshot(snapshot, branchID, paymentID)
	if err != nil || got != want {
		t.Fatalf("unexpected snapshot: %#v %v", got, err)
	}
	if _, err := handler.decryptMidtransPaymentSnapshot(snapshot, branchID, uuid.New()); err == nil {
		t.Fatal("snapshot must not decrypt for another payment")
	}
}

func TestValidateMidtransKeyPairMatchesEnvironment(t *testing.T) {
	valid := []midtransRuntimeConfig{
		{Environment: "sandbox", ServerKey: "SB-Mid-server-value", ClientKey: "SB-Mid-client-value"},
		{Environment: "production", ServerKey: "Mid-server-value", ClientKey: "Mid-client-value"},
	}
	for _, configuration := range valid {
		if err := validateMidtransKeyPair(configuration); err != nil {
			t.Errorf("valid configuration rejected: %v", err)
		}
	}
	invalid := valid[0]
	invalid.Environment = "production"
	if err := validateMidtransKeyPair(invalid); err == nil {
		t.Fatal("sandbox keys must be rejected in production")
	}
}

func TestRedactMidtransSettingAndPaymentSnapshot(t *testing.T) {
	handler := Handler{}
	row := model.Setting{
		Key:   midtransSettingKey,
		Value: json.RawMessage(`{"enabled":true,"environment":"sandbox","merchant_id":"G123","server_key_ciphertext":"v1:server-secret","client_key_ciphertext":"v1:client-secret"}`),
	}
	handler.redactMidtransSetting(&row)
	serialized := string(row.Value)
	if strings.Contains(serialized, "ciphertext") || strings.Contains(serialized, "v1:") {
		t.Fatalf("redacted setting leaks ciphertext: %s", serialized)
	}
	if !strings.Contains(serialized, `"server_key_configured":true`) || !strings.Contains(serialized, `"client_key_configured":true`) {
		t.Fatalf("redacted setting loses configured status: %s", serialized)
	}

	metadata := redactMidtransPaymentMetadata(map[string]any{
		midtransCredentialsSnapshotMetadataKey: "v1:secret", "transaction_id": "transaction-1",
	})
	if _, exists := metadata[midtransCredentialsSnapshotMetadataKey]; exists {
		t.Fatal("payment metadata leaks credential snapshot")
	}
	if metadata["transaction_id"] != "transaction-1" {
		t.Fatal("non-secret payment metadata should be retained")
	}
}
