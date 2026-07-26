package validation

import (
	"bengkel/internal/http/response"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"reflect"
	"strings"
)

var validate = validator.New()

func init() {
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return field.Name
		}
		return name
	})
}
func Struct(value any) []response.FieldError {
	err := validate.Struct(value)
	if err == nil {
		return nil
	}
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return []response.FieldError{{Field: "body", Rule: "invalid", Message: "Payload tidak valid"}}
	}
	out := make([]response.FieldError, 0, len(validationErrors))
	for _, item := range validationErrors {
		out = append(out, response.FieldError{Field: item.Field(), Rule: item.Tag(), Message: message(item)})
	}
	return out
}
func message(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "Wajib diisi"
	case "email":
		return "Format email tidak valid"
	case "min":
		return fmt.Sprintf("Minimal %s karakter/nilai", err.Param())
	case "max":
		return fmt.Sprintf("Maksimal %s karakter/nilai", err.Param())
	case "oneof":
		return "Nilai harus salah satu dari: " + err.Param()
	case "uuid":
		return "Format ID tidak valid"
	case "gt":
		return "Nilai harus lebih besar dari " + err.Param()
	default:
		return "Nilai tidak valid"
	}
}
