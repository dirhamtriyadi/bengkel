package response

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorStoresCodeForRequestLogging(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("request_id", "request-1")

	Error(context, 424, "MIDTRANS_DEPENDENCY_FAILED", "Midtrans tidak tersedia")

	if code := context.GetString(ErrorCodeContextKey); code != "MIDTRANS_DEPENDENCY_FAILED" {
		t.Fatalf("unexpected context error code: %q", code)
	}
}
