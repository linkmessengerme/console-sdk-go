package credentials

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// TypeOAuthClient is the type of the downloadable key-file (a "Google service-account
// style" OAuth client): the private key is handed out once, we keep only the public one.
const TypeOAuthClient = "link_oauth_client"

// File is the JSON key-file downloaded from Console when an OAuth client is created or
// rotated. See docs/console/api-auth/design.md §3.
type File struct {
	Type        string `json:"type"`
	ClientID    string `json:"client_id"`
	PrivateKey  string `json:"private_key"`  // PEM (PKCS#8 Ed25519)
	TokenURL    string `json:"token_url"`    // .../oauth/v1/token
	APIBaseURL  string `json:"api_base_url"` // base for product calls (/sdk/v1/...)
	Issuer      string `json:"issuer"`       // aud for the client_assertion
	Environment string `json:"environment"`  // live | test
	ProjectID   string `json:"project_id"`
}

// Credentials is a validated key-file with the private key already parsed.
type Credentials struct {
	Type        string
	ClientID    string
	PrivateKey  ed25519.PrivateKey
	TokenURL    string
	APIBaseURL  string
	Issuer      string
	Environment string
	ProjectID   string
}

func LoadFile(path string) (*Credentials, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("credentials file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}
	return LoadJSON(data)
}

func LoadJSON(data []byte) (*Credentials, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode credentials JSON: %w", err)
	}
	return FromFile(f)
}

func FromFile(f File) (*Credentials, error) {
	c := &Credentials{
		Type:        strings.TrimSpace(f.Type),
		ClientID:    strings.TrimSpace(f.ClientID),
		TokenURL:    strings.TrimSpace(f.TokenURL),
		APIBaseURL:  strings.TrimSpace(f.APIBaseURL),
		Issuer:      strings.TrimSpace(f.Issuer),
		Environment: strings.TrimSpace(f.Environment),
		ProjectID:   strings.TrimSpace(f.ProjectID),
	}
	if c.Type != TypeOAuthClient {
		return nil, fmt.Errorf("credentials type must be %q", TypeOAuthClient)
	}
	if c.ClientID == "" {
		return nil, errors.New("credentials client_id is required")
	}
	priv, err := ParseEd25519PrivateKey(f.PrivateKey)
	if err != nil {
		return nil, err
	}
	c.PrivateKey = priv
	if err := c.normalizeAndValidateURLs(); err != nil {
		return nil, err
	}
	return c, nil
}

// FromClientKey builds credentials programmatically from a client_id + PEM private key +
// base URL (token_url/issuer are derived from apiBaseURL when custom ones aren't needed).
func FromClientKey(clientID, privateKeyPEM, apiBaseURL string) (*Credentials, error) {
	priv, err := ParseEd25519PrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	c := &Credentials{
		Type:       TypeOAuthClient,
		ClientID:   strings.TrimSpace(clientID),
		PrivateKey: priv,
		APIBaseURL: strings.TrimSpace(apiBaseURL),
	}
	if c.ClientID == "" {
		return nil, errors.New("credentials client_id is required")
	}
	if err := c.normalizeAndValidateURLs(); err != nil {
		return nil, err
	}
	return c, nil
}

// normalizeAndValidateURLs derives token_url/issuer from api_base_url when they are empty,
// and validates all URLs.
func (c *Credentials) normalizeAndValidateURLs() error {
	if err := ValidateURL(c.APIBaseURL, "api_base_url"); err != nil {
		return err
	}
	base := strings.TrimRight(c.APIBaseURL, "/")
	if c.TokenURL == "" {
		c.TokenURL = base + "/oauth/v1/token"
	}
	if c.Issuer == "" {
		c.Issuer = base
	}
	if err := ValidateURL(c.TokenURL, "token_url"); err != nil {
		return err
	}
	return nil
}

func ParseEd25519PrivateKey(pemStr string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("credentials private_key is not valid PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("credentials private_key parse: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("credentials private_key must be an Ed25519 key")
	}
	return priv, nil
}

func ValidateURL(raw, field string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("credentials %s is invalid: %w", field, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("credentials %s must use http or https", field)
	}
	if u.Host == "" {
		return fmt.Errorf("credentials %s host is required", field)
	}
	if u.Fragment != "" {
		return fmt.Errorf("credentials %s must not include a fragment", field)
	}
	return nil
}
