package link

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/linkmessengerme/console-sdk-go/internal/transport"
)

var (
	ErrInvalidIDToken     = errors.New("link: ID token validation failed")
	ErrInvalidLoginConfig = errors.New("link: invalid Login configuration")
	ErrLoginKeySource     = errors.New("link: Login signing keys unavailable")
	loginNonce            = regexp.MustCompile(`^[A-Za-z0-9_-]{32,256}$`)
	loginCompact          = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
)

// LoginConfig is trusted server configuration, never values from an incoming JWT.
// ClientIDs contains the Login client IDs for this backend's iOS/Android/Web apps.
type LoginConfig struct {
	Issuer     string
	APIBaseURL string
	ClientIDs  []string
	HTTPClient *http.Client
}

// IDTokenClaims contains verified identity, not permission to read phone data.
type IDTokenClaims struct {
	Issuer    string
	Subject   string
	ClientID  string
	Nonce     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// LoginClient verifies Link Login ID tokens and reads fresh UserInfo without an
// app private key. Reuse one client to share its bounded signing-key cache.
type LoginClient struct {
	*LoginService
	issuer, base       string
	clients            map[string]bool
	http               *http.Client
	now                func() time.Time
	mu                 sync.Mutex
	keys               map[string]ed25519.PublicKey
	expires, nextFetch time.Time
	loading            chan struct{}
	keyError           error
}

func loginOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(raw, "\\ \t\r\n?#") {
		return "", ErrInvalidLoginConfig
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", ErrInvalidLoginConfig
	}
	return strings.TrimSuffix(raw, "/"), nil
}

func NewLoginClient(cfg LoginConfig) (*LoginClient, error) {
	issuer, err := loginOrigin(cfg.Issuer)
	if err != nil || issuer != cfg.Issuer {
		return nil, ErrInvalidLoginConfig
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = issuer
	}
	base, err := loginOrigin(cfg.APIBaseURL)
	if err != nil || len(cfg.ClientIDs) == 0 || len(cfg.ClientIDs) > 32 {
		return nil, ErrInvalidLoginConfig
	}
	clients := make(map[string]bool, len(cfg.ClientIDs))
	for _, id := range cfg.ClientIDs {
		if id == "" || len(id) > 40 || strings.TrimSpace(id) != id {
			return nil, ErrInvalidLoginConfig
		}
		clients[id] = true
	}
	hc := &http.Client{Timeout: 5 * time.Second}
	if cfg.HTTPClient != nil {
		*hc = *cfg.HTTPClient
	}
	if hc.Timeout <= 0 || hc.Timeout > 5*time.Second {
		hc.Timeout = 5 * time.Second
	}
	hc.Jar = nil
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	tr, err := transport.NewUserClient(base, hc, "link-console-sdk-go/"+Version)
	if err != nil {
		return nil, ErrInvalidLoginConfig
	}
	return &LoginClient{LoginService: &LoginService{transport: tr}, issuer: issuer, base: base, clients: clients, http: hc, now: time.Now}, nil
}

// VerifyIDToken requires the nonce stored by the backend BEFORE sign-in. It does
// not consume it: the caller must atomically consume the login challenge once.
// Refreshed ID tokens without that nonce cannot complete a new login challenge.
func (c *LoginClient) VerifyIDToken(ctx context.Context, raw, expectedNonce string) (*IDTokenClaims, error) {
	if c == nil {
		return nil, ErrNilClient
	}
	if len(raw) > 8192 || !loginCompact.MatchString(raw) || !loginNonce.MatchString(expectedNonce) {
		return nil, ErrInvalidIDToken
	}
	unverified, _, err := jwt.NewParser(jwt.WithJSONNumber()).ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		return nil, ErrInvalidIDToken
	}
	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" || len(kid) > 128 || unverified.Header["alg"] != "EdDSA" || unverified.Header["typ"] != "JWT" {
		return nil, ErrInvalidIDToken
	}
	for _, field := range []string{"crit", "b64", "jku", "jwk", "x5u", "x5c"} {
		if _, present := unverified.Header[field]; present {
			return nil, ErrInvalidIDToken
		}
	}
	// Reject wrong-purpose/issuer/nonce inputs before making network requests.
	if _, err = c.identity(unverified.Claims.(jwt.MapClaims), expectedNonce); err != nil {
		return nil, err
	}
	key, err := c.key(ctx, kid)
	if err != nil {
		return nil, err
	}
	token, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(c.issuer),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithJSONNumber(),
		jwt.WithLeeway(30*time.Second), jwt.WithTimeFunc(c.now))
	if err != nil || !token.Valid {
		return nil, ErrInvalidIDToken
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.identity(token.Claims.(jwt.MapClaims), expectedNonce)
}

