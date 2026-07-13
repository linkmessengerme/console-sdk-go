# Link Console Go SDK

Official Go SDK for Link Developer Platform server-side APIs.

The SDK authenticates with OAuth2 `client_credentials` using a `private_key_jwt`
client assertion (Ed25519), transparently mints and caches short-lived access
tokens, sends the required SDK gateway headers, handles idempotency keys,
normalizes OTP phone numbers, and maps API failures into typed Go errors.

## Install

```sh
go get github.com/linkmessengerme/console-sdk-go
```

## Quickstart

```go
package main

import (
	"context"
	"log"
	"time"

	link "github.com/linkmessengerme/console-sdk-go"
	"github.com/linkmessengerme/console-sdk-go/otp"
)

func main() {
	client, err := link.NewClientFromFile("link-oauth-client.json",
		link.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.OTP.Send(context.Background(), &otp.SendRequest{
		Phone:  "15551234567",
		Code:   "123456",
		Locale: otp.LocaleEnglish, // optional; gateway auto-detects from phone when empty
	}, link.WithIdempotencyKey("signup:user-123:attempt-1"))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("request_id=%s status=%s", resp.RequestID, resp.Status)
}
```

## Credentials File

When you create (or rotate) an OAuth client in Console, you download a JSON
key-file with this format:

```json
{
  "type": "link_oauth_client",
  "client_id": "your-client-id",
  "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
  "token_url": "https://api.example.com/oauth/v1/token",
  "api_base_url": "https://api.example.com",
  "issuer": "https://api.example.com",
  "environment": "live",
  "project_id": "my-project-a1b2c3"
}
```

- `private_key` is a PKCS#8 Ed25519 private key in PEM form. It is issued once at
  key creation; Console keeps only the public key. Treat it like a password.
- `token_url` and `issuer` are optional — when omitted they are derived from
  `api_base_url` (`.../oauth/v1/token` and `api_base_url` respectively).
- The SDK never logs or prints `private_key`.

### Authentication flow

On the first call (and whenever the cached token is near expiry, or after a
`401`), the SDK builds a short-lived `private_key_jwt` client assertion signed
with your Ed25519 key, exchanges it at `token_url` for an app access token via
the `client_credentials` grant, and caches that token until it expires. This is
the Firebase Admin SDK model: the private key stays on your backend, so a fresh
token can be minted at any time and no `refresh_token` is needed.

### Programmatic construction

You can also build a client without a key-file:

```go
client, err := link.NewClient(
	"your-client-id",              // client_id from your key-file
	os.Getenv("LINK_PRIVATE_KEY"), // PEM Ed25519 private key ("private_key")
	link.WithBaseURL("https://api.example.com"), // key-file "api_base_url"
	link.WithTimeout(10*time.Second),
)
```

`link.NewClientFromJSON` accepts the same key-file contents as a `[]byte`.

## Idempotency

Mutating calls always use `Idempotency-Key`. Pass a stable business key for production retries:

```go
link.WithIdempotencyKey("signup:user-123:attempt-1")
```

If you do not pass a key, the SDK generates one for the current call and reuses it across internal retries. Use explicit keys when a retry can happen outside the current process.

## Phone Numbers

`OTP.Send` accepts phone numbers with or without `+` and sends the normalized `+<digits>` form to Link:

```go
otp.SendRequest{Phone: "15551234567"}  // sends "+15551234567"
otp.SendRequest{Phone: "+15551234567"} // sends "+15551234567"
```

## Locale, Template & Variables

`OTP.Send` accepts optional fields controlling the rendered message:

- `Locale` — ISO 639-1 code for the message text. When empty, Link auto-detects it from the phone country code. Supported values are exposed as constants:

```go
otp.LocaleRussian // "ru"
otp.LocaleEnglish // "en"
otp.LocaleKazakh  // "kk"
otp.LocaleUzbek   // "uz"
```

- `TemplateID` — selects a specific message template. When empty, the project's default template (or a hardcoded fallback) is used.
- `Variables` — `map[string]string` of template substitutions (e.g. `{{name}}`, `{{amount}}`). `{{code}}` is filled automatically from `Code`.

The response reports `TemplateUsed` (a template id or `"fallback"`) and `LocaleUsed` (the locale actually used after resolution).

## HTTP Client

The SDK uses Go's standard `net/http`. You can provide your own client:

```go
client, err := link.NewClientFromFile("link-oauth-client.json",
	link.WithHTTPClient(customHTTPClient),
)
```

## Errors

Use helpers and typed errors:

```go
if link.IsRateLimit(err) {
	// retry later
}

if link.IsAuthentication(err) {
	// credentials/token problem
}

if apiErr, ok := link.AsAPIError(err); ok {
	log.Printf("code=%s status=%d", apiErr.Code, apiErr.StatusCode)
}
```
