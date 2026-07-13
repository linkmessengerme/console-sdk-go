package link

import (
	"net/http"
	"strings"
	"time"
)

type Option func(*clientOptions)

type clientOptions struct {
	baseURL         string
	httpClient      *http.Client
	timeout         time.Duration
	retry           RetryConfig
	userAgentSuffix string
	now             func() time.Time
}

func defaultClientOptions() clientOptions {
	return clientOptions{
		timeout: 10 * time.Second,
		retry:   defaultRetryConfig(),
	}
}

func WithBaseURL(baseURL string) Option {
	return func(o *clientOptions) {
		o.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(o *clientOptions) {
		o.httpClient = client
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *clientOptions) {
		o.timeout = timeout
	}
}

func WithRetryConfig(cfg RetryConfig) Option {
	return func(o *clientOptions) {
		o.retry = cfg
	}
}

func WithNoRetries() Option {
	return func(o *clientOptions) {
		o.retry = RetryConfig{MaxRetries: -1}
	}
}

func WithUserAgentSuffix(suffix string) Option {
	return func(o *clientOptions) {
		o.userAgentSuffix = strings.TrimSpace(suffix)
	}
}

func withClock(now func() time.Time) Option {
	return func(o *clientOptions) {
		o.now = now
	}
}
