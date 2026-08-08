package paymentgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
	"github.com/midtrans/midtrans-go/snap"
)

var ErrSnapSessionInProgress = errors.New("midtrans snap transaction is in progress")

type Midtrans struct {
	serverKey   string
	environment midtrans.EnvironmentType
}

type SnapInput struct {
	IdempotencyKey   string
	OrderID          string
	Amount           int64
	ItemID           string
	ItemName         string
	CustomerName     string
	CustomerEmail    string
	CustomerPhone    string
	FinishURL        string
	EnabledPayments  []string
	AutomaticFee     bool
	PaymentFeeConfig []PaymentFeeConfig
}

type PaymentFeeConfig struct {
	PaymentType        string
	Acquirer           string
	CustomerPercentage float64
}

type SnapResult struct {
	Token       string
	RedirectURL string
}

type TransactionStatus struct {
	OrderID                   string
	GrossAmount               string
	OriginalAmount            string
	CustomerImposedPaymentFee string
	PaymentType               string
	Bank                      string
	Store                     string
	Acquirer                  string
	TransactionID             string
	TransactionStatus         string
	FraudStatus               string
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

	request := BuildSnapRequestMap(input)
	result, midtransError := client.CreateTransactionWithMap(request)
	if midtransError != nil {
		return SnapResult{}, providerError(midtransError)
	}
	token, _ := result["token"].(string)
	redirectURL, _ := result["redirect_url"].(string)
	if token == "" {
		message := "Midtrans tidak mengembalikan token transaksi"
		if messages, ok := result["error_messages"].([]any); ok {
			values := make([]string, 0, len(messages))
			for _, value := range messages {
				values = append(values, fmt.Sprint(value))
			}
			if len(values) > 0 {
				message = strings.Join(values, "; ")
			}
		}
		return SnapResult{}, &Error{StatusCode: 502, Message: message}
	}
	return SnapResult{Token: token, RedirectURL: redirectURL}, nil
}

