package paymentgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
	"github.com/midtrans/midtrans-go/snap"
)

type Midtrans struct {
	serverKey   string
	environment midtrans.EnvironmentType
}

type SnapInput struct {
	IdempotencyKey string
	OrderID        string
	Amount         int64
	ItemID         string
	ItemName       string
	CustomerName   string
	CustomerEmail  string
	CustomerPhone  string
	FinishURL      string
}

type SnapResult struct {
	Token       string
	RedirectURL string
}

type TransactionStatus struct {
	OrderID           string
	GrossAmount       string
	TransactionID     string
	TransactionStatus string
	FraudStatus       string
}

func NewMidtrans(serverKey string, production bool) Midtrans {
	environment := midtrans.Sandbox
	if production {
		environment = midtrans.Production
	}
	return Midtrans{serverKey: strings.TrimSpace(serverKey), environment: environment}
}

func (gateway Midtrans) CreateSnap(ctx context.Context, input SnapInput) (SnapResult, error) {
	client := snap.Client{}
	client.New(gateway.serverKey, gateway.environment)
	client.HttpClient = secureHTTPClient()
	client.Options.SetContext(ctx)
	if input.IdempotencyKey != "" {
		client.Options.SetPaymentIdempotencyKey(input.IdempotencyKey)
	}

	request := BuildSnapRequest(input)
	result, midtransError := client.CreateTransaction(request)
	if midtransError != nil {
		return SnapResult{}, providerError(midtransError)
	}
	if result == nil || result.Token == "" {
		message := "Midtrans tidak mengembalikan token transaksi"
		if result != nil && len(result.ErrorMessages) > 0 {
			message = strings.Join(result.ErrorMessages, "; ")
		}
		return SnapResult{}, &Error{StatusCode: 502, Message: message}
	}
	return SnapResult{Token: result.Token, RedirectURL: result.RedirectURL}, nil
}

func (gateway Midtrans) CheckTransaction(ctx context.Context, orderID string) (*TransactionStatus, error) {
	client := coreapi.Client{}
	client.New(gateway.serverKey, gateway.environment)
	client.HttpClient = secureHTTPClient()
	client.Options.SetContext(ctx)
	result, midtransError := client.CheckTransaction(orderID)
	if midtransError != nil {
		return nil, providerError(midtransError)
	}
	if result == nil {
		return nil, nil
	}
	return &TransactionStatus{
		OrderID:           result.OrderID,
		GrossAmount:       result.GrossAmount,
		TransactionID:     result.TransactionID,
		TransactionStatus: result.TransactionStatus,
		FraudStatus:       result.FraudStatus,
	}, nil
}

func secureHTTPClient() midtrans.HttpClient {
	return &midtrans.HttpClientImplementation{
		HttpClient: &http.Client{Timeout: 15 * time.Second},
		Logger:     &midtrans.LoggerImplementation{LogLevel: midtrans.NoLogging},
	}
}

func BuildSnapRequest(input SnapInput) *snap.Request {
	itemName := truncate(input.ItemName, 50)
	if itemName == "" {
		itemName = "Pembayaran bengkel"
	}
	items := []midtrans.ItemDetails{{
		ID:    truncate(input.ItemID, 50),
		Name:  itemName,
		Price: input.Amount,
		Qty:   1,
	}}
	var customer *midtrans.CustomerDetails
	if input.CustomerName != "" || input.CustomerEmail != "" || input.CustomerPhone != "" {
		customer = &midtrans.CustomerDetails{
			FName: truncate(input.CustomerName, 255),
			Email: truncate(input.CustomerEmail, 255),
			Phone: truncate(input.CustomerPhone, 255),
		}
	}
	return &snap.Request{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  input.OrderID,
			GrossAmt: input.Amount,
		},
		Items:          &items,
		CustomerDetail: customer,
		CreditCard:     &snap.CreditCardDetails{Secure: true},
		Callbacks:      &snap.Callbacks{Finish: input.FinishURL},
	}
}

type Error struct {
	StatusCode int
	Message    string
}

func (err *Error) Error() string {
	return err.Message
}

func providerError(err *midtrans.Error) error {
	message := strings.TrimSpace(err.GetMessage())
	if raw := err.GetRawApiResponse(); raw != nil && len(raw.RawBody) > 0 {
		var payload struct {
			StatusMessage string   `json:"status_message"`
			ErrorMessages []string `json:"error_messages"`
		}
		if json.Unmarshal(raw.RawBody, &payload) == nil {
			if len(payload.ErrorMessages) > 0 {
				message = strings.Join(payload.ErrorMessages, "; ")
			} else if payload.StatusMessage != "" {
				message = payload.StatusMessage
			}
		}
	}
	if message == "" {
		message = "Midtrans tidak dapat memproses transaksi"
	}
	return &Error{StatusCode: err.GetStatusCode(), Message: message}
}

func truncate(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximum {
		return string(runes)
	}
	return string(runes[:maximum])
}
