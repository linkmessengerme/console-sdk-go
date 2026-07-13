package idempotency

import "testing"

func TestNewKeyIsValidForGateway(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatalf("NewKey returned error: %v", err)
	}
	if err := Validate(key); err != nil {
		t.Fatalf("generated key is invalid: %v", err)
	}
}

func TestValidateRejectsSpaces(t *testing.T) {
	if err := Validate("bad key"); err == nil {
		t.Fatal("expected validation error")
	}
}
