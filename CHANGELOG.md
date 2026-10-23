# Changelog

## 0.3.0 — 2026-10-24

- Added standalone `NewLoginClient` and `VerifyIDToken`: Ed25519/JWKS signature,
  issuer/client/azp/time checks and required backend nonce; no private key required.
  Includes subject-bound UserInfo, bounded shared key cache and key rotation.
- Added own-backend integration documentation; JWT validation uses `golang-jwt/jwt/v5`.

- Added `Client.Login.GetUserInfo` and `Client.MiniApps.GetUserInfo` with typed
  profile/phone fields and optional `WithExpectedSubject` session binding.
- UserInfo uses only the per-call user bearer, with no app-token exchange,
  automatic retries, redirects, or user-data caching; bounded JSON and safe errors.
## 0.2.0 — 2026-07-13

- OAuth2 `client_credentials` with Ed25519 `private_key_jwt` replaces HMAC authentication.
- Added optional `Locale` field to `otp.SendRequest` (ISO 639-1: `ru`, `en`, `kk`, `uz`; see `otp.Locale*` constants). When omitted, the gateway auto-detects the locale from the phone country code.
- Added optional `TemplateID` and `Variables` fields to `otp.SendRequest` for selecting a message template and supplying template substitutions.
- Added `TemplateUsed` and `LocaleUsed` fields to `otp.SendResponse`.

## 0.1.0

- Initial Go SDK skeleton.
- Added service-key credentials loading from file and JSON.
- Added HMAC-SHA256 request signing compatible with `sdk-api-gw`.
- Added OTP send client with phone normalization and idempotency support.
- Added typed API, authentication, idempotency, rate limit, and connection errors.
