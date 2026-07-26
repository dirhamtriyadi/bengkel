package handler

import (
	"encoding/json"
	"testing"

	"bengkel/internal/paymentgateway"
)

func TestEstimateMidtransProviderFeeUsesMixedRateAndTax(t *testing.T) {
	channel := midtransChannelSettings{
		FeePercentage: 2.9,
		FixedFee:      2000,
		TaxPercentage: 11,
	}
	if got, want := estimateMidtransProviderFee(100000, channel), int64(5439); got != want {
		t.Fatalf("provider fee = %d, want %d", got, want)
	}
}

func TestDeriveMidtransProviderFeeFromActualCustomerFee(t *testing.T) {
	channel := midtransChannelSettings{CustomerPercentage: 50}
	if got, want := deriveMidtransProviderFee(100000, 353, channel), int64(706); got != want {
		t.Fatalf("provider fee = %d, want %d", got, want)
	}
}

func TestParseMidtransFeeSettingsRejectsDuplicateAndInvalidPercentage(t *testing.T) {
	_, err := parseMidtransFeeSettings(json.RawMessage(`{
		"automatic_fee":true,
		"channels":[
			{"payment_type":"qris","label":"QRIS","enabled":true,"customer_percentage":101},
			{"payment_type":"qris","label":"QRIS 2","enabled":true,"customer_percentage":100}
		]
	}`))
	if err == nil {
		t.Fatal("invalid channel settings must be rejected")
	}
}

func TestMidtransFeeBearerIsMerchantWhenAutomaticFeeDisabled(t *testing.T) {
	settings := defaultMidtransFeeSettings()
	settings.AutomaticFee = false
	if got := settings.feeBearer(); got != "merchant" {
		t.Fatalf("fee bearer = %s, want merchant", got)
	}
}

func TestMidtransChannelKeyMapsProviderDetails(t *testing.T) {
	if got := midtransChannelKey(&paymentgateway.TransactionStatus{PaymentType: "bank_transfer", Bank: "bca"}); got != "bca_va" {
		t.Fatalf("channel key = %s, want bca_va", got)
	}
	if got := midtransChannelKey(&paymentgateway.TransactionStatus{PaymentType: "cstore", Store: "alfamart"}); got != "alfamart" {
		t.Fatalf("channel key = %s, want alfamart", got)
	}
}
