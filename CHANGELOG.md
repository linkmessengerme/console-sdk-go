# Changelog

## Unreleased

- Added optional `Locale` field to `otp.SendRequest` (ISO 639-1: `ru`, `en`, `kk`, `uz`, `tk`; see `otp.Locale*` constants). When omitted, the gateway auto-detects the locale from the phone country code.
- Added optional `TemplateID` and `Variables` fields to `otp.SendRequest` for selecting a message template and supplying template substitutions.
- Added `TemplateUsed` and `LocaleUsed` fields to `otp.SendResponse`.

## 0.1.0

- Initial Go SDK skeleton.
- Added service-key credentials loading from file and JSON.
- Added HMAC-SHA256 request signing compatible with `sdk-api-gw`.
- Added OTP send client with phone normalization and idempotency support.
- Added typed API, authentication, idempotency, rate limit, and connection errors.
