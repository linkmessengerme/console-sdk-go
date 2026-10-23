package link

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testLoginIssuer = "https://issuer.example.test"
const testLoginBase = "https://regional.example.test"

var testLoginNonce = strings.Repeat("n", 43)

type loginFixture struct {
	t         *testing.T
	private   ed25519.PrivateKey
	client    *LoginClient
	mu        sync.Mutex
	calls     int
	discovery map[string]any
	jwks      map[string]any
	profile   map[string]any
	status    int
	now       time.Time
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &loginFixture{t: t, private: private, status: 200, now: time.Unix(1791792000, 0)}
	f.discovery = map[string]any{"issuer": testLoginIssuer, "jwks_uri": testLoginBase + "/oauth/v1/jwks.json", "id_token_signing_alg_values_supported": []string{"EdDSA"}}
	f.jwks = map[string]any{"keys": []any{map[string]any{"kid": "key1", "kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "x": base64.RawURLEncoding.EncodeToString(public)}}}
	f.profile = map[string]any{"sub": "user1", "name": "Test", "phone_number": "+12025550123"}
	f.client, err = NewLoginClient(LoginConfig{Issuer: testLoginIssuer, APIBaseURL: testLoginBase, ClientIDs: []string{"ios-client", "android-client"}, HTTPClient: &http.Client{Transport: userRoundTripper(func(r *http.Request) (*http.Response, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		var body any
		switch r.URL.String() {
		case testLoginBase + "/.well-known/openid-configuration":
			body = f.discovery
		case testLoginBase + "/oauth/v1/jwks.json":
			body = f.jwks
		case testLoginBase + "/oauth/v1/userinfo":
			body = f.profile
		default:
			t.Errorf("untrusted request %s", r.URL)
			return nil, errors.New("untrusted target")
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("sent a cookie")
		}
		if strings.HasSuffix(r.URL.Path, "userinfo") {
			if r.Header.Get("Authorization") != "Bearer user-access" {
				t.Error("missing user bearer")
			}
		} else if r.Header.Get("Authorization") != "" {
			t.Error("sent credentials to key source")
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: f.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	f.client.now = func() time.Time { return f.now }
	return f
}

func (f *loginFixture) token(change func(jwt.MapClaims, map[string]any)) string {
	p := jwt.MapClaims{"iss": testLoginIssuer, "sub": "user1", "aud": []string{"ios-client"}, "azp": "ios-client", "nonce": testLoginNonce, "iat": f.now.Unix(), "exp": f.now.Unix() + 300}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, p)
	token.Header["kid"] = "key1"
	if change != nil {
		change(p, token.Header)
	}
	raw, err := token.SignedString(f.private)
	if err != nil {
		f.t.Fatal(err)
	}
	return raw
}

func TestVerifyIDTokenAndSubjectBoundUserInfo(t *testing.T) {
	f := newLoginFixture(t)
	ctx := context.Background()
	raw := f.token(nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := f.client.VerifyIDToken(ctx, raw, testLoginNonce)
			if err != nil || p == nil || p.Subject != "user1" || p.ClientID != "ios-client" {
				t.Errorf("verification failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if f.calls != 2 {
		t.Fatalf("expected one discovery/JWKS fetch, got %d", f.calls)
	}
	p, err := f.client.GetUserInfo(ctx, "user-access", WithExpectedSubject("user1"))
	if err != nil || p.PhoneNumber == nil || *p.PhoneNumber != "+12025550123" || p.PhoneNumberVerified != nil {
		t.Fatalf("profile: %#v %v", p, err)
	}
	f.profile["sub"] = "other"
	if _, err = f.client.GetUserInfo(ctx, "user-access", WithExpectedSubject("user1")); !errors.Is(err, ErrInvalidResponse) {
		t.Fatal(err)
	}
}

func TestVerifyIDTokenRejectsInvalidClaimsBeforeNetwork(t *testing.T) {
	cases := map[string]func(jwt.MapClaims, map[string]any){
		"issuer":             func(p jwt.MapClaims, _ map[string]any) { p["iss"] = "https://evil.test" },
		"audience":           func(p jwt.MapClaims, _ map[string]any) { p["aud"] = "other-client" },
		"multiple audiences": func(p jwt.MapClaims, _ map[string]any) { p["aud"] = []string{"ios-client", "android-client"} },
		"authorized party":   func(p jwt.MapClaims, _ map[string]any) { p["azp"] = "android-client" },
		"missing azp":        func(p jwt.MapClaims, _ map[string]any) { delete(p, "azp") },
		"missing subject":    func(p jwt.MapClaims, _ map[string]any) { delete(p, "sub") },
		"nonce":              func(p jwt.MapClaims, _ map[string]any) { p["nonce"] = strings.Repeat("x", 43) },
		"refresh token":      func(p jwt.MapClaims, _ map[string]any) { delete(p, "nonce") },
		"access token":       func(p jwt.MapClaims, _ map[string]any) { p["token_use"] = "access" },
		"expired":            func(p jwt.MapClaims, _ map[string]any) { p["iat"] = 1791791600; p["exp"] = 1791791900 },
		"future":             func(p jwt.MapClaims, _ map[string]any) { p["iat"] = 1791792100 },
		"missing expiration": func(p jwt.MapClaims, _ map[string]any) { delete(p, "exp") },
		"fractional time":    func(p jwt.MapClaims, _ map[string]any) { p["iat"] = 1791792000.5 },
		"not before":         func(p jwt.MapClaims, _ map[string]any) { p["nbf"] = 1791792100 },
		"jku":                func(_ jwt.MapClaims, h map[string]any) { h["jku"] = "https://evil.test/keys" },
		"critical":           func(_ jwt.MapClaims, h map[string]any) { h["crit"] = []string{} },
		"wrong type":         func(_ jwt.MapClaims, h map[string]any) { h["typ"] = "at+jwt" },
		"missing kid":        func(_ jwt.MapClaims, h map[string]any) { delete(h, "kid") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newLoginFixture(t)
			raw := f.token(change)
			if _, err := f.client.VerifyIDToken(context.Background(), raw, testLoginNonce); !errors.Is(err, ErrInvalidIDToken) {
				t.Fatal(err)
			}
			if f.calls != 0 {
				t.Fatal("invalid claims fetched keys")
			}
		})
	}
}

func TestVerifyIDTokenCryptographyAndCacheRotation(t *testing.T) {
	f := newLoginFixture(t)
	ctx := context.Background()
	raw := f.token(nil)
	if _, err := f.client.VerifyIDToken(ctx, raw, testLoginNonce); err != nil {
		t.Fatal(err)
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	f.private = other
	if _, err := f.client.VerifyIDToken(ctx, f.token(nil), testLoginNonce); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatal(err)
	}
	f.jwks["keys"] = []any{map[string]any{"kid": "key2", "kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "x": base64.RawURLEncoding.EncodeToString(other.Public().(ed25519.PublicKey))}}
	newToken := f.token(func(_ jwt.MapClaims, h map[string]any) { h["kid"] = "key2" })
	for i := 0; i < 10; i++ {
		if _, err := f.client.VerifyIDToken(ctx, newToken, testLoginNonce); !errors.Is(err, ErrInvalidIDToken) {
			t.Fatal(err)
		}
	}
	if f.calls != 2 {
		t.Fatal("unknown kid bypassed cooldown")
	}
	f.now = f.now.Add(31 * time.Second)
	if _, err := f.client.VerifyIDToken(ctx, newToken, testLoginNonce); err != nil {
		t.Fatal(err)
	}
	if f.calls != 4 {
		t.Fatal("keys did not rotate")
	}
	f.now = f.now.Add(301 * time.Second)
	f.status = 503
	if _, err := f.client.VerifyIDToken(ctx, f.token(func(_ jwt.MapClaims, h map[string]any) { h["kid"] = "key2" }), testLoginNonce); !errors.Is(err, ErrLoginKeySource) {
		t.Fatal(err)
	}
}

func TestVerifyIDTokenUntrustedKeySourceAndInputs(t *testing.T) {
	for _, change := range []func(*loginFixture){
		func(f *loginFixture) { f.discovery["issuer"] = "https://other.test" },
		func(f *loginFixture) { f.discovery["jwks_uri"] = "https://other.test/keys" },
		func(f *loginFixture) { f.status = 302 },
		func(f *loginFixture) { keys := f.jwks["keys"].([]any); f.jwks["keys"] = append(keys, keys[0]) },
		func(f *loginFixture) { f.jwks["keys"].([]any)[0].(map[string]any)["d"] = "private" },
		func(f *loginFixture) { f.jwks["keys"].([]any)[0].(map[string]any)["x"] = "bad" },
	} {
		f := newLoginFixture(t)
		change(f)
		if _, err := f.client.VerifyIDToken(context.Background(), f.token(nil), testLoginNonce); !errors.Is(err, ErrLoginKeySource) {
			t.Fatal(err)
		}
	}
	f := newLoginFixture(t)
	for _, raw := range []string{"", "opaque-access", strings.Repeat("a", 8193), "eyJhbGciOiJub25lIn0.e30.eA"} {
		if _, err := f.client.VerifyIDToken(context.Background(), raw, testLoginNonce); !errors.Is(err, ErrInvalidIDToken) {
			t.Fatal(err)
		}
	}
	if _, err := f.client.VerifyIDToken(context.Background(), f.token(nil), ""); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.client.VerifyIDToken(ctx, f.token(nil), testLoginNonce); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatal("invalid input made a request")
	}
	for _, origin := range []string{"", "http://public.test", "https://user:pass@example.test", testLoginIssuer + "/path", testLoginIssuer + "/", testLoginIssuer + "?"} {
		if _, err := NewLoginClient(LoginConfig{Issuer: origin, ClientIDs: []string{"ios-client"}}); !errors.Is(err, ErrInvalidLoginConfig) {
			t.Fatalf("accepted %s", origin)
		}
	}
}
