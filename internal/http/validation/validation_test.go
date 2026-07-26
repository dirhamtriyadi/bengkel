package validation

import "testing"

type sample struct {
	Email string `json:"email" validate:"required,email"`
	Type  string `json:"type" validate:"oneof=part service"`
}

func TestStructUsesJSONFieldNames(t *testing.T) {
	errors := Struct(sample{Email: "wrong", Type: "unknown"})
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if errors[0].Field != "email" {
		t.Fatalf("expected json field name, got %s", errors[0].Field)
	}
}

func TestStructValid(t *testing.T) {
	if errors := Struct(sample{Email: "owner@example.com", Type: "service"}); len(errors) != 0 {
		t.Fatalf("expected no errors, got %#v", errors)
	}
}
