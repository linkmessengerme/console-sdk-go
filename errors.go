package link

import (
	"errors"
	"regexp"
	"strings"

	"github.com/linkmessengerme/console-sdk-go/internal/transport"
)

var (
	ErrMissingCredentials = errors.New("link: credentials are required")
	ErrMissingBaseURL     = errors.New("link: base URL is required — use NewClientFromFile, or pass link.WithBaseURL")
	ErrNilClient          = errors.New("link: client is nil")
	ErrNilRequest         = errors.New("link: request is nil")
	ErrInvalidPhone       = errors.New("link: phone must contain 8-15 digits and may start with +")
	ErrInvalidCode        = errors.New("link: code must be 4-10 latin letters or digits")
)

type APIError = transport.APIError
type AuthenticationError = transport.AuthenticationError
type IdempotencyError = transport.IdempotencyError
type RateLimitError = transport.RateLimitError
type ConnectionError = transport.ConnectionError

var (
	phoneWithPlusRE = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	phoneDigitsRE   = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)
	codeRE          = regexp.MustCompile(`^[A-Za-z0-9]{4,10}$`)
)

func IsAuthentication(err error) bool {
	var target *AuthenticationError
	return errors.As(err, &target)
}

func IsRateLimit(err error) bool {
	var target *RateLimitError
	return errors.As(err, &target)
}

func AsAPIError(err error) (*APIError, bool) {
	var target *APIError
	if errors.As(err, &target) {
		return target, true
	}
	var authErr *AuthenticationError
	if errors.As(err, &authErr) {
		return authErr.APIError, true
	}
	var idemErr *IdempotencyError
	if errors.As(err, &idemErr) {
		return idemErr.APIError, true
	}
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		return rateErr.APIError, true
	}
	return nil, false
}

func normalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	if phoneWithPlusRE.MatchString(phone) {
		return phone, nil
	}
	if phoneDigitsRE.MatchString(phone) {
		return "+" + phone, nil
	}
	return "", ErrInvalidPhone
}

func validateCode(code string) error {
	if !codeRE.MatchString(code) {
		return ErrInvalidCode
	}
	return nil
}
