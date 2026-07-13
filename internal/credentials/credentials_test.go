package credentials

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// keyFileJSON строит валидный key-file; overrides со значением nil удаляют поле.
func keyFileJSON(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	m := map[string]any{
		"type":         TypeOAuthClient,
		"client_id":    "lk_client_test01",
		"private_key":  testKeyPEM(t),
		"api_base_url": "https://api-sdk.example.test",
		"token_url":    "https://api-sdk.example.test/oauth/v1/token",
		"issuer":       "https://api-sdk.example.test",
		"environment":  "live",
		"project_id":   "11111111-1111-1111-1111-111111111111",
	}
	for k, v := range overrides {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLoadJSONValidOAuthClientFile(t *testing.T) {
	creds, err := LoadJSON(keyFileJSON(t, nil))
	if err != nil {
		t.Fatalf("LoadJSON returned error: %v", err)
	}
	if creds.ClientID != "lk_client_test01" {
		t.Fatalf("ClientID mismatch: %q", creds.ClientID)
	}
	if creds.TokenURL != "https://api-sdk.example.test/oauth/v1/token" {
		t.Fatalf("TokenURL mismatch: %q", creds.TokenURL)
	}
	if creds.Issuer != "https://api-sdk.example.test" {
		t.Fatalf("Issuer mismatch: %q", creds.Issuer)
	}
	if len(creds.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("private key not parsed: len=%d", len(creds.PrivateKey))
	}
}

func TestLoadJSONDerivesTokenURLAndIssuer(t *testing.T) {
	creds, err := LoadJSON(keyFileJSON(t, map[string]any{"token_url": nil, "issuer": nil}))
	if err != nil {
		t.Fatalf("LoadJSON returned error: %v", err)
	}
	if creds.TokenURL != "https://api-sdk.example.test/oauth/v1/token" {
		t.Fatalf("derived TokenURL mismatch: %q", creds.TokenURL)
	}
	if creds.Issuer != "https://api-sdk.example.test" {
		t.Fatalf("derived Issuer mismatch: %q", creds.Issuer)
	}
}

func TestLoadJSONRejectsWrongType(t *testing.T) {
	if _, err := LoadJSON(keyFileJSON(t, map[string]any{"type": "service_key"})); err == nil {
		t.Fatal("expected wrong-type error")
	}
}

func TestLoadJSONRejectsMissingClientID(t *testing.T) {
	if _, err := LoadJSON(keyFileJSON(t, map[string]any{"client_id": nil})); err == nil {
		t.Fatal("expected missing client_id error")
	}
}

func TestLoadJSONRequiresAPIBaseURL(t *testing.T) {
	if _, err := LoadJSON(keyFileJSON(t, map[string]any{"api_base_url": nil, "token_url": nil, "issuer": nil})); err == nil {
		t.Fatal("expected missing api_base_url error")
	}
}

func TestLoadJSONRejectsBadPrivateKeyWithoutLeaking(t *testing.T) {
	const bad = "totally-not-a-pem-private-key"
	_, err := LoadJSON(keyFileJSON(t, map[string]any{"private_key": bad}))
	if err == nil {
		t.Fatal("expected private_key error")
	}
	if strings.Contains(err.Error(), bad) {
		t.Fatalf("error leaks private key material: %v", err)
	}
}
