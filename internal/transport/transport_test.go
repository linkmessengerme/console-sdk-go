package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testPriv = mustGenKey()

func mustGenKey() ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv
}

// newServer serves the token endpoint (fixed access_token) + the product path.
func newServer(product http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v1/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":900}`))
	})
	mux.HandleFunc("/sdk/v1/otp/send", product)
	return httptest.NewServer(mux)
}

func testConfig(server *httptest.Server, retry RetryConfig) Config {
	return Config{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		TokenURL:   server.URL + "/oauth/v1/token",
		ClientID:   "lk_client_test01",
		Issuer:     server.URL,
		PrivateKey: testPriv,
		Scope:      "otp",
		Retry:      retry,
		Sleep:      func(context.Context, time.Duration) error { return nil },
	}
}

func TestDoJSONMapsAPIError(t *testing.T) {
	server := newServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "INVALID_TOKEN", "message": "invalid or expired access token"},
		})
	})
	defer server.Close()

	client, err := New(testConfig(server, RetryConfig{MaxRetries: -1}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = client.DoJSON(context.Background(), http.MethodPost, "/sdk/v1/otp/send", map[string]string{"ok": "true"}, nil, RequestOptions{IdempotencyKey: "idem-1"})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
	if authErr.Code != CodeInvalidToken {
		t.Fatalf("code mismatch: %s", authErr.Code)
	}
}

func TestDoJSONRetriesRequestInProgress(t *testing.T) {
	calls := 0
	server := newServer(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"IDEMPOTENCY_KEY_CONFLICT","message":"request with this idempotency key is still in progress","details":{"reason":"request_in_progress"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer server.Close()

	client, err := New(testConfig(server, RetryConfig{MaxRetries: 1, InitialBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	err = client.DoJSON(context.Background(), http.MethodPost, "/sdk/v1/otp/send", map[string]string{"ok": "true"}, &out, RequestOptions{IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 product calls, got %d", calls)
	}
	if !out.OK {
		t.Fatal("response was not decoded")
	}
}

func TestTokenSourceMintsBearer(t *testing.T) {
	var gotAuth string
	server := newServer(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer server.Close()

	client, err := New(testConfig(server, RetryConfig{MaxRetries: -1}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := client.DoJSON(context.Background(), http.MethodPost, "/sdk/v1/otp/send", map[string]string{}, nil, RequestOptions{}); err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected Bearer test-token, got %q", gotAuth)
	}
}
