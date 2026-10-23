package link

import (
	"context"
	"errors"
	"regexp"

	"github.com/linkmessengerme/console-sdk-go/internal/transport"
)

var (
	ErrInvalidAccessToken = transport.ErrInvalidAccessToken
	ErrInvalidUserInfoURL = transport.ErrInvalidUserInfoURL
	ErrInvalidResponse    = transport.ErrInvalidResponse
	ErrInvalidSubject     = errors.New("link: expected subject must be non-empty and at most 255 bytes")
	userPhonePattern      = regexp.MustCompile(`^\+[0-9]{7,15}$`)
)

type LoginUserInfo struct {
	Sub                 string  `json:"sub"`
	Name                *string `json:"name,omitempty"`
	PreferredUsername   *string `json:"preferred_username,omitempty"`
	Picture             *string `json:"picture,omitempty"`
	PhoneNumber         *string `json:"phone_number,omitempty"`
	PhoneNumberVerified *bool   `json:"phone_number_verified,omitempty"`
}

type MiniAppProfile struct {
	Name      *string `json:"name,omitempty"`
	Username  *string `json:"username,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type MiniAppUserInfo struct {
	Sub                 string          `json:"sub"`
	Profile             *MiniAppProfile `json:"profile,omitempty"`
	PhoneNumber         *string         `json:"phone_number,omitempty"`
	PhoneNumberVerified *bool           `json:"phone_number_verified,omitempty"`
}

type userInfoOptions struct{ expectedSub *string }
type UserInfoOption func(*userInfoOptions)

// WithExpectedSubject binds the response to a previously verified user session.
func WithExpectedSubject(sub string) UserInfoOption {
	return func(o *userInfoOptions) { o.expectedSub = &sub }
}

func userInfoSubject(opts []UserInfoOption) (*string, error) {
	var options userInfoOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.expectedSub != nil && (*options.expectedSub == "" || len(*options.expectedSub) > 255) {
		return nil, ErrInvalidSubject
	}
	return options.expectedSub, nil
}

func validateUserInfo(sub string, phone *string, expected *string) error {
	if sub == "" || len(sub) > 255 || (expected != nil && sub != *expected) ||
		(phone != nil && !userPhonePattern.MatchString(*phone)) {
		return ErrInvalidResponse
	}
	return nil
}

type LoginService struct{ transport *transport.Client }
type MiniAppsService struct{ transport *transport.Client }

// GetUserInfo reads fresh data using a Login user access token. Scopes are
// chosen at sign-in; the SDK does not cache the response or refresh the token.
func (s *LoginService) GetUserInfo(ctx context.Context, accessToken string, opts ...UserInfoOption) (*LoginUserInfo, error) {
	if s == nil || s.transport == nil {
		return nil, ErrNilClient
	}
	expected, err := userInfoSubject(opts)
	if err != nil {
		return nil, err
	}
	var out LoginUserInfo
	if err := s.transport.GetUserJSON(ctx, "/oauth/v1/userinfo", accessToken, &out); err != nil {
		return nil, err
	}
	if err := validateUserInfo(out.Sub, out.PhoneNumber, expected); err != nil {
		return nil, err
	}
	if out.PhoneNumber == nil {
		out.PhoneNumberVerified = nil
	}
	return &out, nil
}

// GetUserInfo reads fresh data using the short-lived ma_ token from Mini App init.
func (s *MiniAppsService) GetUserInfo(ctx context.Context, accessToken string, opts ...UserInfoOption) (*MiniAppUserInfo, error) {
	if s == nil || s.transport == nil {
		return nil, ErrNilClient
	}
	if !transport.ValidMiniAppToken(accessToken) {
		return nil, ErrInvalidAccessToken
	}
	expected, err := userInfoSubject(opts)
	if err != nil {
		return nil, err
	}
	var out MiniAppUserInfo
	if err := s.transport.GetUserJSON(ctx, "/miniapps/v1/userinfo", accessToken, &out); err != nil {
		return nil, err
	}
	if err := validateUserInfo(out.Sub, out.PhoneNumber, expected); err != nil {
		return nil, err
	}
	if out.PhoneNumber == nil {
		out.PhoneNumberVerified = nil
	}
	return &out, nil
}
