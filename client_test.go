package link

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkmessengerme/console-sdk-go/otp"
)

const testClientID = "lk_client_test01"

// testPrivPEM — свежий Ed25519 ключ (PKCS#8 PEM) для подписи client_assertion в тестах.
var testPrivPEM = mustGenEd25519PEM()

func mustGenEd25519PEM() string {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// newOTPTestServer обслуживает token endpoint (отдаёт фиксированный access_token) и
// продуктовый /sdk/v1/otp/send (передаётся вызывающим).
func newOTPTestServer(product http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v1/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":900,"scope":"otp"}`))
	})
	mux.HandleFunc("/sdk/v1/otp/send", product)
	return httptest.NewServer(mux)
}

func TestOTPSendNormalizesPhoneAndSendsBearer(t *testing.T) {
	server := newOTPTestServer(func(w http.ResponseWriter, r *http.Request) {
		var got otp.SendRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got.Phone != "+15551234567" {
			t.Fatalf("phone was not normalized: %q", got.Phone)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("expected Bearer test-token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Link-Signature") != "" {
			t.Fatalf("HMAC signature header must be gone, got %q", r.Header.Get("X-Link-Signature"))
		}
		if r.Header.Get("Idempotency-Key") != "signup:user-123:attempt-1" {
			t.Fatalf("idempotency key mismatch: %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_123","phone":"+15551234567","status":"queued"}`))
	})
	defer server.Close()

	client, err := NewClient(testClientID, testPrivPEM, WithBaseURL(server.URL), WithNoRetries())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone: "15551234567",
		Code:  "123456",
	}, WithIdempotencyKey("signup:user-123:attempt-1"))
	if err != nil {
		t.Fatalf("OTP.Send returned error: %v", err)
	}
	if resp.Phone != "+15551234567" {
		t.Fatalf("response phone mismatch: %q", resp.Phone)
	}
}

func TestOTPSendPassesLocaleTemplateAndVariables(t *testing.T) {
	var got otp.SendRequest
	server := newOTPTestServer(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_123","phone":"+15551234567","status":"queued","template_used":"tpl_a1b2c3d4e5f6g7h8","locale_used":"en"}`))
	})
	defer server.Close()

	client, err := NewClient(testClientID, testPrivPEM, WithBaseURL(server.URL), WithNoRetries())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	resp, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone:      "15551234567",
		Code:       "123456",
		Locale:     otp.LocaleEnglish,
		TemplateID: "tpl_a1b2c3d4e5f6g7h8",
		Variables:  map[string]string{"name": "Mikhail"},
	}, WithIdempotencyKey("signup:user-123:attempt-1"))
	if err != nil {
		t.Fatalf("OTP.Send returned error: %v", err)
	}
	if got.Locale != otp.LocaleEnglish {
		t.Fatalf("expected locale %q in wire body, got %q", otp.LocaleEnglish, got.Locale)
	}
	if got.TemplateID != "tpl_a1b2c3d4e5f6g7h8" {
		t.Fatalf("expected template_id in wire body, got %q", got.TemplateID)
	}
	if got.Variables["name"] != "Mikhail" {
		t.Fatalf("expected variables.name in wire body, got %v", got.Variables)
	}
	if resp.TemplateUsed != "tpl_a1b2c3d4e5f6g7h8" || resp.LocaleUsed != "en" {
		t.Fatalf("expected template_used/locale_used in response, got %q/%q", resp.TemplateUsed, resp.LocaleUsed)
	}
}

func TestOTPSendOmitsEmptyLocale(t *testing.T) {
	var rawBody []byte
	server := newOTPTestServer(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_123","phone":"+15551234567","status":"queued"}`))
	})
	defer server.Close()

	client, err := NewClient(testClientID, testPrivPEM, WithBaseURL(server.URL), WithNoRetries())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if _, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone: "15551234567",
		Code:  "123456",
	}, WithIdempotencyKey("signup:user-123:attempt-1")); err != nil {
		t.Fatalf("OTP.Send returned error: %v", err)
	}
	if strings.Contains(string(rawBody), `"locale"`) {
		t.Fatalf("empty locale must be omitted, got body=%s", rawBody)
	}
}

func TestOTPSendRejectsInvalidPhoneLocally(t *testing.T) {
	client, err := NewClient(testClientID, testPrivPEM, WithBaseURL("https://api.example.com"))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	_, err = client.OTP.Send(context.Background(), &otp.SendRequest{Phone: "bad phone", Code: "123456"})
	if err == nil {
		t.Fatal("expected invalid phone error")
	}
}
