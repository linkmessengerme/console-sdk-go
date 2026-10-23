package link

import (
	"context"
	"net/http"
	"strings"

	"github.com/linkmessengerme/console-sdk-go/internal/credentials"
	"github.com/linkmessengerme/console-sdk-go/internal/transport"
	"github.com/linkmessengerme/console-sdk-go/otp"
)

type Client struct {
	OTP      *OTPService
	Login    *LoginService
	MiniApps *MiniAppsService

	transport *transport.Client
	creds     *credentials.Credentials
}

type OTPService struct {
	client *Client
}

// App tokens serve OTP. UserInfo receives its own user token per call.
const scopeOTP = "otp"

// NewClient builds a client programmatically from a client_id + the OAuth client's PEM
// private key. The base URL is passed via link.WithBaseURL. Prefer NewClientFromFile.
func NewClient(clientID, privateKeyPEM string, opts ...Option) (*Client, error) {
	options := defaultClientOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.baseURL == "" {
		return nil, ErrMissingBaseURL
	}
	creds, err := credentials.FromClientKey(clientID, privateKeyPEM, options.baseURL)
	if err != nil {
		return nil, err
	}
	return newClient(creds, options)
}

func NewClientFromFile(path string, opts ...Option) (*Client, error) {
	creds, err := credentials.LoadFile(path)
	if err != nil {
		return nil, err
	}
	return newClientFromCredentials(creds, opts...)
}

func NewClientFromJSON(data []byte, opts ...Option) (*Client, error) {
	creds, err := credentials.LoadJSON(data)
	if err != nil {
		return nil, err
	}
	return newClientFromCredentials(creds, opts...)
}

func newClientFromCredentials(creds *credentials.Credentials, opts ...Option) (*Client, error) {
	options := defaultClientOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.baseURL != "" {
		// The override changes where requests go (api_base_url + token_url); the issuer
		// (token aud = the AS identity) is kept from the key-file.
		creds = cloneCredentials(creds)
		creds.APIBaseURL = options.baseURL
		creds.TokenURL = strings.TrimRight(options.baseURL, "/") + "/oauth/v1/token"
	}
	return newClient(creds, options)
}

func newClient(creds *credentials.Credentials, options clientOptions) (*Client, error) {
	if creds == nil {
		return nil, ErrMissingCredentials
	}
	httpClient := options.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: options.timeout}
	} else if options.timeout > 0 && httpClient.Timeout == 0 {
		copyClient := *httpClient
		copyClient.Timeout = options.timeout
		httpClient = &copyClient
	}

	userAgent := "link-console-sdk-go/" + Version
	if options.userAgentSuffix != "" {
		userAgent += " " + options.userAgentSuffix
	}
	tr, err := transport.New(transport.Config{
		HTTPClient: httpClient,
		BaseURL:    creds.APIBaseURL,
		TokenURL:   creds.TokenURL,
		ClientID:   creds.ClientID,
		Issuer:     creds.Issuer,
		PrivateKey: creds.PrivateKey,
		Scope:      scopeOTP,
		UserAgent:  userAgent,
		Retry:      options.retry.toTransport(),
		Now:        options.now,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{transport: tr, creds: cloneCredentials(creds)}
	c.OTP = &OTPService{client: c}
	c.Login = &LoginService{transport: tr}
	c.MiniApps = &MiniAppsService{transport: tr}
	return c, nil
}

func cloneCredentials(creds *credentials.Credentials) *credentials.Credentials {
	if creds == nil {
		return nil
	}
	cp := *creds
	return &cp
}

func (s *OTPService) Send(ctx context.Context, req *otp.SendRequest, opts ...RequestOption) (*otp.SendResponse, error) {
	if s == nil || s.client == nil {
		return nil, ErrNilClient
	}
	if req == nil {
		return nil, ErrNilRequest
	}
	phone, err := normalizePhone(req.Phone)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if err := validateCode(code); err != nil {
		return nil, err
	}
	requestOpts, err := buildRequestOptions(opts)
	if err != nil {
		return nil, err
	}
	body := otp.SendRequest{
		Phone:      phone,
		Code:       code,
		Locale:     strings.TrimSpace(req.Locale),
		TemplateID: strings.TrimSpace(req.TemplateID),
		Variables:  req.Variables,
	}
	var out otp.SendResponse
	if err := s.client.transport.DoJSON(ctx, http.MethodPost, "/sdk/v1/otp/send", body, &out, requestOpts); err != nil {
		return nil, err
	}
	if out.Phone == "" {
		out.Phone = phone
	}
	return &out, nil
}
