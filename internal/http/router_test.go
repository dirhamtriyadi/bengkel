package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"bengkel/internal/config"
	"bengkel/internal/http/response"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestRouterErrorsAlwaysUseJSONEnvelope(t *testing.T) {
	router := NewRouter(config.Config{}, &gorm.DB{}, zap.NewNop())
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
	}{
		{name: "unknown route", method: stdhttp.MethodGet, path: "/api/v1/not-registered", wantStatus: stdhttp.StatusNotFound, wantCode: "ROUTE_NOT_FOUND"},
		{name: "unsupported method", method: stdhttp.MethodDelete, path: "/health", wantStatus: stdhttp.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("unexpected status: got %d want %d", recorder.Code, test.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
				t.Fatalf("unexpected content type: %q", contentType)
			}
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if envelope.Error == nil || envelope.Error.Code != test.wantCode {
				t.Fatalf("unexpected error envelope: %#v", envelope.Error)
			}
			if envelope.Meta == nil || envelope.Meta.RequestID == "" {
				t.Fatal("error envelope must include request_id")
			}
		})
	}
}
