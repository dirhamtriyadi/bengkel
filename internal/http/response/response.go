package response

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type Meta struct {
	RequestID string `json:"request_id"`
	Page      int    `json:"page,omitempty"`
	PerPage   int    `json:"per_page,omitempty"`
	Total     int64  `json:"total,omitempty"`
	LastPage  int    `json:"last_page,omitempty"`
}
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}
type APIError struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}
type Envelope struct {
	Data  any       `json:"data,omitempty"`
	Meta  *Meta     `json:"meta,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Data: data, Meta: &Meta{RequestID: requestID(c)}})
}
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Data: data, Meta: &Meta{RequestID: requestID(c)}})
}
func Paginated(c *gin.Context, data any, page, perPage int, total int64) {
	last := 0
	if perPage > 0 {
		last = int((total + int64(perPage) - 1) / int64(perPage))
	}
	c.JSON(http.StatusOK, Envelope{Data: data, Meta: &Meta{RequestID: requestID(c), Page: page, PerPage: perPage, Total: total, LastPage: last}})
}
func Error(c *gin.Context, status int, code, message string, details ...FieldError) {
	c.AbortWithStatusJSON(status, Envelope{Meta: &Meta{RequestID: requestID(c)}, Error: &APIError{Code: code, Message: message, Details: details}})
}
func requestID(c *gin.Context) string {
	value, _ := c.Get("request_id")
	id, _ := value.(string)
	return id
}
