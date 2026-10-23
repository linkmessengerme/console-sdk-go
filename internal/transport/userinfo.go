package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// NewUserClient has no app credentials or token source. Only GetUserJSON is used.
func NewUserClient(base string, client *http.Client, agent string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, ErrInvalidUserInfoURL
	}
	return &Client{baseURL: u, httpClient: client, userAgent: agent}, nil
}

var (
	ErrInvalidAccessToken = errors.New("link: a valid user access token is required")
	ErrInvalidUserInfoURL = errors.New("link: UserInfo requires a trusted HTTPS API origin (HTTP only on loopback)")
	ErrInvalidResponse    = errors.New("link: invalid UserInfo response")
	bearerPattern         = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)
)

// GetUserJSON uses only the supplied user token. It never mints, caches or
// refreshes an app token and never retries or follows redirects.
func (c *Client) GetUserJSON(ctx context.Context, path, token string, out any) error {
	if token == "" || len(token) > 8192 || !bearerPattern.MatchString(token) {
		return ErrInvalidAccessToken
	}
	u := *c.baseURL
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if (u.Scheme != "https" && !(u.Scheme == "http" && local)) || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return ErrInvalidUserInfoURL
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ErrInvalidUserInfoURL
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("User-Agent", c.userAgent)
	client := *c.httpClient
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout <= 0 {
		client.Timeout = 10 * time.Second
	}
	resp, err := client.Do(req)
	if err != nil {
		return userInfoConnectionError(ctx)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL.String() != u.String() {
		return ErrInvalidResponse
	}
	if resp.StatusCode != http.StatusOK {
		api := &APIError{StatusCode: resp.StatusCode, Message: "UserInfo request failed"}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			api.Code = CodeInvalidToken
			if resp.StatusCode == http.StatusForbidden {
				api.Code = CodeInsufficientScope
			}
			return &AuthenticationError{APIError: api}
		case http.StatusTooManyRequests:
			api.Code = CodeTooManyRequests
			return &RateLimitError{APIError: api}
		default:
			return api
		}
	}
	const max = 64 * 1024
	kind, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" || resp.ContentLength > max {
		return ErrInvalidResponse
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return userInfoConnectionError(ctx)
	}
	if len(body) > max || !utf8.Valid(body) || json.Unmarshal(body, out) != nil {
		return ErrInvalidResponse
	}
	if ctx.Err() != nil {
		return userInfoConnectionError(ctx)
	}
	return nil
}

func userInfoConnectionError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &ConnectionError{Err: err}
	}
	// Transport errors may echo the request/headers; don't attach their raw cause.
	return &ConnectionError{Err: errors.New("UserInfo request failed")}
}

func ValidMiniAppToken(token string) bool {
	if len(token) != 46 || !strings.HasPrefix(token, "ma_") {
		return false
	}
	for _, b := range token[3:] {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}
