package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestClientChecksRegistrationAndSendsText(t *testing.T) {
	t.Helper()
	requests := 0
	client := New("https://wwebjs.example", "secret", "bengkel-main", time.Second)
	client.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("x-api-key") != "secret" {
			t.Fatalf("missing API key")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch request.URL.Path {
		case "/client/isRegisteredUser/bengkel-main":
			if body["number"] != "628123456789" {
				t.Fatalf("unexpected number: %v", body["number"])
			}
			return jsonResponse(http.StatusOK, `{"success":true,"result":true}`), nil
		case "/client/sendMessage/bengkel-main":
			if body["chatId"] != "628123456789@c.us" || body["contentType"] != "string" || body["content"] != "Invoice Anda" {
				t.Fatalf("unexpected send payload: %#v", body)
			}
			return jsonResponse(http.StatusOK, `{"success":true,"message":{"id":{"_serialized":"message-123"}}}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"success":false,"error":"not_found"}`), nil
		}
	})
	registered, err := client.IsRegistered(context.Background(), "628123456789")
	if err != nil || !registered {
		t.Fatalf("registration check failed: registered=%v err=%v", registered, err)
	}
	result, err := client.SendText(context.Background(), "628123456789", "Invoice Anda")
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID != "message-123" {
		t.Fatalf("unexpected message id: %q", result.MessageID)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
}

func TestClientReturnsProviderError(t *testing.T) {
	client := New("https://wwebjs.example", "secret", "missing", time.Second)
	client.http.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"success":false,"error":"session_not_found"}`), nil
	})
	_, err := client.IsRegistered(context.Background(), "628123456789")
	providerError, ok := err.(*Error)
	if !ok || providerError.StatusCode != http.StatusNotFound || providerError.Message != "session_not_found" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestClientManagesSessionAndQRCode(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("test-image")...)
	client := New("https://wwebjs.example", "secret", "628123456789", time.Second)
	client.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.Header.Get("x-api-key") != "secret" {
			t.Fatalf("unexpected session request: %s %s", request.Method, request.URL.Path)
		}
		switch request.URL.Path {
		case "/session/start/628123456789":
			return jsonResponse(http.StatusOK, `{"success":true,"message":"Session initiated successfully"}`), nil
		case "/session/status/628123456789":
			return jsonResponse(http.StatusOK, `{"success":true,"state":"CONNECTED","message":"session_connected"}`), nil
		case "/session/qr/628123456789/image":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader(string(png)))}, nil
		default:
			return jsonResponse(http.StatusNotFound, `{"success":false,"error":"not_found"}`), nil
		}
	})

	message, err := client.StartSession(context.Background())
	if err != nil || message != "Session initiated successfully" {
		t.Fatalf("unexpected start result: %q %v", message, err)
	}
	status, err := client.Status(context.Background())
	if err != nil || !status.Connected || status.State != "CONNECTED" {
		t.Fatalf("unexpected session status: %#v %v", status, err)
	}
	image, err := client.QRCodeImage(context.Background())
	if err != nil || string(image) != string(png) {
		t.Fatalf("unexpected QR image: %q %v", image, err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNormalizeNumber(t *testing.T) {
	tests := map[string]string{
		"0812-3456-789":   "628123456789",
		"812 3456 789":    "628123456789",
		"+62 812 345 678": "62812345678",
		"0065 8123 4567":  "6581234567",
	}
	for input, expected := range tests {
		actual, err := NormalizeNumber(input)
		if err != nil || actual != expected {
			t.Errorf("NormalizeNumber(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
	for _, invalid := range []string{"", "0812abc", "123", "@c.us"} {
		if _, err := NormalizeNumber(invalid); err == nil {
			t.Errorf("NormalizeNumber(%q) should fail", invalid)
		}
	}
}
