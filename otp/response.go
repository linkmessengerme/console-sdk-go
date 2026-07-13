package otp

type SendResponse struct {
	RequestID string `json:"request_id"`
	Phone     string `json:"phone"`
	Status    string `json:"status"`
	// TemplateUsed is the template that rendered the message: a template id, or
	// the literal "fallback" when the hardcoded fallback was used.
	TemplateUsed string `json:"template_used,omitempty"`
	// LocaleUsed is the locale actually used after auto-detect/fallback resolution.
	LocaleUsed string `json:"locale_used,omitempty"`
}
