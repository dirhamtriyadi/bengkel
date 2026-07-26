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