func (c *LoginClient) identity(m jwt.MapClaims, nonce string) (*IDTokenClaims, error) {
	issuer, _ := m["iss"].(string)
	sub, _ := m["sub"].(string)
	azp, _ := m["azp"].(string)
	actualNonce, _ := m["nonce"].(string)
	aud, err := m.GetAudience()
	if err != nil || len(aud) != 1 || !c.clients[aud[0]] || azp != aud[0] || issuer != c.issuer || sub == "" || len(sub) > 255 || subtle.ConstantTimeCompare([]byte(actualNonce), []byte(nonce)) != 1 {
		return nil, ErrInvalidIDToken
	}
	if _, exists := m["token_use"]; exists {
		return nil, ErrInvalidIDToken
	}
	iat, iok := m["iat"].(json.Number)
	exp, eok := m["exp"].(json.Number)
	i, ie := iat.Int64()
	e, ee := exp.Int64()
	now := c.now().Unix()
	if !iok || !eok || ie != nil || ee != nil || i <= 0 || e <= i || e > 9007199254740991 || i > now+30 || e <= now-30 {
		return nil, ErrInvalidIDToken
	}
	if n, exists := m["nbf"]; exists {
		number, ok := n.(json.Number)
		value, err := number.Int64()
		if !ok || err != nil || value < 0 || value > now+30 {
			return nil, ErrInvalidIDToken
		}
	}
	return &IDTokenClaims{Issuer: issuer, Subject: sub, ClientID: aud[0], Nonce: actualNonce, IssuedAt: time.Unix(i, 0), ExpiresAt: time.Unix(e, 0)}, nil
}

func (c *LoginClient) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		now := c.now()
		if key := c.keys[kid]; key != nil && now.Before(c.expires) {
			c.mu.Unlock()
			return key, nil
		}
		if wait := c.loading; wait != nil {
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if now.Before(c.nextFetch) {
			err := c.keyError
			if err == nil {
				err = ErrInvalidIDToken
			}
			c.mu.Unlock()
			return nil, err
		}
		c.loading = make(chan struct{})
		c.mu.Unlock()
		keys, err := c.loadKeys(ctx)
		c.mu.Lock()
		c.keyError = err
		if err == nil {
			c.keys, c.expires, c.nextFetch = keys, now.Add(5*time.Minute), now.Add(30*time.Second)
		} else {
			c.nextFetch = c.now().Add(time.Second)
			if ctx.Err() != nil {
				c.nextFetch = time.Time{}
			}
		}
		close(c.loading)
		c.loading = nil
		key := c.keys[kid]
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, ErrInvalidIDToken
		}
		return key, nil
	}
}

func (c *LoginClient) loadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var discovery struct {
		Issuer     string   `json:"issuer"`
		JWKS       string   `json:"jwks_uri"`
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := c.publicJSON(ctx, c.base+"/.well-known/openid-configuration", &discovery); err != nil {
		return nil, err
	}
	eddsa := false
	for _, alg := range discovery.Algorithms {
		eddsa = eddsa || alg == "EdDSA"
	}
	if discovery.Issuer != c.issuer || discovery.JWKS != c.base+"/oauth/v1/jwks.json" || !eddsa {
		return nil, ErrLoginKeySource
	}
	var set struct {
		Keys []struct {
			KID   string          `json:"kid"`
			KTY   string          `json:"kty"`
			Curve string          `json:"crv"`
			X     string          `json:"x"`
			Alg   string          `json:"alg"`
			Use   string          `json:"use"`
			Ops   []string        `json:"key_ops"`
			D     json.RawMessage `json:"d"`
		} `json:"keys"`
	}
	if err := c.publicJSON(ctx, discovery.JWKS, &set); err != nil {
		return nil, err
	}
	if len(set.Keys) == 0 || len(set.Keys) > 32 {
		return nil, ErrLoginKeySource
	}
	keys := make(map[string]ed25519.PublicKey)
	for _, k := range set.Keys {
		if k.KID == "" || len(k.KID) > 128 || keys[k.KID] != nil || k.KTY != "OKP" || k.Curve != "Ed25519" || k.Alg != "EdDSA" || k.Use != "sig" || k.D != nil || (k.Ops != nil && (len(k.Ops) != 1 || k.Ops[0] != "verify")) {
			return nil, ErrLoginKeySource
		}
		key, err := base64.RawURLEncoding.Strict().DecodeString(k.X)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, ErrLoginKeySource
		}
		keys[k.KID] = ed25519.PublicKey(key)
	}
	return keys, nil
}

func (c *LoginClient) publicJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ErrLoginKeySource
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "link-console-sdk-go/"+Version)
	resp, err := c.http.Do(req)
	if err != nil {
		return ErrLoginKeySource
	}
	defer resp.Body.Close()
	kind, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" || resp.StatusCode != http.StatusOK || resp.ContentLength > 65536 || (resp.Request != nil && resp.Request.URL.String() != target) {
		return ErrLoginKeySource
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 || !utf8.Valid(data) || json.Unmarshal(data, out) != nil {
		return ErrLoginKeySource
	}
	return nil
}
