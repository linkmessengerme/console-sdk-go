package transport

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const headerIdempotencyKey = "Idempotency-Key"

type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type RequestOptions struct {
	IdempotencyKey string
	// RetrySafe marks reads or operations with durable idempotency in their body.
	RetrySafe bool
}

type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	tokens     *tokenSource
	userAgent  string
	retry      RetryConfig
	sleep      func(context.Context, time.Duration) error
}

type Config struct {
	HTTPClient *http.Client
	BaseURL    string // api_base_url — base for product calls (/sdk/v1/...)
	TokenURL   string // .../oauth/v1/token
	ClientID   string
	Issuer     string // aud client_assertion
	PrivateKey ed25519.PrivateKey
	Scope      string // requested scope of the app token (e.g. "otp")
	UserAgent  string
	Retry      RetryConfig
	Now        func() time.Time
	Sleep      func(context.Context, time.Duration) error
}

func New(cfg Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("base URL must include scheme and host")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "link-console-sdk-go"
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if len(cfg.PrivateKey) == 0 {
		return nil, errors.New("private key is required")
	}
	if strings.TrimSpace(cfg.TokenURL) == "" {
		return nil, errors.New("token URL is required")
	}
	return &Client{
		httpClient: cfg.HTTPClient,
		baseURL:    baseURL,
		tokens: &tokenSource{
			httpClient: cfg.HTTPClient,
			tokenURL:   cfg.TokenURL,
			clientID:   cfg.ClientID,
			issuer:     cfg.Issuer,
			priv:       cfg.PrivateKey,
			scope:      cfg.Scope,
			userAgent:  cfg.UserAgent,
			now:        cfg.Now,
		},
		userAgent: cfg.UserAgent,
		retry:     cfg.Retry,
		sleep:     cfg.Sleep,
	}, nil
}

func (c *Client) DoJSON(ctx context.Context, method, path string, body any, out any, opts RequestOptions) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}

	attempts := 1
	if (opts.IdempotencyKey != "" || opts.RetrySafe) && c.retry.MaxRetries > 0 {
		attempts += c.retry.MaxRetries
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return &ConnectionError{Err: err}
			}
		}
		err = c.doOnce(ctx, method, path, bodyBytes, out, opts)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte, out any, opts RequestOptions) error {
	u := *c.baseURL
	u.Path = joinPath(c.baseURL.Path, path)
	urlStr := u.String()

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}
	resp, respBody, err := c.send(ctx, method, urlStr, body, token, opts.IdempotencyKey)
	if err != nil {
		return err
	}
	// The token may have expired or been revoked between the proactive refresh and this
	// call — do a single auto-refresh (Firebase model: private key on the backend, re-mint anytime).
	if resp.StatusCode == http.StatusUnauthorized {
		c.tokens.Invalidate()
		newToken, terr := c.tokens.Token(ctx)
		if terr == nil && newToken != token {
			resp, respBody, err = c.send(ctx, method, urlStr, body, newToken, opts.IdempotencyKey)
			if err != nil {
				return err
			}
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, method, urlStr string, body []byte, token, idempotencyKey string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	if idempotencyKey != "" {
		req.Header.Set(headerIdempotencyKey, idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, &ConnectionError{Err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, &ConnectionError{Err: err}
	}
	return resp, respBody, nil
}

func (c *Client) backoff(attempt int) time.Duration {
	base := c.retry.InitialBackoff
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	max := c.retry.MaxBackoff
	if max <= 0 {
		max = 2 * time.Second
	}
	delay := base << (attempt - 1)
	if delay > max || delay <= 0 {
		delay = max
	}
	if delay <= time.Nanosecond {
		return delay
	}
	jitter := time.Duration(rand.Int63n(int64(delay / 2)))
	return delay/2 + jitter
}

func isRetryable(err error) bool {
	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return true
	}
	apiErr := apiErrorFrom(err)
	if apiErr == nil {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusConflict:
		if apiErr.Code != CodeIdempotencyKeyConflict {
			return false
		}
		reason, _ := apiErr.Details["reason"].(string)
		return reason == "request_in_progress"
	default:
		return false
	}
}

func apiErrorFrom(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var idemErr *IdempotencyError
	if errors.As(err, &idemErr) {
		return idemErr.APIError
	}
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		return rateErr.APIError
	}
	var authErr *AuthenticationError
	if errors.As(err, &authErr) {
		return authErr.APIError
	}
	return nil
}

func joinPath(base, path string) string {
	base = strings.TrimRight(base, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if base == "" {
		return path
	}
	return base + path
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
