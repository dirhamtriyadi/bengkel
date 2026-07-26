package paymentgateway

import "testing"

func TestBuildSnapRequestKeepsGrossAmountAndItemTotalEqual(t *testing.T) {
	request := BuildSnapRequest(SnapInput{
		OrderID:      "INV-001",
		Amount:       125000,
		ItemID:       "sale-id",
		ItemName:     "Pembayaran INV-001",
		CustomerName: "Pelanggan",
		FinishURL:    "http://localhost:3000/dashboard/sales",
	})

	if request.TransactionDetails.GrossAmt != 125000 {
		t.Fatalf("unexpected gross amount: %d", request.TransactionDetails.GrossAmt)
	}
	if request.Items == nil || len(*request.Items) != 1 {
		t.Fatal("expected exactly one summarized item")
	}
	item := (*request.Items)[0]
	if item.Price*int64(item.Qty) != request.TransactionDetails.GrossAmt {
		t.Fatalf("item total %d does not match gross amount %d", item.Price*int64(item.Qty), request.TransactionDetails.GrossAmt)
	}
	if request.CreditCard == nil || !request.CreditCard.Secure {
		t.Fatal("3DS secure payment must be enabled")
	}
}

func TestBuildSnapRequestTruncatesMidtransItemName(t *testing.T) {
	request := BuildSnapRequest(SnapInput{
		OrderID:  "INV-002",
		Amount:   1000,
		ItemID:   "sale-id",
		ItemName: "Pembayaran dengan nama yang sengaja dibuat sangat panjang untuk batas Midtrans",
	})

	if got := len([]rune((*request.Items)[0].Name)); got > 50 {
		t.Fatalf("item name exceeds Midtrans limit: %d", got)
	}
}

func TestBuildSnapRequestMapUsesMidtransAutomaticFeePerChannel(t *testing.T) {
	request := BuildSnapRequestMap(SnapInput{
		OrderID:         "INV-003",
		Amount:          100000,
		ItemID:          "sale-id",
		ItemName:        "Pembayaran INV-003",
		EnabledPayments: []string{"qris", "bank_transfer"},
		AutomaticFee:    true,
		PaymentFeeConfig: []PaymentFeeConfig{
			{PaymentType: "qris", Acquirer: "gopay", CustomerPercentage: 100},
			{PaymentType: "bank_transfer", CustomerPercentage: 50},
		},
	})

	transaction := (*request)["transaction_details"].(map[string]any)
	if transaction["gross_amount"] != int64(100000) {
		t.Fatalf("gross amount must remain the original invoice amount: %v", transaction["gross_amount"])
	}
	imposition := (*request)["customer_imposed_payment_fee"].(map[string]any)
	if imposition["enable"] != true {
		t.Fatal("automatic fee imposition must be enabled")
	}
	configs := imposition["payment_fee_configs"].([]map[string]any)
	if len(configs) != 2 || configs[0]["payment_type"] != "qris" || configs[0]["customer_percentage"] != float64(100) {
		t.Fatalf("unexpected payment fee configs: %#v", configs)
	}
	if configs[0]["acquirer"] != "gopay" {
		t.Fatalf("unexpected QRIS acquirer: %#v", configs[0])
	}
}
