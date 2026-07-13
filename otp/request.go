package otp

// Supported locales (ISO 639-1) for the OTP message text rendered by Link. If
// Locale is left empty, the gateway auto-detects it from the phone country code.
const (
	LocaleRussian = "ru"
	LocaleEnglish = "en"
	LocaleKazakh  = "kk"
	LocaleUzbek   = "uz"
)

type SendRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
	// Locale is an optional ISO 639-1 code (see Locale* constants). When empty,
	// Link auto-detects the locale from the phone country code.
	Locale string `json:"locale,omitempty"`
	// TemplateID optionally selects a specific message template; when empty the
	// project's default template (or a hardcoded fallback) is used.
	TemplateID string `json:"template_id,omitempty"`
	// Variables holds optional template substitutions (e.g. {{name}}, {{amount}}).
	// {{code}} is filled automatically from Code — do not include it here.
	Variables map[string]string `json:"variables,omitempty"`
}
