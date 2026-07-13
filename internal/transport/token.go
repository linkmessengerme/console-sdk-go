package transport

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	grantClientCredentials = "client_credentials"
	clientAssertionType    = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	assertionTTL           = 5 * time.Minute
	tokenExpirySkew        = 30 * time.Second
)

// tokenSource минтит client_assertion (private_key_jwt) и обменивает его на короткий
// app access_token через /oauth/v1/token (grant client_credentials), кэшируя токен до
// истечения. Это модель Firebase Admin SDK: приватный ключ всегда на бэке → новый
// токен в любой момент, refresh_token не нужен. См. docs/console/api-auth/design.md §4.1.
type tokenSource struct {
	httpClient *http.Client
	tokenURL   string
	clientID   string
	issuer     string
	priv       ed25519.PrivateKey
	scope      string
	userAgent  string
	now        func() time.Time

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (ts *tokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := ts.now()
	if ts.token != "" && now.Before(ts.expiry) {
		return ts.token, nil
	}
	return ts.refreshLocked(ctx, now)
}

// Invalidate сбрасывает кэш — вызывается при 401, чтобы следующий запрос перевыпустил токен.
func (ts *tokenSource) Invalidate() {
	ts.mu.Lock()
	ts.token = ""
	ts.expiry = time.Time{}
	ts.mu.Unlock()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func (ts *tokenSource) refreshLocked(ctx context.Context, now time.Time) (string, error) {
	assertion, err := ts.buildAssertion(now)
	if err != nil {
		return "", &ConnectionError{Err: err}
	}

	form := url.Values{}
	form.Set("grant_type", grantClientCredentials)
	form.Set("client_assertion_type", clientAssertionType)
	form.Set("client_assertion", assertion)
	form.Set("client_id", ts.clientID)
	if ts.scope != "" {
		form.Set("scope", ts.scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", &ConnectionError{Err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if ts.userAgent != "" {
		req.Header.Set("User-Agent", ts.userAgent)
	}

	resp, err := ts.httpClient.Do(req)
	if err != nil {
		return "", &ConnectionError{Err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", &ConnectionError{Err: err}
	}

	var tr tokenResponse
	_ = json.Unmarshal(body, &tr)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ts.tokenError(resp.StatusCode, &tr, resp.Header.Get("X-Request-Id"))
	}
	if tr.AccessToken == "" {
		return "", &AuthenticationError{APIError: &APIError{StatusCode: resp.StatusCode, Message: "token endpoint returned no access_token"}}
	}

	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= tokenExpirySkew {
		ts.expiry = now.Add(ttl)
	} else {
		ts.expiry = now.Add(ttl - tokenExpirySkew)
	}
	ts.token = tr.AccessToken
	return ts.token, nil
}

// tokenError переводит ответ token endpoint в SDK-ошибку: 5xx/таймаут — retryable
// (APIError со status), 4xx (invalid_client/…) — AuthenticationError (не retryable).
func (ts *tokenSource) tokenError(status int, tr *tokenResponse, requestID string) error {
	msg := tr.ErrorDesc
	if msg == "" {
		msg = tr.Error
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	apiErr := &APIError{
		StatusCode: status,
		Code:       ErrorCode(strings.ToUpper(tr.Error)),
		Message:    msg,
		RequestID:  requestID,
	}
	if status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
		return apiErr // retryable via isRetryable
	}
	return &AuthenticationError{APIError: apiErr}
}

func (ts *tokenSource) buildAssertion(now time.Time) (string, error) {
	jti, err := randomJTI()
	if err != nil {
		return "", err
	}
	claims := map[string]any{
		"iss": ts.clientID,
		"sub": ts.clientID,
		"aud": ts.issuer,
		"iat": now.Unix(),
		"exp": now.Add(assertionTTL).Unix(),
		"jti": jti,
	}
	return signEdDSAJWT(ts.priv, claims)
}