func (gateway Midtrans) CheckTransaction(ctx context.Context, orderID string) (*TransactionStatus, error) {
	client := secureHTTPClient()
	options := &midtrans.ConfigOptions{}
	options.SetContext(ctx)
	var result struct {
		OrderID           string `json:"order_id"`
		GrossAmount       string `json:"gross_amount"`
		PaymentType       string `json:"payment_type"`
		Bank              string `json:"bank"`
		Store             string `json:"store"`
		Acquirer          string `json:"acquirer"`
		TransactionID     string `json:"transaction_id"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
		ExtraInfo         struct {
			GrossAmountInfo struct {
				OriginalAmount            any `json:"original_amount"`
				GrossAmount               any `json:"gross_amount"`
				CustomerImposedPaymentFee any `json:"customer_imposed_payment_fee"`
			} `json:"gross_amount_info"`
		} `json:"extra_info"`
	}
	endpoint := fmt.Sprintf("%s/v2/%s/status", gateway.environment.BaseUrl(), url.PathEscape(orderID))
	midtransError := client.Call(http.MethodGet, endpoint, &gateway.serverKey, options, nil, &result)
	if midtransError != nil {
		return nil, providerError(midtransError)
	}
	return &TransactionStatus{
		OrderID:                   result.OrderID,
		GrossAmount:               result.GrossAmount,
		OriginalAmount:            amountString(result.ExtraInfo.GrossAmountInfo.OriginalAmount),
		CustomerImposedPaymentFee: amountString(result.ExtraInfo.GrossAmountInfo.CustomerImposedPaymentFee),
		PaymentType:               result.PaymentType,
		Bank:                      result.Bank,
		Store:                     result.Store,
		Acquirer:                  result.Acquirer,
		TransactionID:             result.TransactionID,
		TransactionStatus:         result.TransactionStatus,
		FraudStatus:               result.FraudStatus,
	}, nil
}

// CancelSnapSession invalidates a Snap token before the customer chooses a
// payment channel. A token that is already cancelled or unknown is safe to
// treat as inactive, which keeps this operation idempotent.
func (gateway Midtrans) CancelSnapSession(ctx context.Context, token string) error {
	endpoint := fmt.Sprintf("%s/snap/v1/transactions/%s/cancel", gateway.environment.SnapURL(), url.PathEscape(strings.TrimSpace(token)))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return &Error{StatusCode: http.StatusInternalServerError, Message: "Tidak dapat membuat permintaan pembatalan sesi Midtrans"}
	}
	request.Header.Set("Authorization", gateway.serverKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return &Error{StatusCode: http.StatusBadGateway, Message: "Midtrans tidak dapat dihubungi untuk membatalkan sesi pembayaran"}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return &Error{StatusCode: http.StatusBadGateway, Message: "Respons pembatalan sesi Midtrans tidak dapat dibaca"}
	}
	return snapSessionCancelResult(response.StatusCode, body)
}

// ExpireTransaction makes an already selected pending payment instruction no
// longer payable before the application creates its replacement attempt.
func (gateway Midtrans) ExpireTransaction(ctx context.Context, orderID string) error {
	client := coreapi.Client{}
	client.New(gateway.serverKey, gateway.environment)
	client.HttpClient = secureHTTPClient()
	client.Options.SetContext(ctx)
	if _, midtransError := client.ExpireTransaction(orderID); midtransError != nil {
		return providerError(midtransError)
	}
	return nil
}

func snapSessionCancelResult(statusCode int, body []byte) error {
	var payload struct {
		ErrorMessages []string `json:"error_messages"`
	}
	_ = json.Unmarshal(body, &payload)
	message := strings.TrimSpace(strings.Join(payload.ErrorMessages, "; "))
	normalized := strings.ToLower(message)
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	if strings.Contains(normalized, "token already canceled") || strings.Contains(normalized, "token already cancelled") || strings.Contains(normalized, "token not found") {
		return nil
	}
	if strings.Contains(normalized, "transaction is on progress") {
		return ErrSnapSessionInProgress
	}
	if message == "" {
		message = "Midtrans menolak pembatalan sesi pembayaran"
	}
	return &Error{StatusCode: statusCode, Message: message}
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

func BuildSnapRequestMap(input SnapInput) *snap.RequestParamWithMap {
	request := BuildSnapRequest(input)
	payload := snap.RequestParamWithMap{
		"transaction_details": map[string]any{
			"order_id":     request.TransactionDetails.OrderID,
			"gross_amount": request.TransactionDetails.GrossAmt,
		},
		"item_details": *request.Items,
		"credit_card": map[string]any{
			"secure": true,
		},
	}
	if request.CustomerDetail != nil {
		payload["customer_details"] = request.CustomerDetail
	}
	if request.Callbacks != nil && request.Callbacks.Finish != "" {
		payload["callbacks"] = map[string]any{"finish": request.Callbacks.Finish}
	}
	if enabledPayments := normalizeSnapEnabledPayments(input.EnabledPayments); len(enabledPayments) > 0 {
		payload["enabled_payments"] = enabledPayments
	}
	if input.AutomaticFee && len(input.PaymentFeeConfig) > 0 {
		configs := make([]map[string]any, 0, len(input.PaymentFeeConfig))
		for _, config := range input.PaymentFeeConfig {
			item := map[string]any{
				"payment_type":        config.PaymentType,
				"customer_percentage": config.CustomerPercentage,
			}
			if config.Acquirer != "" {
				item["acquirer"] = config.Acquirer
			}
			configs = append(configs, item)
		}
		payload["customer_imposed_payment_fee"] = map[string]any{
			"enable":              true,
			"payment_fee_configs": configs,
		}
	}
	return &payload
}

func normalizeSnapEnabledPayments(values []string) []string {
	payments := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		paymentType := strings.ToLower(strings.TrimSpace(value))
		// Snap uses other_qris in enabled_payments, while transaction status
		// responses and the internal reconciliation key use qris.
		if paymentType == "qris" {
			paymentType = "other_qris"
		}
		if paymentType == "" {
			continue
		}
		if _, exists := seen[paymentType]; exists {
			continue
		}
		seen[paymentType] = struct{}{}
		payments = append(payments, paymentType)
	}
	return payments
}

func amountString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return ""
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
