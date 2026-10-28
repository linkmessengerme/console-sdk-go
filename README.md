# Link Console Go SDK

Official Go SDK for Link Developer Platform server-side APIs: OTP delivery,
Login with Link, Mini Apps UserInfo and merchant payments.

For OTP, the SDK authenticates with OAuth2 `client_credentials` using a `private_key_jwt`
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

### OTP authentication flow

On the first OTP call (and whenever the cached token is near expiry, or after a
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

## Login and Mini Apps UserInfo

For Login with your own backend, `NewLoginClient` below uses public configuration
and needs no OTP key file.

Use the same client with a **user access token** supplied per call:

```go
// Login: access token from the user's completed Login authorization flow.
info, err := client.Login.GetUserInfo(ctx, loginAccessToken,
	link.WithExpectedSubject(verifiedSessionSub), // optional session binding
)
if err != nil {
	return err
}
if info.PhoneNumber != nil {
	// Use *info.PhoneNumber for the authorized operation; do not log it.
}

// Mini Apps: accessToken returned by LinkMiniApp.init() (the ma_ token).
miniInfo, err := client.MiniApps.GetUserInfo(ctx, miniAppAccessToken,
	link.WithExpectedSubject(verifiedMiniAppSessionSub),
)
```

Only `ctx` and the access token are required. Do not pass a user ID, a scope list,
an ID token, or an app `client_credentials` token. Request `phone` during Login
authorization or enable it in Mini Apps Permissions; the project must have the
scope approved and the user must consent. The API returns only currently
authorized fields. `PhoneNumber` is `nil` when unavailable; `PhoneNumberVerified`
is a separate `*bool` and is currently omitted by Link. Absence is not `false` or
proof of ownership. Login returns `Name`, `PreferredUsername`, `Picture`; Mini
Apps returns an optional `Profile` (`Name`, `Username`, `AvatarURL`).

If the frontend obtained the token, send it to your backend through your own
authenticated HTTPS API. `WithExpectedSubject` compares against the `sub` from
your already verified Login/Mini App session; this method does not verify an ID
token or establish a backend session itself.

UserInfo calls `GET /oauth/v1/userinfo` or `GET /miniapps/v1/userinfo` at the
configured `api_base_url` using the supplied bearer. The origin must be HTTPS
(HTTP is allowed only on loopback for development), with no path prefix, query,
credentials or fragment. App credentials are not sent or used for UserInfo.
The SDK never caches user tokens or phone data, follows redirects, or retries
UserInfo. It rejects malformed/oversized responses (64 KiB limit), supports
context cancellation and the HTTP timeout, and omits raw response data from
errors. Handle `ErrInvalidResponse` for a malformed response or subject mismatch,
and `IsAuthentication(err)` for a rejected/expired user token.

Get a fresh Login access token through the original authorization/session flow
after expiry. Mini App tokens last 60 seconds; the host reissues an expired one as a
new launch, and the frontend can obtain the current token from `init()` and pass it to the backend.
The server SDK does not refresh either token or replace it with an app token.

## Verify a Login ID token on your backend

```go
// Reuse one LoginClient; configure only your backend's public Login client IDs.
login, err := link.NewLoginClient(link.LoginConfig{
    Issuer: "https://api-sdk.linkmessenger.me",
    APIBaseURL: "https://api-sdk-fr.linkmessenger.me", // optional regional origin
    ClientIDs: []string{"YOUR_IOS_LOGIN_CLIENT_ID", "YOUR_ANDROID_LOGIN_CLIENT_ID"},
})
if err != nil { return err }

// savedChallenge was created by YOUR backend before sign-in and consumed once.
identity, err := login.VerifyIDToken(ctx, idToken, savedChallenge.Nonce)
if err != nil { return err }
profile, err := login.GetUserInfo(ctx, loginAccessToken,
    link.WithExpectedSubject(identity.Subject))
if err != nil { return err }
// Map (identity.Issuer, identity.Subject) to your account. Check profile.PhoneNumber
// if your application requires a phone, then issue your own session tokens.
```

`VerifyIDToken` checks the EdDSA signature and `kid`, configured issuer, exactly
one allowed audience, matching `azp`, subject, `iat`/`exp`/optional `nbf` (30-second
clock tolerance), and the expected nonce. It returns `IDTokenClaims` with
`Issuer`, `Subject`, `ClientID`, `Nonce`, `IssuedAt`, `ExpiresAt` (Go `time.Time`).
Access tokens and refreshed ID tokens without nonce are rejected. This verifier
is for Link Login, not Mini App tokens.

Use `errors.Is(err, link.ErrInvalidIDToken)` for rejected identity and
`errors.Is(err, link.ErrLoginKeySource)` for unavailable discovery/signing keys.
`ErrInvalidLoginConfig` means invalid trusted configuration. `HTTPClient` is
optional; the SDK disables redirects/cookies and caps its timeout at five seconds.
Only configured API origins supply discovery/JWKS; a shared five-minute key cache
coalesces loads, unknown-key refreshes are limited to once per 30 seconds, failed
loads can retry after one second. No app private key is needed.

Generate the nonce on your backend using `crypto/rand` (32 bytes encoded as
unpadded base64url recommended), save it in a short-lived one-use challenge, and
pass it to native sign-in. Allowed nonce format is 32–256 base64url characters.
The expected nonce must come from saved server state, never from the incoming JWT.
The SDK compares nonce; your backend must atomically consume the challenge.
Signature verification does not query revocation or establish your app session.
UserInfo with `WithExpectedSubject` supplies fresh authorized profile/phone.

The complete [own-backend example](https://github.com/linkmessengerme/console-sdk-node/tree/main/examples/own-backend)
shows the same flow in Node, including Swift/Kotlin integration, one-use challenge,
own access/refresh tokens, rotation, refresh reuse revocation and logout.

## Mini Apps merchant payments

Use `client.Payments` with request/response types from
`github.com/linkmessengerme/console-sdk-go/payments`.

```go
// client is created from a Test key file. IDs and buyerSub come from saved backend state.
created, err := client.Payments.CreateOrder(ctx, &payments.CreateOrderRequest{
    AppID: appID, OperationID: savedCreateOperationID,
    MerchantOrderID: merchantOrderID, BuyerSub: verifiedBuyerSub,
    Product: &payments.ProductSnapshot{
        SKU: "coins-100", Version: 1, Name: "100 coins", Kind: payments.ProductKindConsumable,
    },
    // currency and method: the values your Mini App is enabled for in Link Console.
    Payment: &payments.PaymentTerms{AmountMinor: 1250, Currency: currency, Method: method},
    TestOutcome: payments.TestOutcomeSuccess,
})
if err != nil { return err }
if created.Result != payments.CreateOrderResultOK {
    // Handle this business refusal; Checkout can be nil.
    return fmt.Errorf("create result: %s", created.Result)
}
orderID := created.Checkout.Order.OrderID
current, err := client.Payments.GetOrder(ctx, &payments.GetOrderRequest{AppID: appID, OrderID: orderID})
```

`GetOrderByMerchantReference` accepts `payments.GetOrderByMerchantReferenceRequest`
(`AppID`, `MerchantOrderID`); `CancelOrder` accepts `payments.CancelOrderRequest`
(`AppID`, `OrderID`, `OperationID`). Each method returns its correspondingly named
response. `int64` amounts, product/offer versions and revisions remain native Go
`int64`, encoded as decimal strings on the wire. Amounts are positive minor currency
units, never floats. Timestamps use `*time.Time`; an absent `PaidAt` is `nil`.

```go
// Only after a PAID order and your durable grant/receipt transaction has committed:
ack, err := client.Payments.RecordFulfillment(ctx, &payments.RecordFulfillmentRequest{
    AppID: appID, OrderID: paidOrder.OrderID, OperationID: savedFulfillmentOperationID,
    ReceiptID: committedReceiptID, SKU: paidOrder.Product.SKU,
    ProductVersion: paidOrder.Product.Version, BuyerSub: paidOrder.BuyerSub,
})
if err != nil { return err }
if ack.Result != payments.RecordFulfillmentResultOK {
    return fmt.Errorf("fulfillment result: %s", ack.Result)
}
```

The same server OAuth key can serve OTP and payments. Payment methods lazily
request **only `scope=payments`**; OTP continues requesting `scope=otp`. Each scope
has a separate token cache and refresh, so enabling this API does not require
payments permission for OTP-only clients. The OAuth client must allow the requested
scope. Mini App payment access is a separate per-app **Test/Live grant** in the
payments database; a scope alone does not grant access. Both environments require
their own active grant. Environment, project and workspace come from the server
token and cannot be supplied in a payment request.

The backend obtains `buyer_sub` from its authenticated, verified Mini App session
and computes the product snapshot and price itself. Do not trust a buyer identity
or price supplied by Mini App JavaScript. The merchant API does not accept internal
user IDs, phone numbers or OTP codes. Native checkout (`StartAttempt`, OTP,
`ConfirmAttempt`) belongs to the Link host through the native checkout API and requires
the user's Link session. Merchant OAuth cannot replace that session.

All five methods send POST with `Content-Type` and `Accept: application/json`,
using the gateway's ProtoJSON contract (snake_case fields and full enum names):

| Operation | Path suffix after `/sdk/v1/miniapps/payments/orders` |
| --- | --- |
| CreateOrder | empty suffix |
| GetOrder | `/get` |
| GetOrderByMerchantReference | `/by-merchant-reference` |
| CancelOrder | `/cancel` |
| RecordFulfillment | `/fulfillment` |

Persist the complete mutation request **before** sending it, including a nonzero
UUID `operation_id`. Use a distinct UUID for each create, cancel or fulfillment
operation; reuse the same UUID and identical body after timeouts, lost responses
or process restarts. The SDK does not generate this business key. Payments use
body-level idempotency, not the OTP `Idempotency-Key` header. Reads and mutations
with a validated operation UUID use the configured transient retry policy; 401
refreshes only the payment token. Business refusals are never retried automatically.

HTTP 200 means the request was processed, not that the purchase succeeded. Inspect
`result`: create can return access-not-approved/revoked, method-not-available or
idempotency-conflict; cancel can return order-expired, already-paid,
attempt-in-progress, cancelled or idempotency-conflict; fulfillment can return
fulfillment-conflict or idempotency-conflict. The exported result enums preserve
the exact `*_RESULT_CODE_*` wire values. `checkout` can be absent on a create
refusal; `order` can be absent on a cancel/fulfillment idempotency conflict.

Non-2xx ProtoJSON `ApiError` responses use the existing SDK error classes, preserving
`ERROR_CODE_*`, request ID and details (`trace_id`, `field_violations`, `rate_limit`).
401/403 map to authentication errors; 429 maps to rate-limit errors. A missing order
is HTTP 404. OTP's existing error envelope remains supported.

Order states are only `OPEN`, `PROCESSING`, `PAID`, `CANCELLED`, `EXPIRED` (plus the
proto `UNSPECIFIED` value). Fulfillment states are `NOT_REQUESTED`, `PENDING`,
`FULFILLED` (plus `UNSPECIFIED`). Poll the merchant order for authoritative state;
a successful create or native UI callback is not proof of payment. Cancellation is
not a refund. After observing `PAID`, commit your item grant and durable receipt
idempotently in your own database, then record that receipt with `RecordFulfillment`.
That method records an existing grant; it does not grant the item. On a conflict,
reconcile the receipt and returned order rather than granting again.

The [Test merchant example](examples/miniapp_payments/main.go) creates, reads, looks up and cancels an
order. It refuses a key file whose environment is not `test`. Set `LINK_TEST_KEY_FILE`,
`LINK_APP_ID`, `LINK_VERIFIED_BUYER_SUB`, `LINK_MERCHANT_ORDER_ID`, `LINK_PAYMENT_CURRENCY`,
`LINK_PAYMENT_METHOD`, and distinct persisted
`LINK_CREATE_OPERATION_ID` / `LINK_CANCEL_OPERATION_ID` UUIDs, then run `go run ./examples/miniapp_payments`.
The app needs a Test grant. `test_outcome` can be SUCCESS, INSUFFICIENT or DELAYED in
Test only; omit it (or use UNSPECIFIED) for Live. Creating a Test order alone does
not execute the native payment flow. The example does not confirm payments or grant
items. Fulfillment requires a separately completed PAID purchase and durable receipt.

## Idempotency

OTP calls use `Idempotency-Key`; payments use their required `operation_id`. Pass a stable business key for production retries:

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
