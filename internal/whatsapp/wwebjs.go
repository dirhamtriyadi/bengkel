package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBody = 2 << 20

type Error struct {
	StatusCode int
	Message    string
}

func (err *Error) Error() string {
	return err.Message
}

type SendResult struct {
	MessageID string
}

type SessionStatus struct {
	Connected bool   `json:"connected"`
	State     string `json:"state"`
	Message   string `json:"message"`
}

type Client struct {
	baseURL   string
	apiKey    string
	sessionID string
	http      *http.Client
}

func New(baseURL, apiKey, sessionID string, timeout time.Duration) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		sessionID: sessionID,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (client *Client) IsRegistered(ctx context.Context, number string) (bool, error) {
	var envelope apiEnvelope
	if err := client.post(ctx, "/client/isRegisteredUser/"+url.PathEscape(client.sessionID), map[string]any{
		"number": number,
	}, &envelope); err != nil {
		return false, err
	}
	var registered bool
	if err := json.Unmarshal(envelope.Result, &registered); err != nil {
		return false, errors.New("wwebjs-api returned an invalid registration result")
	}
	return registered, nil
}

func (client *Client) SendText(ctx context.Context, number, content string) (SendResult, error) {
	var envelope apiEnvelope
	if err := client.post(ctx, "/client/sendMessage/"+url.PathEscape(client.sessionID), map[string]any{
		"chatId":      number + "@c.us",
		"contentType": "string",
		"content":     content,
	}, &envelope); err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: extractMessageID(envelope.Message)}, nil
}

func (client *Client) StartSession(ctx context.Context) (string, error) {
	var envelope apiEnvelope
	if err := client.get(ctx, "/session/start/"+url.PathEscape(client.sessionID), &envelope); err != nil {
		return "", err
	}
	return rawMessage(envelope.Message), nil
}

func (client *Client) Status(ctx context.Context) (SessionStatus, error) {
	body, statusCode, _, err := client.do(ctx, http.MethodGet, "/session/status/"+url.PathEscape(client.sessionID), nil)
	if err != nil {
		return SessionStatus{}, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return SessionStatus{}, responseError(body, statusCode)
	}
	var result struct {
		Success bool    `json:"success"`
		State   *string `json:"state"`
		Message string  `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SessionStatus{}, &Error{StatusCode: statusCode, Message: "wwebjs-api returned a non-JSON response"}
	}
	state := ""
	if result.State != nil {
		state = *result.State
	}
	return SessionStatus{Connected: result.Success && state == "CONNECTED", State: state, Message: result.Message}, nil
}

func (client *Client) QRCodeImage(ctx context.Context) ([]byte, error) {
	body, statusCode, contentType, err := client.do(ctx, http.MethodGet, "/session/qr/"+url.PathEscape(client.sessionID)+"/image", nil)
	if err != nil {
		return nil, err
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices && strings.HasPrefix(contentType, "image/png") && http.DetectContentType(body) == "image/png" {
		return body, nil
	}
	return nil, responseError(body, statusCode)
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Message json.RawMessage `json:"message"`
	Error   json.RawMessage `json:"error"`
}

func (client *Client) post(ctx context.Context, path string, payload any, destination *apiEnvelope) error {
	responseBody, statusCode, _, err := client.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return &Error{StatusCode: statusCode, Message: "wwebjs-api returned a non-JSON response"}
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices || !destination.Success {
		return envelopeError(destination, statusCode)
	}
	return nil
}

func (client *Client) get(ctx context.Context, path string, destination *apiEnvelope) error {
	responseBody, statusCode, _, err := client.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return &Error{StatusCode: statusCode, Message: "wwebjs-api returned a non-JSON response"}
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices || !destination.Success {
		return envelopeError(destination, statusCode)
	}
	return nil
}

func (client *Client) do(ctx context.Context, method, path string, payload any) ([]byte, int, string, error) {
	var requestBody io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, "", fmt.Errorf("encode wwebjs-api request: %w", err)
		}
		requestBody = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, requestBody)
	if err != nil {
		return nil, 0, "", fmt.Errorf("create wwebjs-api request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("User-Agent", "BengkelOS/1.0")
	if client.apiKey != "" {
		request.Header.Set("x-api-key", client.apiKey)
	}

	response, err := client.http.Do(request)
	if err != nil {
		return nil, 0, "", fmt.Errorf("call wwebjs-api: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if err != nil {
		return nil, response.StatusCode, response.Header.Get("Content-Type"), fmt.Errorf("read wwebjs-api response: %w", err)
	}
	return responseBody, response.StatusCode, response.Header.Get("Content-Type"), nil
}

func responseError(body []byte, statusCode int) error {
	var envelope apiEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return &Error{StatusCode: statusCode, Message: "wwebjs-api returned an invalid response"}
	}
	return envelopeError(&envelope, statusCode)
}

func envelopeError(envelope *apiEnvelope, statusCode int) error {
	message := rawMessage(envelope.Error)
	if message == "" {
		message = rawMessage(envelope.Message)
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return &Error{StatusCode: statusCode, Message: message}
}

func rawMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"message", "error"} {
			if text, ok := object[key].(string); ok {
				return text
			}
		}
	}
	return ""
}

func extractMessageID(raw json.RawMessage) string {
	var message struct {
		ID struct {
			Serialized string `json:"_serialized"`
		} `json:"id"`
		Data struct {
			ID struct {
				Serialized string `json:"_serialized"`
			} `json:"id"`
		} `json:"_data"`
	}
	if json.Unmarshal(raw, &message) != nil {
		return ""
	}
	if message.ID.Serialized != "" {
		return message.ID.Serialized
	}
	return message.Data.ID.Serialized
}

func NormalizeNumber(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("phone number is empty")
	}
	hadInternationalPrefix := strings.HasPrefix(value, "+") || strings.HasPrefix(value, "00")
	var digits strings.Builder
	for index, char := range value {
		switch {
		case char >= '0' && char <= '9':
			digits.WriteRune(char)
		case char == '+' && index == 0:
		case char == ' ' || char == '-' || char == '(' || char == ')' || char == '.':
		default:
			return "", errors.New("phone number contains invalid characters")
		}
	}
	number := digits.String()
	if strings.HasPrefix(number, "00") {
		number = strings.TrimPrefix(number, "00")
	}
	if !hadInternationalPrefix {
		switch {
		case strings.HasPrefix(number, "0"):
			number = "62" + strings.TrimPrefix(number, "0")
		case strings.HasPrefix(number, "8"):
			number = "62" + number
		}
	}
	if len(number) < 8 || len(number) > 15 || strings.HasPrefix(number, "0") {
		return "", errors.New("phone number is not a valid international number")
	}
	return number, nil
}
