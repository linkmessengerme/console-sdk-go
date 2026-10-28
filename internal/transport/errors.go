package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ErrorCode string

const (
	CodeInvalidToken           ErrorCode = "INVALID_TOKEN"
	CodeInsufficientScope      ErrorCode = "INSUFFICIENT_SCOPE"
	CodeIdempotencyKeyConflict ErrorCode = "IDEMPOTENCY_KEY_CONFLICT"
	CodeTooManyRequests        ErrorCode = "TOO_MANY_REQUESTS"
)

type APIError struct {
	StatusCode int
	Code       ErrorCode
	Message    string
	Details    map[string]any
	RequestID  string
}

func (e *APIError) Error() string {
	if e == nil {
		return "link: API error"
	}
	if e.Code == "" {
		return fmt.Sprintf("link: API error (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("link: API error %s (%d): %s", e.Code, e.StatusCode, e.Message)
}

type AuthenticationError struct {
	*APIError
}

func (e *AuthenticationError) Error() string { return e.APIError.Error() }

type IdempotencyError struct {
	*APIError
}

func (e *IdempotencyError) Error() string { return e.APIError.Error() }

type RateLimitError struct {
	*APIError
	RetryAfter string
}

func (e *RateLimitError) Error() string { return e.APIError.Error() }

type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	if e == nil || e.Err == nil {
		return "link: connection error"
	}
	return "link: connection error: " + e.Err.Error()
}

func (e *ConnectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type errorEnvelope struct {
	Error struct {
		Code    ErrorCode      `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

func decodeAPIError(resp *http.Response, body []byte) error {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Message:    http.StatusText(resp.StatusCode),
		RequestID:  resp.Header.Get("X-Request-Id"),
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Code != "" {
		apiErr.Code = envelope.Error.Code
		apiErr.Message = envelope.Error.Message
		apiErr.Details = envelope.Error.Details
	} else {
		// Payments use common.v1.ApiError directly, without the OTP envelope.
		var wire struct {
			Code            ErrorCode        `json:"code"`
			Detail          string           `json:"detail"`
			RequestID       string           `json:"request_id"`
			TraceID         string           `json:"trace_id"`
			FieldViolations []map[string]any `json:"field_violations"`
			RateLimit       map[string]any   `json:"rate_limit"`
		}
		if err := json.Unmarshal(body, &wire); err == nil && wire.Code != "" {
			apiErr.Code = wire.Code
			if wire.Detail != "" {
				apiErr.Message = wire.Detail
			}
			if apiErr.RequestID == "" {
				apiErr.RequestID = wire.RequestID
			}
			apiErr.Details = make(map[string]any)
			if wire.TraceID != "" {
				apiErr.Details["trace_id"] = wire.TraceID
			}
			if wire.FieldViolations != nil {
				apiErr.Details["field_violations"] = wire.FieldViolations
			}
			if wire.RateLimit != nil {
				apiErr.Details["rate_limit"] = wire.RateLimit
			}
		}
	}

	switch apiErr.Code {
	case CodeInvalidToken, CodeInsufficientScope, "ERROR_CODE_UNAUTHENTICATED", "ERROR_CODE_PERMISSION_DENIED":
		return &AuthenticationError{APIError: apiErr}
	case CodeIdempotencyKeyConflict:
		return &IdempotencyError{APIError: apiErr}
	case CodeTooManyRequests, "ERROR_CODE_RATE_LIMITED":
		return &RateLimitError{APIError: apiErr, RetryAfter: resp.Header.Get("Retry-After")}
	default:
		return apiErr
	}
}
