package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

// signEdDSAJWT собирает компактный JWS (EdDSA, header kid не нужен — клиентский
// assertion верифицируется по зарегистрированному публичному ключу клиента).
// Zero-dep: ручная сборка header.payload.signature вместо внешней JWT-библиотеки.
func signEdDSAJWT(priv ed25519.PrivateKey, claims map[string]any) (string, error) {
	header := b64url([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := header + "." + b64url(payloadJSON)
	sig := ed25519.Sign(priv, []byte(signingInput))
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b64url(b), nil
}
