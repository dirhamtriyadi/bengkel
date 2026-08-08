package handler

import (
	"strings"
	"testing"

	"bengkel/internal/paymentgateway"

	"github.com/google/uuid"
)

func TestReplacementMidtransOrderIDIsUniqueAndWithinProviderLimit(t *testing.T) {
	firstID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	secondID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	invoice := strings.Repeat("INV-PANJANG-", 8)
	first := replacementMidtransOrderID(invoice, firstID)
	second := replacementMidtransOrderID(invoice, secondID)
	if len([]rune(first)) > 50 || len([]rune(second)) > 50 {
		t.Fatalf("replacement order id exceeds Midtrans limit: %q %q", first, second)
	}
	if first == second {
		t.Fatal("different payment attempts must have different Midtrans order IDs")
	}
	if !strings.Contains(first, "-R-") {
		t.Fatalf("replacement marker is missing: %s", first)
	}
}

func TestMidtransTransactionSuccessfulOnlyAcceptsSettledFunds(t *testing.T) {
	if !midtransTransactionSuccessful(&paymentgateway.TransactionStatus{TransactionStatus: "settlement"}) {
		t.Fatal("settlement must be treated as successful")
	}
	if !midtransTransactionSuccessful(&paymentgateway.TransactionStatus{TransactionStatus: "capture", FraudStatus: "accept"}) {
		t.Fatal("accepted capture must be treated as successful")
	}
	if midtransTransactionSuccessful(&paymentgateway.TransactionStatus{TransactionStatus: "capture", FraudStatus: "challenge"}) {
		t.Fatal("challenged capture must not be treated as successful")
	}
	if midtransTransactionSuccessful(&paymentgateway.TransactionStatus{TransactionStatus: "pending"}) {
		t.Fatal("pending transaction must not be treated as successful")
	}
}
